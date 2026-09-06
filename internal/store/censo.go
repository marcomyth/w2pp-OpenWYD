package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Item census (0032_item_census): one count per item index and refine level per
// day, so that a jump can be seen at all.
//
// An item in WYD has no identity — it is an index plus three effect pairs, and
// two identical ones are indistinguishable. "Is this one a copy?" has no answer
// here. "How many existed yesterday, how many exist today" does.

// censoAgrupado is the count itself. The refine level lives in whichever of the
// three effect pairs happens to hold EF_SANC, so all three are searched.
const censoAgrupado = `
	SELECT item_index,
	       COALESCE(CASE WHEN eff1 = $1 THEN effv1 END,
	                CASE WHEN eff2 = $1 THEN effv2 END,
	                CASE WHEN eff3 = $1 THEN effv3 END, 0) AS sanc,
	       count(*),
	       count(*) FILTER (WHERE owner_kind = 'char_equip'),
	       count(*) FILTER (WHERE owner_kind = 'char_carry'),
	       count(*) FILTER (WHERE owner_kind = 'account_cargo')
	  FROM item
	 WHERE item_index > 0
	 GROUP BY 1, 2`

const censoMetaSelect = `SELECT dia, contado_em, unidades, variedades FROM item_census_meta`

// RecordCensus takes today's snapshot, unless it was already taken.
//
// The "unless" is the whole contract: the caller runs this every few hours
// rather than betting on a process staying alive for exactly 24, and the second
// call of the day has to be free. item_census_meta is what answers "already
// done" — counting rows in item_census would answer "no" forever on an empty
// world.
//
// The second return says whether it counted now. False with no error means
// today was already photographed, which is the ordinary case, not a problem.
func (s *Store) RecordCensus(ctx context.Context) (domain.CensusRun, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CensusRun{}, false, fmt.Errorf("store: begin census: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ja bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM item_census_meta WHERE dia = current_date)`).Scan(&ja); err != nil {
		return domain.CensusRun{}, false, fmt.Errorf("store: check census day: %w", err)
	}
	if ja {
		run, err := censoDoDia(ctx, tx)
		return run, false, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO item_census (dia, item_index, sanc, unidades, equipados, mochila, bau)
		SELECT current_date, * FROM (`+censoAgrupado+`) c
		ON CONFLICT (dia, item_index, sanc) DO NOTHING`, domain.EffSanc); err != nil {
		return domain.CensusRun{}, false, fmt.Errorf("store: count items: %w", err)
	}

	// The meta totals come from the census rows, not from a second pass over
	// item: a count taken twice is a count that can disagree with itself.
	var run domain.CensusRun
	err = tx.QueryRow(ctx, `
		INSERT INTO item_census_meta (dia, unidades, variedades)
		SELECT current_date, COALESCE(sum(unidades), 0), count(*)
		  FROM item_census WHERE dia = current_date
		ON CONFLICT (dia) DO NOTHING
		RETURNING dia, contado_em, unidades, variedades`).
		Scan(&run.Day, &run.CountedAt, &run.Units, &run.Kinds)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Another server got there in between. Its photo is as good as this one.
		if run, err = censoDoDia(ctx, tx); err != nil {
			return domain.CensusRun{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.CensusRun{}, false, fmt.Errorf("store: commit census: %w", err)
		}
		return run, false, nil
	case err != nil:
		return domain.CensusRun{}, false, fmt.Errorf("store: record census meta: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.CensusRun{}, false, fmt.Errorf("store: commit census: %w", err)
	}
	return run, true, nil
}

func censoDoDia(ctx context.Context, tx pgx.Tx) (domain.CensusRun, error) {
	var run domain.CensusRun
	err := tx.QueryRow(ctx, censoMetaSelect+` WHERE dia = current_date`).
		Scan(&run.Day, &run.CountedAt, &run.Units, &run.Kinds)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CensusRun{}, nil
	}
	if err != nil {
		return domain.CensusRun{}, fmt.Errorf("store: read census day: %w", err)
	}
	return run, nil
}

// CensusQuery asks what moved over a window.
type CensusQuery struct {
	// Dias is how far back to compare. With less history than that, the
	// comparison falls back to the oldest snapshot there is, and
	// CensusCompare.De says which day that was.
	Dias int
	// Subiu orders by growth; false orders by loss, which is the other failure
	// mode — a trade interrupted in the wrong order destroys an item instead of
	// copying it.
	Subiu bool
	// SoRefinado drops the refine-0 rows, where the noise of ordinary farming
	// lives.
	SoRefinado bool
	Limit      int
}

// CensusGrowth compares the newest snapshot against one Dias older.
func (s *Store) CensusGrowth(ctx context.Context, q CensusQuery) (domain.CensusCompare, error) {
	if q.Dias <= 0 {
		q.Dias = 7
	}
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}

	var ate, de time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT u.d, COALESCE(r.d, m.d)
		  FROM (SELECT max(dia) AS d FROM item_census_meta) u
		  LEFT JOIN LATERAL (
		        SELECT max(dia) AS d FROM item_census_meta WHERE dia <= u.d - $1::int
		  ) r ON true
		  CROSS JOIN (SELECT min(dia) AS d FROM item_census_meta) m
		 WHERE u.d IS NOT NULL`, q.Dias).Scan(&ate, &de)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CensusCompare{}, nil // nenhuma foto ainda
	}
	if err != nil {
		return domain.CensusCompare{}, fmt.Errorf("store: census range: %w", err)
	}

	var cmp domain.CensusCompare
	if cmp.Ate, err = s.censoDe(ctx, ate); err != nil {
		return domain.CensusCompare{}, err
	}
	if cmp.De, err = s.censoDe(ctx, de); err != nil {
		return domain.CensusCompare{}, err
	}

	// FULL JOIN, not LEFT: an item that vanished between the two days has no row
	// on the newer side, and dropping it would hide the loss half of the answer.
	ordem := "DESC"
	if !q.Subiu {
		ordem = "ASC"
	}
	refinado := ""
	if q.SoRefinado {
		refinado = " AND COALESCE(h.sanc, a.sanc) > 0"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(h.item_index, a.item_index), COALESCE(h.sanc, a.sanc),
		       COALESCE(h.unidades, 0), COALESCE(a.unidades, 0),
		       COALESCE(h.equipados, 0), COALESCE(h.mochila, 0), COALESCE(h.bau, 0)
		  FROM (SELECT * FROM item_census WHERE dia = $1) h
		  FULL JOIN (SELECT * FROM item_census WHERE dia = $2) a
		    ON a.item_index = h.item_index AND a.sanc = h.sanc
		 WHERE COALESCE(h.unidades, 0) <> COALESCE(a.unidades, 0)`+refinado+`
		 ORDER BY COALESCE(h.unidades, 0) - COALESCE(a.unidades, 0) `+ordem+`,
		          COALESCE(h.sanc, a.sanc) DESC
		 LIMIT $3`, ate, de, q.Limit)
	if err != nil {
		return domain.CensusCompare{}, fmt.Errorf("store: census growth: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c domain.ItemCensus
		if err := rows.Scan(&c.Index, &c.Sanc, &c.Units, &c.Was,
			&c.Equipped, &c.Carried, &c.Stored); err != nil {
			return domain.CensusCompare{}, fmt.Errorf("store: scan census row: %w", err)
		}
		c.Delta = c.Units - c.Was
		cmp.Linha = append(cmp.Linha, c)
	}
	if err := rows.Err(); err != nil {
		return domain.CensusCompare{}, fmt.Errorf("store: iterate census: %w", err)
	}
	return cmp, nil
}

// CensusHistory is one item and refine level over time, newest first.
//
// The trend is the only reading here that means much: a single day's jump can
// be a dozen people logging out at once, since nothing reaches the database
// until logout.
func (s *Store) CensusHistory(ctx context.Context, index int32, sanc int16, dias int) ([]domain.CensusPoint, error) {
	if dias <= 0 || dias > 365 {
		dias = 60
	}
	rows, err := s.pool.Query(ctx, `
		SELECT dia, unidades, equipados, mochila, bau
		  FROM item_census
		 WHERE item_index = $1 AND sanc = $2
		   AND dia > current_date - $3::int
		 ORDER BY dia DESC`, index, sanc, dias)
	if err != nil {
		return nil, fmt.Errorf("store: census history: %w", err)
	}
	defer rows.Close()

	var out []domain.CensusPoint
	for rows.Next() {
		var p domain.CensusPoint
		if err := rows.Scan(&p.Day, &p.Units, &p.Equipped, &p.Carried, &p.Stored); err != nil {
			return nil, fmt.Errorf("store: scan census history: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate census history: %w", err)
	}
	return out, nil
}

func (s *Store) censoDe(ctx context.Context, dia time.Time) (domain.CensusRun, error) {
	var run domain.CensusRun
	err := s.pool.QueryRow(ctx, censoMetaSelect+` WHERE dia = $1`, dia).
		Scan(&run.Day, &run.CountedAt, &run.Units, &run.Kinds)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CensusRun{}, nil
	}
	if err != nil {
		return domain.CensusRun{}, fmt.Errorf("store: read census %s: %w", dia.Format("2006-01-02"), err)
	}
	return run, nil
}
