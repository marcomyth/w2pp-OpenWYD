package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// A confirmação do pagamento de uma venda em dinheiro real.
//
// É o ponto onde o dinheiro de fora encontra o jogo, e por isso é o ponto onde
// tudo o que pode dar errado dá. As três coisas que este arquivo garante:
//
//  1. IDEMPOTÊNCIA sobre a referência externa. A processadora repete aviso por
//     desenho — é assim que ela garante entrega —, e confirmação repetida não
//     pode entregar duas vezes. A âncora é a mesma do donate_topup_order (0010).
//  2. O DINHEIRO E A ENTREGA NA MESMA TRANSAÇÃO. Enfileirar a entrega fora da
//     transação que marca PAGO abre a janela em que uma queda deixa o dinheiro
//     registrado e o item não, ou o contrário.
//  3. O QUE NÃO DÁ PARA ENTREGAR VIRA DÍVIDA VISÍVEL, e não silêncio. É o estado
//     5, PAGA_SEM_ITEM, que a 0105 já previu.
//
// O QUE ESTE ARQUIVO NÃO FAZ, de propósito: tirar o item do vendedor. Enquanto a
// marca do escrow (0104) está no slot, o item é intocável — não se move, não se
// altera, não se refina, não se vende. Então não há pressa: retirar tarde é igual
// a retirar cedo, porque nada pode acontecer com ele no meio. A retirada roda
// dentro do laço do tmServer, que é o dono do baú vivo, e acontece quando der —
// agora se o vendedor estiver em jogo, no login dele se não estiver. O comprador
// nunca espera por isso.

// ResultadoCobranca é o que a confirmação fez.
type ResultadoCobranca int

const (
	// CobrancaNaoEncontrada: a referência externa não é de nenhuma cobrança
	// nossa. Não é erro — é o aviso de um pagamento que não nos diz respeito, ou
	// um aviso forjado.
	CobrancaNaoEncontrada ResultadoCobranca = iota
	// CobrancaConfirmada: o dinheiro entrou agora e a entrega foi enfileirada.
	CobrancaConfirmada
	// CobrancaJaConfirmada: já tinha sido confirmada antes. Nada mudou, e isso é
	// o caminho normal do aviso repetido, não uma anomalia.
	CobrancaJaConfirmada
	// CobrancaPagaSemItem: o dinheiro entrou e NÃO havia item para entregar — o
	// anúncio foi cancelado, ou a marca do escrow já tinha soltado. Vira dívida
	// com uma pessoa, numa fila que alguém olha.
	CobrancaPagaSemItem
	// CobrancaValorDivergente: a processadora diz ter recebido um valor diferente
	// do que a cobrança pedia.
	//
	// NÃO ENTREGA E NÃO FECHA A LINHA. Pagar menos e receber o item seria comprar
	// com desconto de si mesmo; pagar mais e receber sem troco seria o contrário.
	// Os dois pedem uma pessoa, e nenhum pede uma decisão automática — é por isso
	// que a cobrança fica como está em vez de virar PAGA_SEM_ITEM, que já é um
	// destino (reembolso) e este caso ainda não tem.
	CobrancaValorDivergente
)

// Status de rmt_cobranca (0105).
const (
	cobrancaAberta      int16 = 1
	cobrancaPaga        int16 = 2
	cobrancaCancelada   int16 = 3
	cobrancaExpirada    int16 = 4
	cobrancaPagaSemItem int16 = 5
)

// Status de rmt_anuncio (0105).
const (
	anuncioAtivo     int16 = 1
	anuncioVendido   int16 = 2
	anuncioCancelado int16 = 3
)

