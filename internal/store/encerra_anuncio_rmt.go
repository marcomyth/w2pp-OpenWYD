package store

import (
	"context"
	"fmt"
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
	rows, err := s.pool.Query(ctx, `
		WITH alvo AS (
			SELECT a.id, a.cargo_slot,
			       EXISTS(SELECT 1 FROM rmt_cobranca c
			               WHERE c.anuncio_id = a.id AND c.status = $3) AS tem_cobranca
			  FROM rmt_anuncio a
			 WHERE a.id = ANY($1) AND a.status = $4
			 ORDER BY a.id
			   FOR UPDATE
		), fechados AS (
			UPDATE rmt_anuncio SET status = $2, encerrado_em = now()
			 WHERE id IN (SELECT id FROM alvo WHERE NOT tem_cobranca)
		), esperando AS (
			-- Com cobrança aberta o anúncio FICA ativo, para o Pix atrasado ainda
			-- encontrar o que entregar. A marca de que a barraca caiu é o que
			-- permite soltar o cadeado depois, quando a cobrança fechar.
			UPDATE rmt_anuncio SET barraca_caiu = TRUE
			 WHERE id IN (SELECT id FROM alvo WHERE tem_cobranca)
		)
		SELECT id, cargo_slot, tem_cobranca FROM alvo ORDER BY id`,
		ids, anuncioCancelado, cobrancaAberta, anuncioAtivo)
	if err != nil {
		return nil, fmt.Errorf("store: encerrar anuncios %v: %w", ids, err)
	}
	defer rows.Close()
	var fim []AnuncioEncerrado
	for rows.Next() {
		var a AnuncioEncerrado
		if err := rows.Scan(&a.AnuncioID, &a.CargoSlot, &a.CobrancaAberta); err != nil {
			return nil, fmt.Errorf("store: encerrar anuncios: %w", err)
		}
		fim = append(fim, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: encerrar anuncios %v: %w", ids, err)
	}
	return fim, nil
}

// SlotsDeEscrowMorto devolve os slots desta conta cuja marca não segura mais
// nada: o anúncio não está ativo, não vendeu, e não há cobrança aberta.
//
// É a faxina, e ela existe porque a retirada da marca é PREGUIÇOSA por desenho.
// Três buracos caem aqui, e nenhum deles tem outro conserto:
//
//  1. O anúncio foi cancelado com o vendedor fora do jogo. Ninguém estava no laço
//     para apagar a marca do baú vivo, porque não havia baú vivo.
//  2. A cobrança expirou ou foi cancelada depois de a barraca já ter descido. O
//     anúncio ficou ATIVO esperando o pagamento — de propósito, para o Pix
//     atrasado ainda poder chegar — e é a coluna `barraca_caiu` (0111) que conta
//     essa história. Ativo com barraca de pé: o cadeado fica. Ativo sem barraca e
//     sem cobrança: o cadeado sai.
//  3. O servidor caiu entre criar o anúncio e gravar a marca — ou entre cancelar
//     e salvar. Sobra marca sem anúncio vivo, ou anúncio vivo sem marca.
//
// NÃO inclui o anúncio VENDIDO: aquele slot também precisa ser limpo, mas o item
// vai embora e não volta para o dono, e misturar as duas listas seria confundir
// "solte" com "entregue". Quem cuida do vendido é o SlotsVendidosPendentes.
//
// Marca apontando para anúncio que NÃO EXISTE também entra: é lixo de qualquer
// jeito, e deixá-lo de fora significaria um item preso sem nada que o explique.
func (s *Store) SlotsDeEscrowMorto(ctx context.Context, accountID int64) ([]int16, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.slot
		  FROM item i
		  LEFT JOIN rmt_anuncio a ON a.id = i.rmt_anuncio
		 WHERE i.owner_kind = 'account_cargo' AND i.account_id = $1
		   AND i.rmt_anuncio <> 0
		   AND (a.id IS NULL OR a.status = $2 OR (a.status = $4 AND a.barraca_caiu))
		   AND NOT EXISTS (SELECT 1 FROM rmt_cobranca c
		                    WHERE c.anuncio_id = i.rmt_anuncio AND c.status = $3)
		 ORDER BY i.slot`, accountID, anuncioCancelado, cobrancaAberta, anuncioAtivo)
	if err != nil {
		return nil, fmt.Errorf("store: slots de escrow morto a=%d: %w", accountID, err)
	}
	defer rows.Close()
	var slots []int16
	for rows.Next() {
		var slot int16
		if err := rows.Scan(&slot); err != nil {
			return nil, fmt.Errorf("store: slots de escrow morto: %w", err)
		}
		slots = append(slots, slot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: slots de escrow morto a=%d: %w", accountID, err)
	}
	return slots, nil
}
