package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// O COMEÇO DA COBRANÇA: onde o comprador entra no caminho do dinheiro.
//
// A confirmação (cobranca_rmt.go) já existia e já era idempotente. O que faltava
// era a linha que ela confirma. Sem isto, o único jeito de uma cobrança existir
// era alguém escrever no banco à mão.
//
// TODAS AS RECUSAS DAQUI SÃO PREVISTAS, e por isso voltam no RESULTADO e não como
// erro. Anúncio que acabou de vender, comprador que é o próprio vendedor, outra
// pessoa que abriu o QR meio segundo antes — nada disso é falha do sistema, é o
// sistema funcionando. Erro fica para o que ninguém previu.

// ResultadoAbertura é o que a tentativa de abrir uma cobrança produziu.
type ResultadoAbertura int

const (
	// CobrancaAberta: nasceu agora, e o comprador pode pagar.
	CobrancaAbertaOK ResultadoAbertura = iota
	// CobrancaJaExistia: a MESMA referência externa já tinha uma linha. É a
	// idempotência, e é o caminho normal de um pedido repetido — a tela do
	// comprador recarregou, a rede engasgou, o app tentou de novo.
	CobrancaJaExistia
	// AnuncioNaoDisponivel: o anúncio não está mais ativo. Vendeu para outra
	// pessoa, ou o vendedor fechou a barraca e cancelou.
	AnuncioNaoDisponivel
	// AnuncioComOutraCobranca: já existe uma cobrança aberta contra este anúncio.
	// É a invariante que impede dois compradores de pagarem pelo mesmo item e os
	// dois terem razão — ela mora no índice único da 0105, não aqui.
	AnuncioComOutraCobranca
	// CompradorEOVendedor: comprar de si mesmo. Não há ganho no item, e o custo
	// é a CHAMADA — cada cobrança bate na processadora, que cobra por isso.
	CompradorEOVendedor
	// ItemNaoEstaPreso: o anúncio está ativo mas a marca do escrow não aponta
	// para ele. As duas coisas discordam, e discordância em linha de dinheiro se
	// resolve NÃO cobrando.
	ItemNaoEstaPreso
	// CompradorJaTemCobranca: esta conta já tem uma cobrança aberta, de outro
	// item.
	//
	// É o que impede uma conta de clicar em todas as prateleiras do mercado e
	// prender o estoque inteiro por cinco minutos, de graça e repetidamente. E é
	// também o que mantém a página do site honesta: ela mostra UMA cobrança, e uma
	// segunda nunca poderia ser paga porque o comprador nunca veria o código dela.
	CompradorJaTemCobranca
)

// CobrancaRMT é a cobrança que nasceu, do jeito que quem chamou precisa dela.
type CobrancaRMT struct {
	CobrancaID      int64
	AnuncioID       int64
	VendedorConta   int64
	ValorCentavos   int64
	ExpiraEm        time.Time
	ChavePixDestino string
}

// JanelaPadraoCobranca é quanto tempo o comprador tem para pagar.
//
// QUINZE MINUTOS desde 25/09/2026, e antes eram cinco. A troca é dos dois lados:
// janela curta prende menos o item do vendedor e faz o Pix atrasado — que ainda
// entrega — ser mais comum; janela longa faz o contrário. Quem decide é o volume de
// `pago_com_atraso`, que a 0105 guarda justamente para esta pergunta ter resposta.
//
// Cinco era apertado para alguém pagando Pix de verdade, e um pagamento que cai
// pouco depois do prazo não entrega o item na hora: vai para reembolso, e a pessoa vê
// "venceu" tendo pagado.
//
// ESTE VALOR TEM DE CASAR COM handler.JanelaDeCobranca, do tmServer. Os dois são
// padrões do MESMO prazo, lidos em processos diferentes: o jogo promete o tempo na
// mensagem ao comprador, e é este lado que grava o expira_em. Se discordarem, a
// mensagem mente — e mentir sobre prazo de pagamento é a mentira mais cara que esta
// tela pode contar.
const JanelaPadraoCobranca = 15 * time.Minute

// Método de pagamento (0105). Coluna e nunca parte de nome, para cartão reusar a
// mesma tabela.
const metodoPix int16 = 1

