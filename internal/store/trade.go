package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Trade log persistence (0025_trade_log). The tmServer writes one row per
// completed player-to-player trade, off the game loop; the admin panel reads
// them back when somebody reports a scam.
//
// Writes are best-effort by design: a trade that already happened in the world
// must not be undone because Postgres was slow, and the caller logs a failure
// rather than retrying. The row is evidence, not state.

// tradeItemJSON is one item as it is stored. The shape is
// {"index":N,"eff":[[e,v],[e,v],[e,v]]} — short because a busy server writes a
// lot of these, and readable because a moderator will sometimes read the raw
// row.
type tradeItemJSON struct {
	Index int32     `json:"index"`
	Eff   [3][2]int `json:"eff"`
}

func tradeItemsToJSON(items []domain.TradeItem) ([]byte, error) {
	out := make([]tradeItemJSON, 0, len(items))
	for _, it := range items {
		out = append(out, tradeItemJSON{
			Index: it.Index,
			Eff: [3][2]int{
				{int(it.Eff[0][0]), int(it.Eff[0][1])},
				{int(it.Eff[1][0]), int(it.Eff[1][1])},
				{int(it.Eff[2][0]), int(it.Eff[2][1])},
			},
		})
	}
	return json.Marshal(out)
}

func tradeItemsFromJSON(raw []byte) []domain.TradeItem {
	var in []tradeItemJSON
	if len(raw) == 0 || json.Unmarshal(raw, &in) != nil {
		// A row the panel cannot decode is still a row that says a trade
		// happened between two players at a time. Losing the item list is worth
		// less than losing the trade.
		return nil
	}
	out := make([]domain.TradeItem, 0, len(in))
	for _, it := range in {
		t := domain.TradeItem{Index: it.Index}
		for i := 0; i < 3; i++ {
			t.Eff[i] = [2]uint8{uint8(it.Eff[i][0]), uint8(it.Eff[i][1])}
		}
		out = append(out, t)
	}
	return out
}

// RecordTrade stores one completed trade.
func (s *Store) RecordTrade(ctx context.Context, t domain.TradeRecord) error {
	itensA, err := tradeItemsToJSON(t.ItemsA)
	if err != nil {
		return fmt.Errorf("store: encode trade items a: %w", err)
	}
	itensB, err := tradeItemsToJSON(t.ItemsB)
	if err != nil {
		return fmt.Errorf("store: encode trade items b: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO trade_log (char_a, char_b, conta_a, conta_b, ouro_a, ouro_b, itens_a, itens_b)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		// nullableID (npc.go) turns a zero id into SQL NULL: 0 is not an account,
		// and a literal 0 in the column would look like one.
		t.CharA, t.CharB, nullableID(t.AccountA), nullableID(t.AccountB),
		t.GoldA, t.GoldB, itensA, itensB)
	if err != nil {
		return fmt.Errorf("store: record trade %q<->%q: %w", t.CharA, t.CharB, err)
	}
	return nil
}

// TradeQuery filters a trade search. An empty Char matches every trade, which is
// what an operator browsing recent activity wants.
type TradeQuery struct {
	Char  string
	Limit int
	// Offset skips rows so the panel can turn the page. A cap that only ever
	// showed the most recent hundred was honest and still a dead end: there was
	// no way to reach the hundred-and-first.
	Offset int
}

// ListTrades returns trades involving Char (on either side), newest first.
func (s *Store) ListTrades(ctx context.Context, q TradeQuery) ([]domain.TradeRecord, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	nome := strings.TrimSpace(q.Char)

	// One statement for both cases rather than two: with an empty name the
	// clause is trivially true and the planner falls back to the timestamp
	// index, which is exactly the browse-recent path.
	rows, err := s.pool.Query(ctx, `
		SELECT id, ocorrido_em, char_a, char_b, conta_a, conta_b, ouro_a, ouro_b, itens_a, itens_b
		  FROM trade_log
		 WHERE $1 = '' OR char_a = $1 OR char_b = $1
		 ORDER BY ocorrido_em DESC, id DESC
		 LIMIT $2 OFFSET $3`, nome, q.Limit, max(q.Offset, 0))
	if err != nil {
		return nil, fmt.Errorf("store: list trades: %w", err)
	}
	defer rows.Close()

	var out []domain.TradeRecord
	for rows.Next() {
		var t domain.TradeRecord
		var contaA, contaB *int64
		var itensA, itensB []byte
		var quando time.Time
		if err := rows.Scan(&t.ID, &quando, &t.CharA, &t.CharB, &contaA, &contaB,
			&t.GoldA, &t.GoldB, &itensA, &itensB); err != nil {
			return nil, fmt.Errorf("store: scan trade: %w", err)
		}
		t.At = quando
		if contaA != nil {
			t.AccountA = *contaA
		}
		if contaB != nil {
			t.AccountB = *contaB
		}
		t.ItemsA = tradeItemsFromJSON(itensA)
		t.ItemsB = tradeItemsFromJSON(itensB)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate trades: %w", err)
	}
	return out, nil
}
