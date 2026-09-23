package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// A RECONCILIAÇÃO DO ESCROW: o que conserta tudo o que ficou pelo caminho.
//
// O encerramento da barraca cuida do caso feliz — o vendedor está em jogo, a
// barraca desce, o anúncio acaba. Este arquivo cuida dos outros, que são a
// maioria, e todos têm a mesma forma: alguma metade do par anúncio/cadeado ficou
// de pé sem a outra.
//
// O GANCHO É O LOGIN DO VENDEDOR, e a razão é uma frase: quem está entrando NÃO
// TEM BARRACA DE PÉ, por definição. Então todo anúncio ativo dele é, neste
// instante, um anúncio sem vitrine — e um anúncio sem vitrine ou acaba, ou está
// esperando um Pix. Não há terceiro caso, e é isso que torna a varredura segura.
//
// TRÊS BURACOS, e nenhum deles tinha conserto antes:
//
//  1. ANÚNCIO ATIVO ÓRFÃO PRENDENDO O SLOT. Soltar só o cadeado não bastava: o
//     anúncio continuava ATIVO, e o índice de um ativo por slot recusava todo
//     anúncio novo naquele slot. O jogador veria "não deu para montar a barraca"
//     para sempre, sem nada no jogo que explicasse por quê.
//  2. ANÚNCIO ATIVO SEM MARCA. O servidor caiu entre criar o anúncio e salvar o
//     baú. A faxina antiga não o via, porque ela partia do ITEM marcado e aqui
//     não há item marcado. Esta parte do banco, e por isso o enxerga.
//  3. COBRANÇA QUE FECHOU DEPOIS DA BARRACA. O anúncio ficou ativo esperando um
//     Pix que não veio. Agora ninguém mais vai pagar, e ele pode acabar.

// ReconciliarEscrowRMT põe em dia o escrow de uma conta e devolve os slots cujo
// cadeado já não segura nada.
//
// Roda no login do vendedor, numa transação só. A ordem importa: primeiro os
// anúncios acabam, depois os cadeados são listados — listar antes deixaria de
// fora justamente os que acabaram de morrer nesta mesma passada.
//
// Quem APAGA a marca do baú vivo continua sendo o laço. Esta função só responde
// quais podem sair; escrever na coluna daqui, com o vendedor em jogo, seria
// escrever por cima do dono do baú.
func (s *Store) ReconciliarEscrowRMT(ctx context.Context, accountID int64) ([]int16, error) {
	var slots []int16
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := encerraAnunciosSemBarraca(ctx, tx, accountID); err != nil {
			return err
		}
		var err error
		slots, err = cadeadosMortos(ctx, tx, accountID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return slots, nil
}

// encerraAnunciosSemBarraca é o encerramento da barraca aplicado a quem não tem
// barraca nenhuma.
//
// Mesma separação em comandos do EncerrarAnunciosRMT, e pelo mesmo motivo: em
// READ COMMITTED, ler as cobranças no MESMO comando que trava os anúncios usa o
// snapshot velho e não enxerga a cobrança commitada enquanto o lock era esperado.
func encerraAnunciosSemBarraca(ctx context.Context, tx pgx.Tx, accountID int64) error {
	rows, err := tx.Query(ctx, `
		SELECT id FROM rmt_anuncio
		 WHERE vendedor_conta = $1 AND status = $2
		 ORDER BY id FOR UPDATE`, accountID, anuncioAtivo)
	if err != nil {
		return fmt.Errorf("store: reconciliar escrow a=%d: travando os anuncios: %w", accountID, err)
	}
	var ativos []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("store: reconciliar escrow a=%d: %w", accountID, err)
		}
		ativos = append(ativos, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: reconciliar escrow a=%d: %w", accountID, err)
	}
	if len(ativos) == 0 {
		return nil
	}

	com, err := cobrancasAbertasDe(ctx, tx, ativos)
	if err != nil {
		return fmt.Errorf("store: reconciliar escrow a=%d: %w", accountID, err)
	}
	var cancelar, esperar []int64
	for _, id := range ativos {
		if com[id] {
			esperar = append(esperar, id)
		} else {
			cancelar = append(cancelar, id)
		}
	}
	if len(cancelar) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_anuncio SET status = $2, encerrado_em = now()
			 WHERE id = ANY($1)`, cancelar, anuncioCancelado); err != nil {
			return fmt.Errorf("store: reconciliar escrow: cancelando %v: %w", cancelar, err)
		}
	}
	// Com cobrança aberta o anúncio fica de pé, e só registra que não há barraca.
	// Ele volta a esta mesma varredura no próximo login, quando a cobrança já
	// terá fechado de um jeito ou de outro.
	if len(esperar) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_anuncio SET barraca_caiu = TRUE
			 WHERE id = ANY($1) AND NOT barraca_caiu`, esperar); err != nil {
			return fmt.Errorf("store: reconciliar escrow: marcando %v: %w", esperar, err)
		}
	}
	return nil
}