// AbrirCobrancaRMT cria a tentativa de pagamento de um comprador contra um
// anúncio.
//
// Uma transação só, e todas as conferências dentro dela. Perguntar antes e
// inserir depois deixaria a fresta que importa: o anúncio vendido, ou outra
// cobrança aberta, no intervalo entre as duas.
//
// A COLISÃO DE DUAS COBRANÇAS NÃO É TRATADA POR PERGUNTA, e sim pelo índice
// único da 0105 (`rmt_cobranca_uma_aberta_por_anuncio`). Duas pessoas clicando no
// mesmo segundo passam as duas pela conferência e só uma passa pelo índice, e é
// essa diferença que faz a invariante valer de verdade. A pergunta continua
// existindo porque ela dá a RESPOSTA CERTA ao caso comum; o índice dá a resposta
// certa ao caso raro.
//
// DEVOLVE A CHAVE PIX DO VENDEDOR porque é ela que vira o QR. Lida aqui dentro,
// na mesma transação: a chave trocada entre a leitura e a cobrança geraria um QR
// para a conta errada, e o dinheiro cairia no lugar errado sem ninguém errar nada.
func (s *Store) AbrirCobrancaRMT(ctx context.Context, anuncioID, compradorConta int64,
	referenciaExterna string, janela time.Duration,
) (ResultadoAbertura, CobrancaRMT, error) {
	if janela <= 0 {
		janela = JanelaPadraoCobranca
	}
	var res ResultadoAbertura
	var cob CobrancaRMT
	cob.AnuncioID = anuncioID

	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// A idempotência primeiro, e antes de qualquer conferência: um pedido
		// repetido tem de encontrar a MESMA linha mesmo que o anúncio já tenha
		// vendido desde então. Conferir antes faria o repetido ser recusado por
		// um motivo que não é dele.
		var statusExistente int16
		err := tx.QueryRow(ctx, `
			SELECT c.id, c.anuncio_id, c.valor_centavos, c.expira_em, c.status
			  FROM rmt_cobranca c WHERE c.referencia_externa = $1 FOR UPDATE`,
			referenciaExterna).Scan(&cob.CobrancaID, &cob.AnuncioID, &cob.ValorCentavos,
			&cob.ExpiraEm, &statusExistente)
		if err == nil {
			res = CobrancaJaExistia
			return completaDestino(ctx, tx, &cob)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("store: abrir cobranca ref=%q: %w", referenciaExterna, err)
		}

		// O FOR UPDATE AQUI É METADE DO CONSERTO DA CORRIDA com o encerramento
		// da barraca. Travando o anúncio, as duas operações passam a se enfileirar
		// na mesma linha: ou a cobrança nasce antes e o encerramento a enxerga, ou
		// o encerramento passa antes e a leitura abaixo vê o anúncio já fechado. A
		// outra metade está no EncerrarAnunciosRMT, que separa a leitura das
		// cobranças num comando próprio.
		var statusAnuncio int16
		var cargoSlot int16
		var barracaCaiu bool
		if err := tx.QueryRow(ctx, `
			SELECT vendedor_conta, cargo_slot, preco_centavos, status, barraca_caiu
			  FROM rmt_anuncio WHERE id = $1 FOR UPDATE`, anuncioID).
			Scan(&cob.VendedorConta, &cargoSlot, &cob.ValorCentavos, &statusAnuncio,
				&barracaCaiu); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				res = AnuncioNaoDisponivel
				return nil
			}
			return fmt.Errorf("store: abrir cobranca: lendo o anuncio %d: %w", anuncioID, err)
		}
		// Ativo NÃO BASTA. Um anúncio cuja barraca caiu continua ativo de
		// propósito, para o Pix atrasado daquela cobrança ainda encontrar o que
		// entregar — mas ele não está mais à venda para ninguém, e cobrar contra
		// ele venderia uma vitrine que não existe.
		if statusAnuncio != anuncioAtivo || barracaCaiu {
			res = AnuncioNaoDisponivel
			return nil
		}
		if cob.VendedorConta == compradorConta {
			res = CompradorEOVendedor
			return nil
		}
		// ABAIXO DO MÍNIMO NÃO ABRE COBRANÇA, e esta trava existe para os anúncios
		// que JÁ ESTÃO no banco abaixo dele.
		//
		// A trava principal é na montagem da barraca, no jogo, onde o vendedor pode
		// consertar. Mas quando o mínimo nasceu (24/09/2026) já havia anúncio de um
		// centavo gravado, de um teste. Em vez de fechá-los à força — mexer em anúncio
		// de outra pessoa por migração — eles simplesmente não vendem, e morrem
		// sozinhos quando a barraca descer, como todo anúncio.
		//
		// A resposta é a MESMA de indisponível, de propósito: para quem compra, um
		// anúncio que não pode ser vendido e um que não existe mais dão no mesmo, e
		// inventar um motivo novo no contrato obrigaria um handshake com o site por
		// uma situação que vai desaparecer sozinha.
		if cob.ValorCentavos < PrecoMinimoRMTCentavos || cob.ValorCentavos > TetoDaVendaRMTCentavos {
			res = AnuncioNaoDisponivel
			return nil
		}

		// O item TEM de estar preso. Anúncio ativo com o cadeado solto significa
		// que o item pode ter saído por outro caminho, e cobrar aqui produziria
		// dinheiro entrando sem nada para entregar.
		var preso bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM item
				 WHERE owner_kind = 'account_cargo' AND account_id = $1
				   AND slot = $2 AND rmt_anuncio = $3)`,
			cob.VendedorConta, cargoSlot, anuncioID).Scan(&preso); err != nil {
			return fmt.Errorf("store: abrir cobranca: conferindo o cadeado do anuncio %d: %w",
				anuncioID, err)
		}
		if !preso {
			res = ItemNaoEstaPreso
			return nil
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO rmt_cobranca
				(anuncio_id, comprador_conta, referencia_externa, valor_centavos,
				 metodo, status, expira_em)
			VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
			RETURNING id, expira_em`,
			anuncioID, compradorConta, referenciaExterna, cob.ValorCentavos,
			metodoPix, cobrancaAberta, janela.String()).Scan(&cob.CobrancaID, &cob.ExpiraEm)
		if ehConflitoDeIndice(err, "rmt_cobranca_uma_aberta_por_comprador") {
			// A conta já está pagando outra coisa. Recusa prevista, e o índice é
			// quem a garante mesmo com dois cliques no mesmo instante.
			return errCompradorJaTemCobranca
		}
		if ehConflitoDeUnicidade(err) {
			// Outra pessoa abriu o QR primeiro. O índice é quem resolve, e ele
			// resolve mesmo no meio-segundo em que a conferência acima não
			// resolveria.
			//
			// Sai por ERRO SENTINELA e não devolvendo o resultado: a violação de
			// índice aborta a transação no Postgres, e tentar commitá-la em
			// seguida falha com "commit unexpectedly resulted in rollback". A
			// alternativa seria um savepoint antes do INSERT, e não vale: aqui não
			// há mais nada a commitar de qualquer forma. O sentinela é traduzido de
			// volta em recusa prevista lá fora.
			return errOutraCobrancaAberta
		}
		if err != nil {
			return fmt.Errorf("store: abrir cobranca a=%d anuncio=%d: %w",
				compradorConta, anuncioID, err)
		}
		res = CobrancaAbertaOK
		return completaDestino(ctx, tx, &cob)
	})
	if errors.Is(err, errOutraCobrancaAberta) {
		return AnuncioComOutraCobranca, CobrancaRMT{}, nil
	}
	if errors.Is(err, errCompradorJaTemCobranca) {
		return CompradorJaTemCobranca, CobrancaRMT{}, nil
	}
	if err != nil {
		return AnuncioNaoDisponivel, CobrancaRMT{}, err
	}
	return res, cob, nil
}

