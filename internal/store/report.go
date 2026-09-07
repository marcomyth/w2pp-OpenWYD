package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Player report persistence (0028_player_report). The tmServer writes one row
// per /reportar, off the game loop; the admin panel reads the queue.
//
// Writes are best-effort like the trade log: the player was told their report
// went in, and a slow Postgres must not turn that into an error on their screen.
// The caller logs a failure rather than retrying.

// RecordReport stores one report, clamping what a crafted packet could inflate.
//
// The clamps are here rather than only at the game handler because this is the
// last place before the database: a second caller added later would otherwise
// have to remember them.
func (s *Store) RecordReport(ctx context.Context, r domain.PlayerReport) error {
	texto := strings.TrimSpace(r.Text)
	if texto == "" {
		return fmt.Errorf("store: record report from %q: empty text", r.Character)
	}
	if len(texto) > domain.MaxReportText {
		texto = texto[:domain.MaxReportText]
	}
	perto := r.Nearby
	if len(perto) > domain.MaxReportNearby {
		perto = perto[:domain.MaxReportNearby]
	}
	if perto == nil {
		perto = []string{}
	}
	porPerto, err := json.Marshal(perto)
	if err != nil {
		return fmt.Errorf("store: encode nearby: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO player_report
			(expira_em, conta_id, conta, char_nome, char_nivel, texto, pos_x, pos_y, por_perto)
		VALUES (now() + make_interval(days => $1), $2, $3, $4, $5, $6, $7, $8, $9)`,
		domain.ReportRetentionDays,
		// nullableID (npc.go) turns a zero id into SQL NULL: 0 is not an account.
		nullableID(r.AccountID), r.Account, r.Character, r.Level, texto, r.X, r.Y, porPerto)
	if err != nil {
		return fmt.Errorf("store: record report from %q: %w", r.Character, err)
	}
	return nil
}

// ReportQuery filters the report queue.
type ReportQuery struct {
	// SoAbertos limits the list to reports nobody has handled — the working
	// queue, which is what the page opens on.
	SoAbertos bool
	// AccountID limits the list to one account's reports; 0 means every account.
	AccountID int64
	Limit     int
	// Offset skips rows so the panel can turn the page. A queue longer than one
	// screen still has to be worked to the end.
	Offset int
}

// ListReports returns reports, oldest-open first.
//
// It deletes expired rows before reading. The table holds text a person wrote
// and the names of people who happened to be standing nearby, and the schema
// promises those go away after domain.ReportRetentionDays — a promise nothing
// else in this project keeps, because there is no periodic job to hang it on.
// Doing it here is honest about what it guarantees: nothing expired is ever
// shown, and it leaves the database the next time somebody opens the page.
//
// The delete is bounded by the expiry index and runs on a page a handful of
// people open a handful of times a day, so it is not a write hidden on a hot
// read path.
func (s *Store) ListReports(ctx context.Context, q ReportQuery) ([]domain.PlayerReport, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM player_report WHERE expira_em <= now()`); err != nil {
		// Not fatal: failing to purge must not also hide the queue. The rows that
		// survived are still filtered out of the read below.
		return nil, fmt.Errorf("store: purge expired reports: %w", err)
	}

	// Open reports first and oldest first inside that: whoever waited longest is
	// who should be seen next. Handled ones follow, newest first, because there
	// the question is "what happened recently".
	rows, err := s.pool.Query(ctx, `
		SELECT id, criado_em, expira_em, conta_id, conta, char_nome, char_nivel,
		       texto, pos_x, pos_y, por_perto, tratado_em, tratado_por
		  FROM player_report
		 WHERE expira_em > now()
		   AND (NOT $1 OR tratado_em IS NULL)
		   AND ($2 = 0 OR conta_id = $2)
		 ORDER BY (tratado_em IS NULL) DESC,
		          CASE WHEN tratado_em IS NULL THEN criado_em END ASC,
		          criado_em DESC
		 LIMIT $3 OFFSET $4`, q.SoAbertos, q.AccountID, q.Limit, max(q.Offset, 0))
	if err != nil {
		return nil, fmt.Errorf("store: list reports: %w", err)
	}
	defer rows.Close()

	var out []domain.PlayerReport
	for rows.Next() {
		var r domain.PlayerReport
		var contaID, tratadoPor *int64
		var porPerto []byte
		if err := rows.Scan(&r.ID, &r.At, &r.ExpiresAt, &contaID, &r.Account,
			&r.Character, &r.Level, &r.Text, &r.X, &r.Y, &porPerto,
			&r.HandledAt, &tratadoPor); err != nil {
			return nil, fmt.Errorf("store: scan report: %w", err)
		}
		if contaID != nil {
			r.AccountID = *contaID
		}
		if tratadoPor != nil {
			r.HandledBy = *tratadoPor
		}
		r.Nearby = nearbyFromJSON(porPerto)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate reports: %w", err)
	}
	return out, nil
}

// nearbyFromJSON decodes the bystander list. A row the panel cannot decode is
// still a report somebody wrote, so losing the names beats losing the report.
func nearbyFromJSON(raw []byte) []string {
	var out []string
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

// MarkReportHandled closes one report. Marking an already-closed report is not
// an error: two moderators clicking the same row is a race, not a mistake, and
// the first one keeps the credit.
func (s *Store) MarkReportHandled(ctx context.Context, reportID, staffID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE player_report
		   SET tratado_em = now(), tratado_por = $2
		 WHERE id = $1 AND tratado_em IS NULL`, reportID, nullableID(staffID))
	if err != nil {
		return fmt.Errorf("store: mark report %d handled: %w", reportID, err)
	}
	return nil
}

// ReportCounts is the queue at a glance.
type ReportCounts struct {
	Abertos int
	Total   int
	// MaisAntigo is when the oldest open report came in; zero when none are
	// open. It is the number that says whether the queue is being worked.
	MaisAntigo time.Time
}

// CountReports summarizes the queue without loading it.
func (s *Store) CountReports(ctx context.Context) (ReportCounts, error) {
	var c ReportCounts
	var maisAntigo *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE tratado_em IS NULL),
		       count(*),
		       min(criado_em) FILTER (WHERE tratado_em IS NULL)
		  FROM player_report
		 WHERE expira_em > now()`).Scan(&c.Abertos, &c.Total, &maisAntigo)
	if err != nil {
		return ReportCounts{}, fmt.Errorf("store: count reports: %w", err)
	}
	if maisAntigo != nil {
		c.MaisAntigo = *maisAntigo
	}
	return c, nil
}
