package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func jeffiFixture(t *testing.T, coin int32) (*Dispatcher, *world.World, *world.Session, *world.Entity, *world.Entity) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16}, log, nil, d.Handle)
	e := &world.Entity{ID: 0, Mode: world.MobUser, Name: "Heroi", Level: 300, BaseMaxHP: 1000, MaxHP: 1000, HP: 1000, Coin: coin}
	npc := &world.Entity{ID: world.MaxUser, Merchant: merchantJeffi}
	return d, w, &world.Session{Conn: 0, Mode: world.UserPlay}, e, npc
}

func resto(index int16, amount int) world.Item {
	it := world.Item{Index: index}
	if amount > 1 {
		setItemAmount(&it, amount)
	}
	return it
}

func contaNaBolsa(e *world.Entity, index int16) int {
	return countCarried(e, activeCarryLimit(e), index)
}

func TestJeffiFazPoeirasECobraPorPoeira(t *testing.T) {
	d, w, s, e, npc := jeffiFixture(t, 3*jeffiPoeiraPrice+7)
	e.Carry[0] = resto(itemRestoOri, 25)
	for i := 1; i <= 10; i++ {
		e.Carry[i] = resto(itemRestoLac, 1)
	}
	d.jeffi(w, s, e, npc)

	if got := contaNaBolsa(e, jeffiPoeiraOri); got != 2 {
		t.Errorf("Poeiras de Ori = %d, want 2 (25 Restos)", got)
	}
	if got := contaNaBolsa(e, jeffiPoeiraLac); got != 1 {
		t.Errorf("Poeiras de Lac = %d, want 1 (10 Restos)", got)
	}
	if got := contaNaBolsa(e, itemRestoOri); got != 5 {
		t.Errorf("Restos de Ori = %d, want 5 — cada Poeira leva exatamente 10", got)
	}
	if got := contaNaBolsa(e, itemRestoLac); got != 0 {
		t.Errorf("Restos de Lac = %d, want 0", got)
	}
	if e.Coin != 7 {
		t.Errorf("gold = %d, want 7 — três Poeiras custam 3M", e.Coin)
	}
	for i := 0; i < activeCarryLimit(e); i++ {
		it := e.Carry[i]
		if it.Index != jeffiPoeiraOri && it.Index != jeffiPoeiraLac {
			continue
		}
		if !hasAmountEffect(it) {
			t.Fatalf("Poeira slot %d sem EF_AMOUNT — empilhável assim derruba o cliente no login", i)
		}
		if v := it.Effects[2].Value; v < 50 || v >= 100 {
			t.Errorf("Poeira slot %d terceiro valor = %d, want [50,100) (:9505)", i, v)
		}
	}
}

func TestJeffiNaoComeRestosAMais(t *testing.T) {
	// The legacy Combine would clear both stacks (28) for one Poeira.
	d, w, s, e, npc := jeffiFixture(t, 2*jeffiPoeiraPrice)
	e.Carry[0] = resto(itemRestoOri, 8)
	e.Carry[1] = resto(itemRestoOri, 20)
	d.jeffi(w, s, e, npc)

	if got := contaNaBolsa(e, jeffiPoeiraOri); got != 2 {
		t.Errorf("Poeiras = %d, want 2 (28 Restos)", got)
	}
	if got := contaNaBolsa(e, itemRestoOri); got != 8 {
		t.Errorf("Restos = %d, want 8", got)
	}
}

func TestJeffiPoeirasEmpilhamComBolsaQuaseCheia(t *testing.T) {
	// The Restos drop in piles of 120: eleven piles and one free slot must still
	// make every Poeira, piled, at 1M each.
	d, w, s, e, npc := jeffiFixture(t, 132*jeffiPoeiraPrice)
	limit := activeCarryLimit(e)
	for i := 0; i < 11; i++ {
		e.Carry[i] = resto(itemRestoOri, 120)
	}
	for i := 11; i < limit-1; i++ {
		e.Carry[i] = world.Item{Index: 400}
	}
	d.jeffi(w, s, e, npc)

	if got := contaNaBolsa(e, jeffiPoeiraOri); got != 132 {
		t.Errorf("Poeiras de Ori = %d, want 132 (1320 Restos)", got)
	}
	if got := contaNaBolsa(e, itemRestoOri); got != 0 {
		t.Errorf("Restos = %d, want 0", got)
	}
	for i := 0; i < limit; i++ {
		if e.Carry[i].Index == jeffiPoeiraOri && itemAmount(e.Carry[i]) > maxStackAmount {
			t.Errorf("slot %d com %d Poeiras, teto %d", i, itemAmount(e.Carry[i]), maxStackAmount)
		}
	}
	if e.Coin != 0 {
		t.Errorf("gold = %d, want 0", e.Coin)
	}
}