// errOutraCobrancaAberta carrega a recusa do índice único para fora da
// transação abortada. Nunca sai desta camada: a AbrirCobrancaRMT o traduz em
// AnuncioComOutraCobranca, que é recusa prevista e não falha.
var errOutraCobrancaAberta = errors.New("store: ja existe cobranca aberta para este anuncio")

// errCompradorJaTemCobranca carrega, pelo mesmo caminho, a recusa do índice de uma
// aberta por comprador.
var errCompradorJaTemCobranca = errors.New("store: o comprador ja tem cobranca aberta")

// ehConflitoDeIndice reconhece a violação de um índice único ESPECÍFICO.
//
// Pelo nome do índice e não só pelo código 23505: hoje há dois índices que a mesma
// inserção pode violar — um por anúncio e um por comprador —, e eles dizem coisas
// diferentes ao jogador. "Alguém está pagando esse item" manda esperar por aquele
// item; "você já tem um pagamento aberto" manda terminar o que começou. Tratar os
// dois como um só faria a mensagem mentir metade das vezes.

// completaDestino lê a chave Pix do vendedor, que é o que vira o QR.
// PrecoMinimoRMTCentavos é o menor preço que um item pode ter em dinheiro real.
//
// R$ 1,00, decisão da Hanna de 24/09/2026. UM LUGAR SÓ porque DUAS travas o leem — a
// montagem da barraca, no jogo, e a abertura da cobrança, aqui — e dois números que
// deveriam ser iguais acabam diferentes no dia em que alguém muda um.
//
// POR QUE EXISTE UM MÍNIMO: a processadora cobra taxa por cobrança. Um item de um
// centavo custa mais para vender do que rende, e o repasse ao vendedor sairia
// negativo. Não é regra de gosto, é aritmética.
//
// SUBIU DE R$ 1,00 PARA R$ 5,00 quando a taxa da casa entrou (taxa_rmt.go). Com R$ 0,80
// fixos por venda, R$ 1,00 deixaria o vendedor com R$ 0,15 — e uma venda que entrega
// quinze centavos não é uma venda, é uma reclamação. O TestATaxaNuncaComeAVendaInteira
// é o que prende os dois números juntos: baixar este mínimo sem olhar a taxa quebra
// aquele teste em vez de quebrar o bolso de quem vendeu.
const PrecoMinimoRMTCentavos = 500

