//go:build integration

package store

import (
	"context"
	"testing"
)

// TestLojaDeRcoinListaPorAbaECompraComPrecoVisto cobre o lado do banco da Loja de
// Rcoin do jogo: a lista por aba sai da 0160/0163, e a compra recusa sem cobrar
// quando o preço mudou, quando a oferta não tem aba e quando falta saldo.
func TestLojaDeRcoinListaPorAbaECompraComPrecoVisto(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	// Esquema do zero: os testes de donate que rodam antes apagam o catálogo, e
	// aqui se testa justamente o que a 0160 e a 0163 gravam.
	resetTestSchema(ctx, pool)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)

	// As 72 ofertas da 0160 e as abas da 0163.
	todas, err := s.ListRcoinOffers(ctx, 0)
	if err != nil || len(todas) != 72 {
		t.Fatalf("todas = %d (err %v), want 72", len(todas), err)
	}
	quantas := map[int16]int{}
	for _, o := range todas {
		quantas[o.Category]++
	}
	want := map[int16]int{1: 17, 2: 11, 3: 6, 4: 20, 5: 9, 6: 9}
	for c, n := range want {
		if quantas[c] != n {
			t.Errorf("aba %d = %d ofertas, want %d", c, quantas[c], n)
		}
	}
	fadas, err := s.ListRcoinOffers(ctx, 5)
	if err != nil || len(fadas) != 9 {
		t.Fatalf("fadas = %d (err %v), want 9", len(fadas), err)
	}
	// A fada vem na forma entregue: os dias em EF_WDAY, prontos para o tooltip.
	f := fadas[0]
	if f.Title != "Fada Azul 3 dias" || f.Eff1 != efWDay || f.EffV1 != 3 || f.ExpiresDays != 3 {
		t.Errorf("primeira fada = %+v", f)
	}
	if fora, _ := s.ListRcoinOffers(ctx, 7); len(fora) != 0 {
		t.Errorf("aba 7 devolveu %d ofertas, want 0", len(fora))
	}

	var conta int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, donate_balance) VALUES ('rcoin_jogo_test','x',100) RETURNING id`).
		Scan(&conta); err != nil {
		t.Fatalf("conta: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, conta) })

	// Preço visto diferente: nada é cobrado.
	res, bal, del, err := s.BuyRcoinOffer(ctx, conta, f.ID, f.Price-1)
	if err != nil || res != RcoinBuyPriceChanged || bal != 100 || del != 0 {
		t.Errorf("preço mudou = (%d, %d, %d, %v), want (PriceChanged, 100, 0, nil)", res, bal, del, err)
	}

	// Compra certa: 60 saem, e a entrega fica na fila com o id devolvido.
	res, bal, del, err = s.BuyRcoinOffer(ctx, conta, f.ID, f.Price)
	if err != nil || res != RcoinBuyOK || bal != 40 || del == 0 {
		t.Fatalf("compra = (%d, %d, %d, %v), want (OK, 40, id, nil)", res, bal, del, err)
	}
	pend, err := s.PendingItemDeliveries(ctx, conta)
	if err != nil || len(pend) != 1 || pend[0].ID != del || pend[0].Item.Index != 3901 {
		t.Fatalf("fila = %+v (err %v)", pend, err)
	}

	// Sem saldo: recusa e devolve o saldo atual.
	res, bal, _, err = s.BuyRcoinOffer(ctx, conta, f.ID, f.Price)
	if err != nil || res != RcoinBuyNoFunds || bal != 40 {
		t.Errorf("sem saldo = (%d, %d, %v), want (NoFunds, 40, nil)", res, bal, err)
	}

	// Oferta sem aba não se compra no jogo, mesmo pedida pelo id.
	if _, err := pool.Exec(ctx, `UPDATE donate_shop_item SET category = 0 WHERE id = $1`, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE account SET donate_balance = 1000 WHERE id = $1`, conta); err != nil {
		t.Fatal(err)
	}
	res, bal, _, err = s.BuyRcoinOffer(ctx, conta, f.ID, f.Price)
	if err != nil || res != RcoinBuyUnavailable || bal != 1000 {
		t.Errorf("sem aba = (%d, %d, %v), want (Unavailable, 1000, nil)", res, bal, err)
	}
	if res, _, _, _ := s.BuyRcoinOffer(ctx, conta, 99999999, 1); res != RcoinBuyUnavailable {
		t.Errorf("oferta inexistente = %d, want Unavailable", res)
	}
	_, _ = pool.Exec(ctx, `UPDATE donate_shop_item SET category = 5 WHERE id = $1`, f.ID)
}