// VendaRMT é o que a confirmação devolve para quem for tirar o item do vendedor.
//
// Ela carrega o slot e o anúncio, e não o item: o tmServer confere a MARCA
// daquele slot contra AnuncioID antes de mexer, que é a única pergunta que
// importa lá — "este slot ainda é o deste anúncio?".
type VendaRMT struct {
	CobrancaID     int64
	AnuncioID      int64
	VendedorConta  int64
	CompradorConta int64
	CargoSlot      int16
	// EntregaID é a linha da caixa postal do comprador. Zero quando não houve
	// entrega (PAGA_SEM_ITEM).
	EntregaID int64
	// PagoComAtraso marca a confirmação que chegou DEPOIS do cancelamento ou da
	// expiração. Se isso virar rotina, o prazo da cobrança está errado.
	PagoComAtraso bool
}

// ConfirmarCobrancaRMT registra o pagamento e enfileira a entrega, numa
// transação só.
//
// A ENTREGA VAI SEMPRE PELA CAIXA POSTAL, mesmo com o comprador em jogo. Entregar
// direto no baú vivo economizaria um salto e custaria a prova: sem a linha da
// caixa postal não há `entrega_id`, e sem `entrega_id` não dá para distinguir
// "entreguei" de "ainda não entreguei" — que é exatamente a distinção que o
// estado 5 existe para preservar. Para o comprador em jogo, quem encurta a espera
// é o DeliverNow, chamado depois desta função, pelo mesmo caminho que a loja de
// doação já usa.
//
// O QUE O COMPRADOR RECEBE VEM DA FOTOGRAFIA do anúncio, e não do item no baú do
// vendedor. Os dois são iguais enquanto a marca está lá — é para isso que ela
// serve —, e ler a fotografia significa que o comprador não espera o vendedor
// estar em jogo para receber.
//
// A hora do pagamento e de que relógio ela veio chegam de fora, em pagoEm e origem:
// quem consulta a processadora é o webServer, e é ele que sabe se a resposta trouxe
// o `paid_at` ou não. valorObservado é o valor que ela diz ter recebido, conferido
// aqui dentro, com a linha travada.

// OrigemDaHora diz de que relógio veio o instante do pagamento.
type OrigemDaHora string

const (
	// HoraDaProcessadora: o paid_at que a consulta devolveu.
	HoraDaProcessadora OrigemDaHora = "syncpay"
	// HoraDoServidor: o instante em que a consulta VIU o pagamento, usado quando a
	// processadora não deu a hora — a consulta caiu para a V1, que não a documenta.
	HoraDoServidor OrigemDaHora = "servidor"
)

