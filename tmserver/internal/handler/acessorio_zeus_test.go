package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Brinco de Zeus ganhou a Defesa do degrau (150) e cresce com o refino pela
// mesma conta do Hércules: +15 dá 600 de Defesa e 84 de velocidade (21 × 4,0),
// os números do tooltip.
func TestBrincoDeZeusMais15(t *testing.T) {
	const brincoZeus = 593
	d := New(Config{
		ItemEffects: map[int][]content.BaseEffect{brincoZeus: {{Eff: efAttSpeed, Val: 21}, {Eff: efAc, Val: 150}}},
		ItemPos:     map[int]int{brincoZeus: nPosAcessorio1},
	})
	it := world.Item{Index: brincoZeus, Effects: [3]world.Effect{{Effect: efSanc}}}
	if !refine.Set(&it, 15, 0) {
		t.Fatal("não gravou +15")
	}
	if got := d.itemAbilityRefined(it, efAc); got != 600 {
		t.Errorf("Defesa do Brinco de Zeus +15 = %d, esperado 600", got)
	}
	if got := d.itemAbilityRefined(it, efAttSpeed); got != 84 {
		t.Errorf("velocidade do Brinco de Zeus +15 = %d, esperado 84", got)
	}
}
