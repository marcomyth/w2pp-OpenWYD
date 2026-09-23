package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// O FIM DO ANÚNCIO, que é a metade que faltava do escrow.
//
// Até aqui o sistema sabia prender e sabia entregar. O que ele não sabia era
// SOLTAR: um anúncio que não vendeu ficava ativo para sempre, e a marca no slot
// deixava o item intocável para sempre junto. A mensagem que o jogo mostra —
// "cancele o anúncio antes de vendê-lo de novo" — pedia uma coisa que não
// existia.
//
// A REGRA, em uma frase: o anúncio morre com a barraca, mas a marca só sai
// quando não houver dinheiro em jogo sobre ela.
//
// São coisas diferentes de propósito. O anúncio é a VITRINE, e a vitrine é da
// barraca: fechou a barraca, some da vitrine. A marca é o CADEADO, e o cadeado
// responde ao dinheiro: enquanto existir uma cobrança aberta, alguém pode estar
// com um QR na mão, e soltar o item agora seria vendê-lo duas vezes — o Pix
// atrasado chegaria e a confirmação não teria o que entregar.
//
// QUEM TIRA A MARCA DO BAÚ VIVO É SEMPRE O LAÇO, nunca este arquivo. O laço é
// dono do baú; escrever na coluna `item.rmt_anuncio` daqui, com o vendedor em
// jogo, seria escrever por cima do dono e o próximo save do laço traria a marca
// de volta. Estas funções só RESPONDEM o que pode sair; quem apaga é o tmServer.

// AnuncioEncerrado é o destino de um anúncio quando a barraca desce.
type AnuncioEncerrado struct {
	AnuncioID int64
	CargoSlot int16
	// CobrancaAberta diz que havia dinheiro em jogo. O anúncio saiu da vitrine
	// assim mesmo, mas a marca FICA até a cobrança fechar.
	CobrancaAberta bool
}

// EncerrarAnunciosRMT fecha os anúncios de uma barraca que está descendo.
//
// Os que não têm cobrança aberta viram CANCELADO e o slot deles volta livre. Os
// que têm ficam ATIVOS: o comprador está com o QR na mão, o pagamento dele ainda
// vale, e cancelar agora produziria o pior caso de todos — dinheiro entrando
// contra um anúncio morto, que é o estado PAGA_SEM_ITEM, dívida com uma pessoa.
//
// O anúncio sair da vitrine não depende desta linha: a vitrine é lida das
// barracas VIVAS (ver 0105), então ele some de lá sozinho quando a barraca desce.
// O que fica ativo aqui fica ativo só para o pagamento poder chegar.
//
// Devolve o destino de cada anúncio, na ordem dos ids pedidos que existiam. Quem
// chamou usa CobrancaAberta para saber quais marcas pode apagar do baú vivo.
func (s *Store) EncerrarAnunciosRMT(ctx context.Context, ids []int64) ([]AnuncioEncerrado, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var fim []AnuncioEncerrado
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// TRÊS COMANDOS E NÃO UM, e a separação é o conserto de uma corrida que
		// custa dinheiro.
		//
		// Em READ COMMITTED cada COMANDO pega um snapshot. Um comando só que
		// travasse o anúncio e perguntasse por EXISTS se há cobrança responderia a
		// segunda pergunta com o snapshot VELHO: o Postgres reavalia a LINHA
		// TRAVADA contra a versão nova (EvaluatePlanQual), mas não reavalia o que
		// a subconsulta leu em outra tabela.
		//
		// O caso: a cobrança é commitada enquanto o encerramento espera o lock. O
		// anúncio viraria CANCELADO com uma cobrança aberta pendurada, e o Pix
		// desse comprador cairia em PAGA_SEM_ITEM — dívida com quem pagou direito.
		// Está provado em TestEncerrarNaoCancelaAnuncioQueGanhouCobrancaNoMeio: com
		// um comando só, o teste fica vermelho.
		//
		// Primeiro comando: TRAVA. Depois dele, ninguém mais abre cobrança contra
		// estes anúncios — a abertura também trava o anúncio com FOR UPDATE.
		travados, err := travaAnunciosAtivos(ctx, tx, ids)
		if err != nil || len(travados) == 0 {
			return err
		}
		// Segundo comando: LÊ as cobranças, com snapshot NOVO, que já enxerga o
		// que foi commitado enquanto esperávamos o lock.
		comCobranca, err := anunciosComCobrancaAberta(ctx, tx, travados)
		if err != nil {
			return err
		}
		var cancelar, esperar []int64
		for _, a := range travados {
			if comCobranca[a.AnuncioID] {
				a.CobrancaAberta = true
				esperar = append(esperar, a.AnuncioID)
			} else {
				cancelar = append(cancelar, a.AnuncioID)
			}
			fim = append(fim, a)
		}
		// Terceiro: escreve. Os sem cobrança saem; os com cobrança ficam ativos e
		// só ganham a marca de que a barraca caiu, para o Pix atrasado ainda
		// encontrar o que entregar.
		if len(cancelar) > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE rmt_anuncio SET status = $2, encerrado_em = now()
				 WHERE id = ANY($1)`, cancelar, anuncioCancelado); err != nil {
				return fmt.Errorf("store: encerrar anuncios: cancelando %v: %w", cancelar, err)
			}
		}
		if len(esperar) > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE rmt_anuncio SET barraca_caiu = TRUE WHERE id = ANY($1)`, esperar); err != nil {
				return fmt.Errorf("store: encerrar anuncios: marcando %v: %w", esperar, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fim, nil
}

// travaAnunciosAtivos trava e devolve, em ordem de id, os anúncios da lista que
// ainda estão ativos. A ordem é fixa de propósito: dois encerramentos com listas
// que se cruzam travam na mesma sequência e não se abraçam.
func travaAnunciosAtivos(ctx context.Context, tx pgx.Tx, ids []int64) ([]AnuncioEncerrado, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, cargo_slot FROM rmt_anuncio
		 WHERE id = ANY($1) AND status = $2
		 ORDER BY id FOR UPDATE`, ids, anuncioAtivo)
	if err != nil {
		return nil, fmt.Errorf("store: encerrar anuncios: travando %v: %w", ids, err)
	}
	defer rows.Close()
	var out []AnuncioEncerrado
	for rows.Next() {
		var a AnuncioEncerrado
		if err := rows.Scan(&a.AnuncioID, &a.CargoSlot); err != nil {
			return nil, fmt.Errorf("store: encerrar anuncios: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: encerrar anuncios: travando %v: %w", ids, err)
	}
	return out, nil
}

// anunciosComCobrancaAberta pergunta, num comando PRÓPRIO, quais dos anúncios
// travados têm dinheiro em jogo. O comando próprio é o ponto: é o snapshot novo
// dele que enxerga a cobrança commitada enquanto o lock era esperado.
func anunciosComCobrancaAberta(ctx context.Context, tx pgx.Tx, travados []AnuncioEncerrado) (map[int64]bool, error) {
	ids := make([]int64, 0, len(travados))
	for _, a := range travados {
		ids = append(ids, a.AnuncioID)
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT anuncio_id FROM rmt_cobranca
		 WHERE anuncio_id = ANY($1) AND status = $2`, ids, cobrancaAberta)
	if err != nil {
		return nil, fmt.Errorf("store: encerrar anuncios: lendo as cobrancas de %v: %w", ids, err)
	}
	defer rows.Close()
	com := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: encerrar anuncios: %w", err)
		}
		com[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: encerrar anuncios: lendo as cobrancas: %w", err)
	}
	return com, nil
}
