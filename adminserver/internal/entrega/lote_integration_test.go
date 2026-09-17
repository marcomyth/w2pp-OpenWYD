//go:build integration

package entrega

import (
	"context"
	"encoding/json"
	"testing"
)

// TestEnfileirarLoteGravaUmaLinhaPorEspaco: 250 Diamantes viram três linhas
// pendentes, cada uma com a sua pilha no efeito, que é o que o dreno do tmServer
// materializa no baú.
func TestEnfileirarLoteGravaUmaLinhaPorEspaco(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	id := conta(t, pool, "lote_teste")
	st := New(pool)

	lote, err := Lote(Item{Index: 2441}, 250)
	if err != nil {
		t.Fatalf("Lote: %v", err)
	}
	ids, err := st.EnfileirarLote(ctx, 1, id, lote)
	if err != nil || len(ids) != 3 {
		t.Fatalf("EnfileirarLote = %v, %v; want três ids", ids, err)
	}
	rows, err := pool.Query(ctx, `SELECT payload FROM delivery_queue WHERE account_id = $1 AND status = 'pending' ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var pilhas []int
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		var p map[string]any
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatal(err)
		}
		if p["eff1"] != float64(61) {
			t.Errorf("payload sem EF_AMOUNT no primeiro efeito: %s", body)
		}
		pilhas = append(pilhas, int(p["effv1"].(float64)))
	}
	if len(pilhas) != 3 || pilhas[0] != 120 || pilhas[1] != 120 || pilhas[2] != 10 {
		t.Errorf("pilhas gravadas = %v, want [120 120 10]", pilhas)
	}
}
