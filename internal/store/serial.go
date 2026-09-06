package store

import (
	"context"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Item serials (0033_item_serial): the identity an item never had.
//
// An item is an index plus three effect pairs, and two identical ones are
// indistinguishable — so "which of these two is the copy" had no answer. A
// serial gives it one, and does it in a column the client never sees, so a
// marked item keeps all three of its effects.

// ReserveSerials hands out a block of serials and returns the first one.
//
// A block, not one at a time, because the caller is the game loop: it owns all
// world state alone and never blocks, so it cannot ask the database for a number
// while stamping an item. It reserves a few thousand at boot, spends them from
// memory, and reserves more before running out.
//
// The UPDATE is the whole concurrency story. It takes a row lock, so two servers
// asking at once get two disjoint blocks and neither ever learns the other
// existed. That is why this is a counter row and not a sequence: a sequence
// hands out one number per call, which is precisely what the loop cannot do.
func (s *Store) ReserveSerials(ctx context.Context, quantos int64) (int64, error) {
	if quantos <= 0 || quantos > 1_000_000 {
		return 0, fmt.Errorf("store: reserve serials: bad block size %d", quantos)
	}
	var primeiro int64
	if err := s.pool.QueryRow(ctx, `
		UPDATE item_serial_seq SET proximo = proximo + $1
		RETURNING proximo - $1`, quantos).Scan(&primeiro); err != nil {
		return 0, fmt.Errorf("store: reserve %d serials: %w", quantos, err)
	}
	return primeiro, nil
}

// ListDupes returns the serials carried by more than one item row, newest
// duplicate first, with who holds each copy.
//
// This is the payoff of the whole plan, and the only part of it that produces
// proof rather than suspicion. Everything else answers "something appeared";
// two rows with one serial answer "this is the copy, and here is who has it".
//
// It cannot see a duplicate made before 0033: those items were stamped
// separately on their owners' next save and got different numbers. Marking
// starts from now.
func (s *Store) ListDupes(ctx context.Context, limite int) ([]domain.ItemDup, error) {
	if limite <= 0 || limite > 500 {
		limite = 100
	}
	rows, err := s.pool.Query(ctx, `
		WITH repetidos AS (
			SELECT serial FROM item
			 WHERE serial <> 0
			 GROUP BY serial HAVING count(*) > 1
			 ORDER BY serial DESC
			 LIMIT $1
		)
		SELECT i.serial, i.item_index, i.owner_kind,
		       COALESCE(c.name, ''), COALESCE(a.name, ac.name, ''),
		       i.eff1, i.effv1, i.eff2, i.effv2, i.eff3, i.effv3
		  FROM item i
		  JOIN repetidos r ON r.serial = i.serial
		  LEFT JOIN character c ON c.id = i.character_id
		  LEFT JOIN account a ON a.id = c.account_id
		  LEFT JOIN account ac ON ac.id = i.account_id
		 ORDER BY i.serial DESC, i.id`, limite)
	if err != nil {
		return nil, fmt.Errorf("store: list dupes: %w", err)
	}
	defer rows.Close()

	var out []domain.ItemDup
	for rows.Next() {
		var d domain.ItemDup
		if err := rows.Scan(&d.Serial, &d.Index, &d.Onde, &d.Character, &d.Account,
			&d.Eff[0][0], &d.Eff[0][1], &d.Eff[1][0], &d.Eff[1][1],
			&d.Eff[2][0], &d.Eff[2][1]); err != nil {
			return nil, fmt.Errorf("store: scan dupe row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate dupes: %w", err)
	}
	return out, nil
}

// CountMarked reports how many item rows carry a serial and how many do not.
//
// The screen needs both to be honest: an empty duplicate list means nothing
// until you know whether anything is marked at all. Right after the migration
// everything reads zero, and that is not the same as "no duplicates".
func (s *Store) CountMarked(ctx context.Context) (marcados, semMarca int64, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE serial <> 0), count(*) FILTER (WHERE serial = 0)
		  FROM item`).Scan(&marcados, &semMarca)
	if err != nil {
		return 0, 0, fmt.Errorf("store: count marked items: %w", err)
	}
	return marcados, semMarca, nil
}
