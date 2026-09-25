package store

import (
	"context"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// ListExpRanking returns the persisted character Top EXP projection in ranking
// order. Level >= 1000 entries are skipped, matching the legacy CRanking load.
//
// QUEM NÃO APARECE: conta com cargo (admin ou moderator) e conta marcada como
// `fora_do_ranking`. No dia em que o servidor abriu, a equipe estava no topo da lista do
// site e do bot, e um ranking que mostra quem testou o jogo não mede nada.
//
// SÃO DOIS CRITÉRIOS PORQUE SÃO DOIS PROBLEMAS. O cargo cobre quem é da equipe de
// direito. A marca cobre quem está testando com conta comum e a quem não se quer dar
// poder de GM só para sumir de uma lista.
//
// E NENHUM DOS DOIS É NOME. Depois do wipe de 25/09/2026 os nomes antigos ficaram
// livres, e um jogador de verdade pode ter criado um deles — excluir por nome tiraria
// esse jogador do ranking sem ninguém entender por quê. Uma lista de nomes no código
// também envelhece calada, porque ninguém revisa o que parece estar funcionando.
//
// O MESMO FILTRO VALE NO countExpRanking. Se só a lista filtrasse, a página diria "1 de
// 340" mostrando 337, e o total errado é pior que o nome a mais: ele faz o leitor
// procurar uma página que não existe.
func (s *Store) ListExpRanking(ctx context.Context, limit, offset int) ([]domain.RankingEntry, int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.name, c.class, c.clan, c.guild_id, c.level, c.exp, c.class_master, count(*) OVER()
		  FROM character c
		  JOIN account a ON a.id = c.account_id
		 WHERE c.level < 1000 AND a.role = 'player' AND NOT a.fora_do_ranking
		 ORDER BY
		       CASE c.class_master WHEN 2 THEN 1 WHEN 1 THEN 2 ELSE c.class_master END DESC,
		       c.exp DESC,
		       c.level DESC,
		       c.name ASC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list exp ranking: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.RankingEntry, 0, limit)
	total := 0
	for rows.Next() {
		var e domain.RankingEntry
		if err := rows.Scan(&e.Name, &e.Class, &e.Clan, &e.GuildID, &e.Level, &e.Exp, &e.ClassMaster, &total); err != nil {
			return nil, 0, fmt.Errorf("store: scan exp ranking: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: list exp ranking: %w", err)
	}
	total, err = fallbackTotal(ctx, len(entries), offset, total, s.countExpRanking)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (s *Store) countExpRanking(ctx context.Context) (int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM character c
		  JOIN account a ON a.id = c.account_id
		 WHERE c.level < 1000 AND a.role = 'player' AND NOT a.fora_do_ranking`).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count exp ranking: %w", err)
	}
	return total, nil
}

// fallbackTotal covers a `count(*) OVER()` window function's blind spot: it
// only appears on returned rows, so a page past the last row (empty result,
// non-zero offset) would otherwise report total=0. In that case only, run a
// plain COUNT(*) instead.
func fallbackTotal(ctx context.Context, rowsReturned, offset, windowTotal int, count func(context.Context) (int, error)) (int, error) {
	if rowsReturned != 0 || offset == 0 {
		return windowTotal, nil
	}
	return count(ctx)
}