func (s *Store) ConfirmarCobrancaRMT(ctx context.Context, referenciaExterna string,
	pagoEm time.Time, origem OrigemDaHora, valorObservado int64,
) (ResultadoCobranca, VendaRMT, error) {
	var res ResultadoCobranca
	var venda VendaRMT

	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var statusCobranca int16
		var entregaAtual *int64
		var atrasoAtual bool
		var expiraEm time.Time
		var valorCobrado int64
		err := tx.QueryRow(ctx, `
			SELECT id, anuncio_id, comprador_conta, status, entrega_id, pago_com_atraso,
			       expira_em, valor_centavos
			  FROM rmt_cobranca WHERE referencia_externa = $1 FOR UPDATE`,
			referenciaExterna).Scan(&venda.CobrancaID, &venda.AnuncioID, &venda.CompradorConta,
			&statusCobranca, &entregaAtual, &atrasoAtual, &expiraEm, &valorCobrado)
		if errors.Is(err, pgx.ErrNoRows) {
			res = CobrancaNaoEncontrada
			return nil
		}
		if err != nil {
			return fmt.Errorf("store: confirmar cobranca ref=%q: %w", referenciaExterna, err)
		}

		// Já resolvida: o aviso repetido encontra a mesma linha e não faz nada.
		// Este é o caminho NORMAL, e não uma anomalia — a processadora repete de
		// propósito.
		if statusCobranca == cobrancaPaga || statusCobranca == cobrancaPagaSemItem {
			if entregaAtual != nil {
				venda.EntregaID = *entregaAtual
			}
			venda.PagoComAtraso = atrasoAtual
			if err := s.completaVenda(ctx, tx, &venda); err != nil {
				return err
			}
			if statusCobranca == cobrancaPaga {
				res = CobrancaJaConfirmada
			} else {
				res = CobrancaPagaSemItem
			}
			return nil
		}

		// O QUE DECIDE É A HORA DO PAGAMENTO, e não a hora em que o aviso chegou
		// nem o estado em que a nossa linha está.
		//
		// A diferença aparece no caso que mais dói: o comprador paga no minuto
		// 4:59, o aviso da processadora atrasa, e quando ele chega a varredura já
		// expirou a cobrança. Decidindo pelo nosso estado, esse pagamento viraria
		// dívida — ele pagou DENTRO do prazo e ficaria sem o item. Decidindo pelo
		// `paid_at`, ele recebe.
		//
		// O campo é o `paid_at` da consulta de transação da SyncPay (V2), que é
		// nulo até o pagamento existir. NÃO é o `updated_at`, que muda por
		// qualquer modificação.
		//
		// Relógio de fora contra relógio nosso é uma comparação imperfeita, e é a
		// melhor disponível: a alternativa é o nosso relógio contra o momento em
		// que a rede entregou o aviso, que é pior de todas as formas.
		// O VALOR TEM DE BATER, e a conferência é aqui dentro porque é aqui que a
		// linha está travada: ler o valor fora da transação e comparar depois
		// deixaria a fresta em que ele muda no meio.
		//
		// Depois da idempotência, de propósito: um aviso repetido de uma cobrança
		// já paga não é reconferido, porque o valor que importava já foi conferido
		// quando ela foi paga.
		if valorObservado > 0 && valorObservado != valorCobrado {
			// GRAVA, e é esse o conserto. Antes isto saía sem escrever nada: o
			// dinheiro tinha entrado, a linha continuava aberta, e a varredura do
			// prazo a vencia e soltava o item. Alguém pagava e não sobrava linha
			// que dissesse isso.
			//
			// A cobrança CONTINUA ABERTA de propósito: ela não se resolveu, e tudo
			// que pergunta "há dinheiro em jogo?" — o encerramento da barraca, a
			// reconciliação, a faxina — segue respondendo sim e segurando o item
			// até uma pessoa olhar.
			if _, err := tx.Exec(ctx, `
				UPDATE rmt_cobranca
				   SET valor_divergente_centavos = $2, paga_em = $3, origem_da_hora = $4
				 WHERE id = $1`, venda.CobrancaID, valorObservado, pagoEm,
				string(origem)); err != nil {
				return fmt.Errorf("store: confirmar cobranca: gravando a divergencia de %d: %w",
					venda.CobrancaID, err)
			}
			res = CobrancaValorDivergente
			return nil
		}
		if pagoEm.IsZero() {
			// Sem hora do pagamento não dá para decidir, e adivinhar aqui é
			// decidir sobre o dinheiro de alguém no escuro. Quem chama trata como
			// falha e tenta de novo; a confirmação repetida é o caminho normal.
			return fmt.Errorf("store: confirmar cobranca %q: sem a hora do pagamento",
				referenciaExterna)
		}
		// DUAS PERGUNTAS DIFERENTES, e conflatá-las foi um erro meu que só apareceu
		// quando a suíte de integração passou a rodar na CI.
		//
		// `foraDoPrazo` é a que DECIDE a entrega: o dinheiro chegou depois do prazo
		// que o vendedor combinou? Só ela pode barrar o item, porque só ela fala do
		// combinado entre as duas pessoas.
		//
		// `pago_com_atraso` é a que se CONTA, e a 0105 escreveu o que ela significa:
		// "a confirmação que chegou DEPOIS do cancelamento ou da expiração". É mais
		// larga de propósito — ela existe para responder "isso virou rotina?", e o
		// caso do comprador que paga em 4:59 com o aviso chegando em 5:10 É um caso
		// desses, mesmo tendo entregado direito.
		//
		// Eu havia reduzido a coluna à pergunta estreita. O resultado: aquele caso
		// deixava de ser contado, e o número que responde "o prazo está errado?"
		// passava a esconder justamente a situação que o prazo causa. A entrega nunca
		// esteve errada; o registro estava.
		foraDoPrazo := pagoEm.After(expiraEm)
		linhaJaFechada := statusCobranca == cobrancaCancelada || statusCobranca == cobrancaExpirada
		venda.PagoComAtraso = foraDoPrazo || linhaJaFechada

		var statusAnuncio int16
		var it itemPayload
		if err := tx.QueryRow(ctx, `
			SELECT vendedor_conta, cargo_slot, status,
			       item_index, eff1, effv1, eff2, effv2, eff3, effv3
			  FROM rmt_anuncio WHERE id = $1 FOR UPDATE`, venda.AnuncioID).
			Scan(&venda.VendedorConta, &venda.CargoSlot, &statusAnuncio,
				&it.ItemIndex, &it.Eff1, &it.EffV1, &it.Eff2, &it.EffV2, &it.Eff3, &it.EffV3); err != nil {
			return fmt.Errorf("store: confirmar cobranca: lendo o anuncio %d: %w", venda.AnuncioID, err)
		}

		entregavel, err := temItemParaEntregar(ctx, tx, venda, statusAnuncio)
		if err != nil {
			return err
		}
		// PAGO FORA DO PRAZO NÃO ENTREGA, mesmo que o item ainda esteja lá.
		//
		// Parece desperdício — o item está disponível, por que não dar? Porque o
		// prazo é a única coisa que o vendedor tem. Ele combinou prender o item por
		// cinco minutos; entregar aos dez seria decidir por ele que a venda ainda
		// valia. E a esta altura o item pode já ter sido solto e vendido a outra
		// pessoa, e a diferença entre "ainda está lá" e "já foi" é o acaso de
		// alguns segundos.
		//
		// O dinheiro volta: o caminho do atrasado é o reembolso, não o item.
		if foraDoPrazo {
			entregavel = false
		}
		if !entregavel {
			// O dinheiro entrou e não há item. Marcar PAGA com entrega_id nulo
			// seria indistinguível de "ainda não entreguei", e a linha sumiria no
			// meio das normais. Este estado existe para ela NÃO sumir.
			if _, err := tx.Exec(ctx, `
				UPDATE rmt_cobranca
				   SET status = $2, paga_em = $4, encerrada_em = now(), pago_com_atraso = $3,
				       origem_da_hora = $5
				 WHERE id = $1`, venda.CobrancaID, cobrancaPagaSemItem, venda.PagoComAtraso,
				pagoEm, string(origem)); err != nil {
				return fmt.Errorf("store: confirmar cobranca: marcando sem item %d: %w", venda.CobrancaID, err)
			}
			res = CobrancaPagaSemItem
			return nil
		}

		carga, err := json.Marshal(it)
		if err != nil {
			return fmt.Errorf("store: confirmar cobranca: montando a entrega: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO delivery_queue (account_id, kind, payload, source)
			VALUES ($1, 'item', $2, $3) RETURNING id`,
			venda.CompradorConta, carga, fmt.Sprintf("rmt_anuncio:%d", venda.AnuncioID)).
			Scan(&venda.EntregaID); err != nil {
			return fmt.Errorf("store: confirmar cobranca: enfileirando a entrega: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE rmt_cobranca
			   SET status = $2, paga_em = $5, encerrada_em = now(),
			       entrega_id = $3, pago_com_atraso = $4, origem_da_hora = $6
			 WHERE id = $1`,
			venda.CobrancaID, cobrancaPaga, venda.EntregaID, venda.PagoComAtraso,
			pagoEm, string(origem)); err != nil {
			return fmt.Errorf("store: confirmar cobranca: marcando paga %d: %w", venda.CobrancaID, err)
		}
		// O anúncio sai da vitrine agora. A MARCA DO ESCROW FICA: ela é o que
		// mantém o item do vendedor intocável até alguém tirá-lo, e tirar é
		// trabalho do laço do tmServer.
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_anuncio SET status = $2, encerrado_em = now() WHERE id = $1`,
			venda.AnuncioID, anuncioVendido); err != nil {
			return fmt.Errorf("store: confirmar cobranca: fechando o anuncio %d: %w", venda.AnuncioID, err)
		}
		res = CobrancaConfirmada
		return nil
	})
	if err != nil {
		return CobrancaNaoEncontrada, VendaRMT{}, err
	}
	return res, venda, nil
}

// temItemParaEntregar responde a única pergunta que decide entre entregar e
// virar dívida.
//
// Duas perguntas e não uma, de propósito: o anúncio tem de estar ATIVO, e a marca
// do escrow tem de continuar apontando para ELE. As duas dizem a mesma coisa
// quando tudo está certo, e é justamente por isso que conferir as duas vale —
// discordarem significa que uma invariante quebrou em algum lugar, e aí o certo
// é NÃO entregar e deixar uma pessoa olhar.
func temItemParaEntregar(ctx context.Context, tx pgx.Tx, venda VendaRMT, statusAnuncio int16) (bool, error) {
	if statusAnuncio != anuncioAtivo {
		return false, nil
	}
	var marcado bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM item
			 WHERE owner_kind = 'account_cargo' AND account_id = $1
			   AND slot = $2 AND rmt_anuncio = $3)`,
		venda.VendedorConta, venda.CargoSlot, venda.AnuncioID).Scan(&marcado); err != nil {
		return false, fmt.Errorf("store: confirmar cobranca: conferindo a marca do anuncio %d: %w",
			venda.AnuncioID, err)
	}
	return marcado, nil
}

