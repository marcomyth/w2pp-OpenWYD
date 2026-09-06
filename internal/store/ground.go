package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Ground log persistence (0031_ground_log). The tmServer writes one row per drop
// and per pickup, off the game loop; the panel reads them back when somebody
// reports that an item went to the wrong person.
//
// Best-effort like the trade log: the item already moved in the world, and
// failing to write about it must not undo it. The caller logs and moves on.

// RecordGround stores one drop or pickup.
func (s *Store) RecordGround(ctx context.Context, g domain.GroundEvent) error {
	acao := strings.TrimSpace(string(g.Acao))
	if acao != string(domain.GroundLargou) && acao != string(domain.GroundPegou) {
		return fmt.Errorf("store: record ground: unknown action %q", acao)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ground_log
			(expira_em, acao, conta_id, char_nome, item_index,
			 eff1, effv1, eff2, effv2, eff3, effv3, pos_x, pos_y, chao_id)
		VALUES (now() + make_interval(days => $1), $2, $3, $4, $5,
		        $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		domain.GroundRetentionDays, acao,
		// nullableID (npc.go) turns a zero id into SQL NULL: 0 is not an account.
		nullableID(g.AccountID), g.Character, g.Item.Index,
		g.Item.Eff[0][0], g.Item.Eff[0][1], g.Item.Eff[1][0], g.Item.Eff[1][1],
		g.Item.Eff[2][0], g.Item.Eff[2][1], g.X, g.Y, g.GroundID)
	if err != nil {
		return fmt.Errorf("store: record ground %s by %q: %w", acao, g.Character, err)
	}
	return nil
}

// GroundQuery filters the ground log. An empty Char matches everything, which is
// what browsing recent activity wants.
type GroundQuery struct {
	Char  string
	Limit int
}

// ListGround returns ground events, newest first.
//
// It deletes expired rows before reading, the same way the report queue does and
// for a different reason: this table is not about privacy, it is about volume.
// Dropping things is constant, and without the sweep it would grow forever
// holding discarded potions from two years ago.
func (s *Store) ListGround(ctx context.Context, q GroundQuery) ([]domain.GroundEvent, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM ground_log WHERE expira_em <= now()`); err != nil {
		return nil, fmt.Errorf("store: purge expired ground log: %w", err)
	}

	// One statement for both cases: with an empty name the clause is trivially
	// true and the planner falls back to the timestamp index, which is exactly
	// the browse-recent path.
	rows, err := s.pool.Query(ctx, `
		SELECT id, ocorrido_em, acao, conta_id, char_nome, item_index,
		       eff1, effv1, eff2, effv2, eff3, effv3, pos_x, pos_y, chao_id
		  FROM ground_log
		 WHERE expira_em > now()
		   AND ($1 = '' OR char_nome = $1)
		 ORDER BY ocorrido_em DESC, id DESC
		 LIMIT $2`, strings.TrimSpace(q.Char), q.Limit)
	if err != nil {
		return nil, fmt.Errorf("store: list ground log: %w", err)
	}
	defer rows.Close()

	var out []domain.GroundEvent
	for rows.Next() {
		var g domain.GroundEvent
		var contaID *int64
		var acao string
		if err := rows.Scan(&g.ID, &g.At, &acao, &contaID, &g.Character, &g.Item.Index,
			&g.Item.Eff[0][0], &g.Item.Eff[0][1], &g.Item.Eff[1][0], &g.Item.Eff[1][1],
			&g.Item.Eff[2][0], &g.Item.Eff[2][1], &g.X, &g.Y, &g.GroundID); err != nil {
			return nil, fmt.Errorf("store: scan ground row: %w", err)
		}
		g.Acao = domain.GroundAcao(acao)
		if contaID != nil {
			g.AccountID = *contaID
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate ground log: %w", err)
	}
	return out, nil
}