// TetoDaVendaRMTCentavos é o maior preço que um item pode ter em dinheiro real.
//
// R$ 500,00, decisão da Hanna. NÃO existia teto nenhum antes, e a falta dele é mais
// perigosa que a falta do mínimo: um preço de R$ 50.000 num anúncio é um erro de
// digitação plausível, e do outro lado dele há um Pix de verdade saindo da conta de
// alguém. O mínimo protege o vendedor de vender de graça; o teto protege o comprador de
// pagar uma fortuna por engano, e a casa de virar caminho de lavagem.
//
// LIDO NOS DOIS LUGARES, como o mínimo: a montagem da barraca, no jogo, e a abertura da
// cobrança. Só na montagem deixaria um cliente remendado passar por cima.
const TetoDaVendaRMTCentavos = 50_000

func completaDestino(ctx context.Context, tx pgx.Tx, cob *CobrancaRMT) error {
	err := tx.QueryRow(ctx, `
		SELECT a.vendedor_conta, r.chave
		  FROM rmt_anuncio a
		  JOIN rmt_recebedor r ON r.account_id = a.vendedor_conta
		 WHERE a.id = $1`, cob.AnuncioID).Scan(&cob.VendedorConta, &cob.ChavePixDestino)
	if errors.Is(err, pgx.ErrNoRows) {
		// Sem chave não há para onde mandar. Não é recusa de abertura — a
		// abertura já aconteceu, ou já existia —, é uma cobrança que ninguém
		// consegue pagar, e quem chamou precisa saber disso pelo campo vazio.
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: abrir cobranca: lendo a chave do anuncio %d: %w", cob.AnuncioID, err)
	}
	return nil
}