// cobrancasAbertasDe responde, num comando PRÓPRIO, quais anúncios têm dinheiro
// em jogo. O comando próprio é o ponto — ver a nota em encerraAnunciosSemBarraca.
func cobrancasAbertasDe(ctx context.Context, tx pgx.Tx, ids []int64) (map[int64]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT anuncio_id FROM rmt_cobranca
		 WHERE anuncio_id = ANY($1) AND status = $2`, ids, cobrancaAberta)
	if err != nil {
		return nil, fmt.Errorf("lendo as cobrancas de %v: %w", ids, err)
	}
	defer rows.Close()
	com := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		com[id] = true
	}
	return com, rows.Err()
}

// cadeadosMortos lista os slots cuja marca aponta para um anúncio que já não
// segura nada: cancelado, ou inexistente.
//
// Depois do encerramento acima, "ativo" já significa "esperando um Pix", então
// não há mais o caso ativo-mas-morto para tratar aqui. O VENDIDO continua de
// fora, e essa é a distinção mais cara de todo o escrow: a faxina DEVOLVE o item
// ao dono, e o vendido tem de ser RETIRADO, porque já foi pago e entregue a
// outra pessoa. Misturá-los criaria a segunda cópia que o escrow existe para
// impedir.
func cadeadosMortos(ctx context.Context, tx pgx.Tx, accountID int64) ([]int16, error) {
	rows, err := tx.Query(ctx, `
		SELECT i.slot
		  FROM item i
		  LEFT JOIN rmt_anuncio a ON a.id = i.rmt_anuncio
		 WHERE i.owner_kind = 'account_cargo' AND i.account_id = $1
		   AND i.rmt_anuncio <> 0
		   AND (a.id IS NULL OR a.status = $2)
		 ORDER BY i.slot`, accountID, anuncioCancelado)
	if err != nil {
		return nil, fmt.Errorf("store: reconciliar escrow a=%d: lendo os cadeados: %w", accountID, err)
	}
	defer rows.Close()
	var slots []int16
	for rows.Next() {
		var slot int16
		if err := rows.Scan(&slot); err != nil {
			return nil, fmt.Errorf("store: reconciliar escrow a=%d: %w", accountID, err)
		}
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

// ReconciliarEscrowNoBoot fecha os anúncios que sobreviveram a uma queda do
// servidor, para TODAS as contas de uma vez.
//
// No boot ninguém está em jogo, então nenhuma barraca existe — a mesma frase que
// torna a varredura do login segura vale aqui para o mundo inteiro, e vale com
// mais força.
//
// Sem isto, uma queda com barracas de pé deixaria todo aquele estoque anunciado
// para sempre: vitrine vazia (as barracas morreram com o processo), anúncios
// ativos no banco, e os slots presos pelo índice de um ativo por slot. Os donos
// só descobririam ao tentar montar de novo.
//
// Não devolve slots: não há baú carregado para soltar. Os cadeados saem no login
// de cada um, pela ReconciliarEscrowRMT.
func (s *Store) ReconciliarEscrowNoBoot(ctx context.Context) (cancelados, esperando int, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE rmt_anuncio a SET barraca_caiu = TRUE
			 WHERE a.status = $1 AND NOT a.barraca_caiu
			   AND EXISTS (SELECT 1 FROM rmt_cobranca c
			                WHERE c.anuncio_id = a.id AND c.status = $2)`,
			anuncioAtivo, cobrancaAberta)
		if err != nil {
			return fmt.Errorf("store: reconciliar escrow no boot: marcando: %w", err)
		}
		esperando = int(tag.RowsAffected())

		tag, err = tx.Exec(ctx, `
			UPDATE rmt_anuncio a SET status = $3, encerrado_em = now()
			 WHERE a.status = $1
			   AND NOT EXISTS (SELECT 1 FROM rmt_cobranca c
			                    WHERE c.anuncio_id = a.id AND c.status = $2)`,
			anuncioAtivo, cobrancaAberta, anuncioCancelado)
		if err != nil {
			return fmt.Errorf("store: reconciliar escrow no boot: cancelando: %w", err)
		}
		cancelados = int(tag.RowsAffected())
		return nil
	})
	return cancelados, esperando, err
}