// completaVenda preenche o vendedor e o slot numa resposta de aviso repetido.
//
// Quem chama precisa deles mesmo quando nada mudou: é assim que a fila de
// PAGA_SEM_ITEM vira fila de REtentar, e não só de reclamar. Um aviso que chega
// de novo depois de o vendedor finalmente entrar em jogo é a chance de a retirada
// acontecer.
func (s *Store) completaVenda(ctx context.Context, tx pgx.Tx, venda *VendaRMT) error {
	if err := tx.QueryRow(ctx,
		`SELECT vendedor_conta, cargo_slot FROM rmt_anuncio WHERE id = $1`, venda.AnuncioID).
		Scan(&venda.VendedorConta, &venda.CargoSlot); err != nil {
		return fmt.Errorf("store: confirmar cobranca: relendo o anuncio %d: %w", venda.AnuncioID, err)
	}
	return nil
}

// SlotsVendidosPendentes devolve os slots do baú desta conta que ainda seguram
// um item cujo anúncio JÁ FOI VENDIDO.
//
// É a outra ponta da preguiça. A confirmação do pagamento deixa a marca do
// escrow de pé de propósito — enquanto ela está lá o item é intocável, então
// tirá-lo pode esperar o vendedor aparecer. Esta consulta é o que o tmServer
// pergunta quando ele aparece.
//
// REMOVER, e nunca devolver ao dono. É a diferença que importa entre esta marca e
// a de um anúncio cancelado: aquele volta para as mãos do vendedor, este já foi
// pago e entregue a outra pessoa. Devolver aqui seria criar a segunda cópia que o
// escrow inteiro existe para impedir.
func (s *Store) SlotsVendidosPendentes(ctx context.Context, accountID int64) ([]int16, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.slot
		  FROM item i
		  JOIN rmt_anuncio a ON a.id = i.rmt_anuncio
		 WHERE i.owner_kind = 'account_cargo' AND i.account_id = $1
		   AND i.rmt_anuncio <> 0 AND a.status = $2
		 ORDER BY i.slot`, accountID, anuncioVendido)
	if err != nil {
		return nil, fmt.Errorf("store: slots vendidos a=%d: %w", accountID, err)
	}
	defer rows.Close()
	var slots []int16
	for rows.Next() {
		var slot int16
		if err := rows.Scan(&slot); err != nil {
			return nil, fmt.Errorf("store: slots vendidos: %w", err)
		}
		slots = append(slots, slot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: slots vendidos a=%d: %w", accountID, err)
	}
	return slots, nil
}