// CancelarCobrancaRMT fecha uma cobrança que não vai ser paga.
//
// SEM CHAMADOR EM PRODUÇÃO HOJE, e isso é decisão e não esquecimento.
//
// Ela existia para o logout do comprador, e esse caminho SAIU: cancelar ali não
// soltava o item de ninguém — a marca do escrow segura até o prazo acabar, de
// qualquer jeito — e quebrava a compra de quem fecha o jogo para pagar no celular,
// que é o movimento natural. A cobrança passa a fechar de dois jeitos só: paga ou
// vencida.
//
// Fica porque a staff vai precisar dela: uma cobrança presa por um caso que
// ninguém previu tem de poder ser fechada por uma pessoa. E porque tirar e voltar a
// escrever é mais caro do que manter cinco linhas com o motivo escrito.
//
// AS CONDIÇÕES PARA LIGAR ISTO NUMA TELA, porque quem for fazê-lo não vai ter o
// contexto de hoje:
//
//  1. CANCELAR A NOSSA LINHA NÃO INVALIDA O CÓDIGO NA PROCESSADORA. Não existe rota
//     conhecida para cancelar um cash-in lá. O copia-e-cola continua pagável depois
//     de a linha fechar, e é por isso que o item NÃO pode ser solto antes do
//     expira_em mais a consulta que confirma que não houve pagamento — a mesma
//     regra de toda cobrança. Uma tela que "cancela e libera" criaria exatamente o
//     caso que o resto deste arquivo existe para evitar: dinheiro entrando contra
//     um item que já foi para outra pessoa.
//  2. É AÇÃO DE ADMIN, com auditoria na MESMA transação, no molde do
//     transicaoDaStaff em reembolso_rmt.go. Mexer no dinheiro de alguém sem
//     registro é a mudança que ninguém consegue explicar depois.
//
// Fora isso, nenhum chamador. Se você está lendo isto porque quer chamá-la de
// outro lugar, provavelmente não quer.
//
// SÓ CANCELA O QUE ESTÁ ABERTO, e o WHERE é quem garante — não a ordem em que as
// coisas acontecem. Uma cobrança PAGA que fosse cancelada por um caminho de
// desistência apagaria o registro de um pagamento que existiu; e o cancelamento
// chega tarde por desenho, porque vem de um evento de rede.
//
// CANCELAR NÃO IMPEDE O PIX ATRASADO DE ENTREGAR. A confirmação lê esta linha
// pela referência externa, vê o status CANCELADA, e entrega assim mesmo marcando
// `pago_com_atraso`. É regra da Hanna e está em cobranca_rmt.go: quem pagou
// direito recebe, mesmo tendo pagado tarde.
func (s *Store) CancelarCobrancaRMT(ctx context.Context, referenciaExterna string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET status = $2, encerrada_em = now()
		 WHERE referencia_externa = $1 AND status = $3`,
		referenciaExterna, cobrancaCancelada, cobrancaAberta)
	if err != nil {
		return false, fmt.Errorf("store: cancelar cobranca ref=%q: %w", referenciaExterna, err)
	}
	return tag.RowsAffected() > 0, nil
}

// ExpirarCobrancasRMT fecha as cobranças cujo prazo acabou e que NINGUÉM PODERIA
// TER PAGO.
//
// A expiração é por VARREDURA e não por temporizador, e isso não é preguiça: um
// temporizador por cobrança morre com o processo, e o prazo tem de continuar
// valendo depois de um reinício. A linha guarda `expira_em`; a varredura só lê o
// que o banco já sabe.
//
// SÓ AS SEM IDENTIFIER, e essa condição é a correção de um buraco que devolvia
// dinheiro de gente. Antes, esta varredura vencia qualquer cobrança no prazo, sem
// perguntar nada à processadora — e a reconciliação então soltava o item do
// vendedor. Um Pix pago no último segundo, com o aviso atrasado, virava um
// pagamento sem item: a pessoa pagava, não recebia, e o item já tinha voltado para
// o vendedor.
//
// O identifier nasce junto com o código Pix. Sem ele não há código, e sem código
// ninguém conseguiu pagar — então estas aqui vencem sem se perguntar nada. As
// outras são vencidas pela varredura do webserver, DEPOIS de ela consultar a
// processadora, porque é lá que moram as credenciais da ponte.
//
// Se o webserver estiver fora, as com identifier param de vencer e o item do
// vendedor fica preso além do combinado. É a troca deliberada: item preso se
// conserta, dinheiro entregue a quem não devia não. O tamanho da fila sai no log
// pela CobrancasVencidasSemConferir.
//
// A DIVERGENTE NÃO VENCE. Nela o dinheiro JÁ ENTROU, com o valor errado, e está
// esperando uma pessoa. Vencer aqui soltaria o item de quem recebeu o pagamento —
// o contrário exato do que o prazo existe para fazer.
//
// Devolve os anúncios afetados pelo mesmo motivo da função acima.
func (s *Store) ExpirarCobrancasRMT(ctx context.Context) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE rmt_cobranca SET status = $1, encerrada_em = now()
		 WHERE status = $2 AND expira_em <= now()
		   AND valor_divergente_centavos IS NULL
		   AND identifier_syncpay IS NULL
		RETURNING anuncio_id`, cobrancaExpirada, cobrancaAberta)
	if err != nil {
		return nil, fmt.Errorf("store: expirar cobrancas: %w", err)
	}
	defer rows.Close()
	var anuncios []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: expirar cobrancas: %w", err)
		}
		anuncios = append(anuncios, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: expirar cobrancas: %w", err)
	}
	return anuncios, nil
}

// ehConflitoDeUnicidade reconhece a violação de índice único do Postgres (23505).
//
// Pelo CÓDIGO e nunca pelo texto: o texto muda com a versão e com o idioma do
// servidor, e uma comparação de texto que falha aqui trataria "outra pessoa
// chegou primeiro" como falha de infraestrutura.
func ehConflitoDeIndice(err error, indice string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == indice
}

func ehConflitoDeUnicidade(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