func TestJeffiRecusaSemRestosOuSemGold(t *testing.T) {
	cases := []struct {
		name   string
		coin   int32
		restos int
	}{
		{name: "9 restos", coin: jeffiPoeiraPrice, restos: 9},
		{name: "sem gold", coin: jeffiPoeiraPrice - 1, restos: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, w, s, e, npc := jeffiFixture(t, tc.coin)
			e.Carry[0] = resto(itemRestoOri, tc.restos)
			d.jeffi(w, s, e, npc)
			if e.Coin != tc.coin {
				t.Errorf("gold = %d, want %d intacto", e.Coin, tc.coin)
			}
			if got := contaNaBolsa(e, itemRestoOri); got != tc.restos {
				t.Errorf("Restos = %d, want %d intactos", got, tc.restos)
			}
		})
	}
}

func TestJeffiBolsaCheiaNaoCobraNemConsome(t *testing.T) {
	d, w, s, e, npc := jeffiFixture(t, jeffiPoeiraPrice)
	e.Carry[0] = resto(itemRestoOri, 15)
	for i := 1; i < activeCarryLimit(e); i++ {
		e.Carry[i] = world.Item{Index: 400}
	}
	d.jeffi(w, s, e, npc)

	if e.Coin != jeffiPoeiraPrice {
		t.Errorf("gold = %d, want %d — nada foi feito", e.Coin, jeffiPoeiraPrice)
	}
	if got := contaNaBolsa(e, itemRestoOri); got != 15 {
		t.Errorf("Restos = %d, want 15 — sem espaço a pilha fica como estava", got)
	}
}

func TestJeffiPurificaCirculo(t *testing.T) {
	cases := []struct {
		name   string
		piece  int16
		price  int32
		pureLo int16
	}{
		{name: "círculo", piece: itemCirclePiece, price: jeffiCirclePrice, pureLo: itemCirclePure},
		{name: "círculo comp", piece: itemCircleCompPiece, price: jeffiCircleCompPrice, pureLo: itemCircleCompPure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, w, s, e, npc := jeffiFixture(t, tc.price+3)
			e.Equip[jeffiCircleSlot] = world.Item{Index: tc.piece}
			e.Carry[0] = resto(itemRestoOri, 10) // the circle wins: no Poeira
			d.jeffi(w, s, e, npc)

			got := e.Equip[jeffiCircleSlot].Index
			if got < tc.pureLo || got >= tc.pureLo+jeffiCircleVariants {
				t.Errorf("círculo = %d, want [%d,%d)", got, tc.pureLo, tc.pureLo+jeffiCircleVariants)
			}
			if e.Coin != 3 {
				t.Errorf("gold = %d, want 3", e.Coin)
			}
			if n := contaNaBolsa(e, itemRestoOri); n != 10 {
				t.Errorf("Restos = %d, want 10 — com a peça equipada o Jeffi só purifica", n)
			}
		})
	}

	d, w, s, e, npc := jeffiFixture(t, jeffiCircleCompPrice-1)
	e.Equip[jeffiCircleSlot] = world.Item{Index: itemCircleCompPiece}
	d.jeffi(w, s, e, npc)
	if e.Equip[jeffiCircleSlot].Index != itemCircleCompPiece || e.Coin != jeffiCircleCompPrice-1 {
		t.Errorf("sem gold: círculo %d gold %d, want intactos", e.Equip[jeffiCircleSlot].Index, e.Coin)
	}
}

// O gold limita o lote: com 2M e Restos para 5 Poeiras, saem 2 e o resto fica
// como Restos.
func TestJeffiGoldLimitaAsPoeiras(t *testing.T) {
	d, w, s, e, npc := jeffiFixture(t, 2*jeffiPoeiraPrice+5)
	e.Carry[0] = resto(itemRestoOri, 50)
	d.jeffi(w, s, e, npc)

	if got := contaNaBolsa(e, jeffiPoeiraOri); got != 2 {
		t.Errorf("Poeiras = %d, want 2 (gold para 2)", got)
	}
	if got := contaNaBolsa(e, itemRestoOri); got != 30 {
		t.Errorf("Restos = %d, want 30", got)
	}
	if e.Coin != 5 {
		t.Errorf("gold = %d, want 5", e.Coin)
	}
}
