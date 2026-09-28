package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Luvas do teste da EF_ACADD2: a Manopla com os 17 de defesa extra no catálogo
// (Manoplas Elementais(M)) e uma luva igual com 30, para comparar.
const (
	luvaBase17 = 1360
	luvaBase30 = 1361
)

func acAdd2Config() Config {
	return Config{
		ItemEffects: map[int][]content.BaseEffect{
			luvaBase17: {{Eff: efAcAdd, Val: 17}},
			luvaBase30: {{Eff: efAcAdd, Val: 30}},
		},
		ItemPos: map[int]int{luvaBase17: 16, luvaBase30: 16},
	}
}

// defesaCom devolve a AC do personagem com só esta luva equipada.
func defesaCom(d *Dispatcher, luva world.Item) int {
	e := &world.Entity{ID: 1, Level: 50}
	e.Equip[5] = luva
	d.refreshScore(e)
	return int(e.AC)
}

// TestAcAdd2SubstituiADefesaDoCatalogo: a EF_ACADD2 na vaga 1 ou 2 vale NO LUGAR
// da EF_ACADD do catálogo (Basedef.cpp:1717-1721), como o crítico. A Manopla de 17
// com um add de 30 defende como uma luva de 30, não de 47; e o add na vaga 0 não
// arma a troca nem conta.
func TestAcAdd2SubstituiADefesaDoCatalogo(t *testing.T) {
	d := New(acAdd2Config())
	base17 := defesaCom(d, world.Item{Index: luvaBase17})
	base30 := defesaCom(d, world.Item{Index: luvaBase30})
	if base17 == base30 {
		t.Fatalf("as duas luvas de base dão a mesma defesa (%d); o teste não distingue nada", base17)
	}

	for _, c := range []struct {
		nome string
		luva world.Item
		quer int
	}{
		{"add 30 na vaga 1", world.Item{Index: luvaBase17, Effects: [3]world.Effect{
			{Effect: efSanc}, {Effect: efAcAdd2, Value: 30}}}, base30},
		{"add 30 na vaga 2", world.Item{Index: luvaBase17, Effects: [3]world.Effect{
			{Effect: efSanc}, {Effect: efDamage, Value: 18}, {Effect: efAcAdd2, Value: 30}}}, base30},
		{"add na vaga 0 não troca", world.Item{Index: luvaBase17, Effects: [3]world.Effect{
			{Effect: efAcAdd2, Value: 30}}}, base17},
	} {
		t.Run(c.nome, func(t *testing.T) {
			if got := defesaCom(d, c.luva); got != c.quer {
				t.Errorf("defesa %d, quer %d (base 17 = %d, base 30 = %d)", got, c.quer, base17, base30)
			}
		})
	}
}
