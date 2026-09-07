package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Chat log persistence (0034_chat_log).
//
// It answers one question, and it is the one support gets every day: somebody
// opens a ticket saying they were insulted, threatened or talked into a scam,
// and until now there was nothing to look at. The report queue (0028) keeps the
// moment and who was nearby; it does not keep what was said.
//
// Chat is the highest-volume thing a game server produces, so the writes arrive
// in BATCHES — the tmServer buffers a few seconds' worth and sends them at once.
// One round trip per line would put the database in the path of every sentence
// anybody types.

// chatLoteMax caps one batch. A caller that sends more is refused rather than
// silently truncated: a log missing the middle of a conversation is worse than
// one that failed loudly.
const chatLoteMax = 500

// RecordChat stores a batch of chat lines.
//
// Best-effort from the caller's side — the words were already said and heard,
// and failing to write them down must not disturb the game — but it is
// all-or-nothing here: half a batch is half a conversation, and the gap would
// look like silence rather than like a failure.
func (s *Store) RecordChat(ctx context.Context, linhas []domain.ChatLinha) error {
	if len(linhas) == 0 {
		return nil
	}
	if len(linhas) > chatLoteMax {
		return fmt.Errorf("store: record chat: batch of %d exceeds %d", len(linhas), chatLoteMax)
	}

	lote := &pgx.Batch{}
	for _, l := range linhas {
		tipo := strings.TrimSpace(string(l.Tipo))
		if tipo != string(domain.ChatPublico) && tipo != string(domain.ChatSussurro) {
			return fmt.Errorf("store: record chat: unknown type %q", tipo)
		}
		var alvo *string
		if l.Alvo != "" {
			a := l.Alvo
			alvo = &a
		}
		quando := l.At
		if quando.IsZero() {
			quando = time.Now()
		}
		lote.Queue(`
			INSERT INTO chat_log
				(ocorrido_em, tipo, conta_id, char_nome, alvo_nome, texto, pos_x, pos_y)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			quando, tipo,
			// nullableID (npc.go) turns a zero id into SQL NULL: 0 is not an account.
			nullableID(l.AccountID), l.Character, alvo, l.Texto, l.X, l.Y)
	}

	res := s.pool.SendBatch(ctx, lote)
	defer func() { _ = res.Close() }()
	for i := range linhas {
		if _, err := res.Exec(); err != nil {
			return fmt.Errorf("store: record chat line %d of %d: %w", i+1, len(linhas), err)
		}
	}
	return nil
}

// ChatQuery filters the log. Every field is optional; with none of them set the
// result is simply the most recent lines, which is what browsing wants.
type ChatQuery struct {
	// Char matches the speaker OR the person a whisper was addressed to. One
	// field for both because the question is "what passed through this player",
	// and asking a moderator to check two boxes to get one answer is how a
	// conversation ends up half-read.
	Char string
	// Texto is a substring, case-insensitive.
	Texto string
	// Tipo restricts to one channel; empty means both.
	Tipo  domain.ChatTipo
	Desde time.Time
	Ate   time.Time
	Limit int
	// Offset skips rows, so the panel can turn the page. A conversation worth
	// reading is longer than one screen, and a cap that only ever showed the
	// most recent hundred was a dead end in the middle of a ticket.
	Offset int
}

// ListChat returns lines newest first.
//
// It does NOT sweep expired rows on read, unlike the report queue and the ground
// log. The sweep here deletes by a retention that can change, runs in the
// dbServer's periodic job, and can touch a lot of rows at once — putting that on
// the path of a moderator opening a page would make the page slow exactly when
// the table is biggest.
func (s *Store) ListChat(ctx context.Context, q ChatQuery) ([]domain.ChatLinha, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	nome := strings.TrimSpace(q.Char)
	texto := strings.TrimSpace(q.Texto)
	tipo := strings.TrimSpace(string(q.Tipo))
	if tipo != "" && tipo != string(domain.ChatPublico) && tipo != string(domain.ChatSussurro) {
		return nil, fmt.Errorf("store: list chat: unknown type %q", tipo)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, ocorrido_em, tipo, conta_id, char_nome, alvo_nome, texto, pos_x, pos_y
		  FROM chat_log
		 WHERE ($1 = '' OR char_nome = $1 OR alvo_nome = $1)
		   AND ($2 = '' OR texto ILIKE '%' || $2 || '%')
		   AND ($3 = '' OR tipo = $3)
		   AND ($4::timestamptz IS NULL OR ocorrido_em >= $4)
		   AND ($5::timestamptz IS NULL OR ocorrido_em <= $5)
		 ORDER BY ocorrido_em DESC, id DESC
		 LIMIT $6 OFFSET $7`,
		nome, texto, tipo, nullableTime(q.Desde), nullableTime(q.Ate), q.Limit, q.Offset)
	if err != nil {
		return nil, fmt.Errorf("store: list chat: %w", err)
	}
	defer rows.Close()

	var out []domain.ChatLinha
	for rows.Next() {
		var l domain.ChatLinha
		var contaID *int64
		var alvo *string
		var tipo string
		if err := rows.Scan(&l.ID, &l.At, &tipo, &contaID, &l.Character,
			&alvo, &l.Texto, &l.X, &l.Y); err != nil {
			return nil, fmt.Errorf("store: scan chat row: %w", err)
		}
		l.Tipo = domain.ChatTipo(tipo)
		if contaID != nil {
			l.AccountID = *contaID
		}
		if alvo != nil {
			l.Alvo = *alvo
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate chat: %w", err)
	}
	return out, nil
}

// PurgeChat deletes everything older than dias and records what it did.
//
// The retention is compared against ocorrido_em rather than a per-row deadline
// on purpose: lowering the number has to shrink what is ALREADY stored, which is
// what "30 days, 20 if it gets heavy" means. A per-row expiry would only ever
// apply to lines written after the change.
func (s *Store) PurgeChat(ctx context.Context, dias int) (int64, error) {
	if dias <= 0 || dias > domain.ChatRetencaoMax {
		return 0, fmt.Errorf("store: purge chat: retention of %d days is out of range (1..%d)",
			dias, domain.ChatRetencaoMax)
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM chat_log WHERE ocorrido_em < now() - make_interval(days => $1)`, dias)
	if err != nil {
		return 0, fmt.Errorf("store: purge chat: %w", err)
	}
	n := tag.RowsAffected()
	if _, err := s.pool.Exec(ctx, `
		UPDATE chat_log_meta
		   SET varrido_em = now(), dias = $1, apagadas = apagadas + $2`, dias, n); err != nil {
		return n, fmt.Errorf("store: record chat sweep: %w", err)
	}
	return n, nil
}

// ChatSweep reports what the last sweep did.
//
// The panel reads this instead of its own configuration: the retention lives in
// the dbServer's environment, the panel is a different process, and a screen
// that announced a number nobody was enforcing would be worse than one that says
// nothing.
func (s *Store) ChatSweep(ctx context.Context) (domain.ChatVarredura, error) {
	var v domain.ChatVarredura
	var quando *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT varrido_em, dias, apagadas FROM chat_log_meta`).Scan(&quando, &v.Dias, &v.Apagadas)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ChatVarredura{}, nil
	}
	if err != nil {
		return domain.ChatVarredura{}, fmt.Errorf("store: read chat sweep: %w", err)
	}
	if quando != nil {
		v.VarridoEm = *quando
	}
	return v, nil
}

// nullableTime maps a zero time to SQL NULL, so an unset filter matches
// everything instead of matching year one.
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
