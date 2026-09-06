package itemstat

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
)

// The two effects the score model deliberately does not carry travel beside the
// effect list, and land in their own maps. Putting them in Effects would need
// them on the itemeffect whitelist, and that whitelist is what keeps them out of
// CurrentScore — so these tests are guarding the separation, not the plumbing.
func TestAlcanceEUsoVaoParaOsMapasProprios(t *testing.T) {
	efeitos := map[int][]content.BaseEffect{}
	reqs := map[int]content.ItemReq{}
	alcances := map[int]int16{1415: 1}
	usos := map[int]int{1415: 0}

	Apply(efeitos, reqs, alcances, usos, map[int]Override{
		1415: {Range: 3, Volatile: 66, Effects: []content.BaseEffect{{Eff: 2, Val: 100}}},
	})

	if alcances[1415] != 3 {
		t.Errorf("alcance = %d, want 3", alcances[1415])
	}
	if usos[1415] != 66 {
		t.Errorf("uso = %d, want 66", usos[1415])
	}
	// And they did NOT leak into the effect list, which is what feeds the score.
	for _, ef := range efeitos[1415] {
		if ef.Val == 3 || ef.Val == 66 {
			t.Errorf("efeitos = %+v — alcance ou uso entrou no score", efeitos[1415])
		}
	}
}

func TestZeroApagaAEntradaEmVezDeGravarZero(t *testing.T) {
	// A missing key and a zero value read identically to every consumer, so
	// keeping only one of the two shapes means a map dump says what the item
	// actually has. It is also how a moderator strips an item of its reach.
	alcances := map[int]int16{1415: 5}
	usos := map[int]int{1415: 58}

	Apply(map[int][]content.BaseEffect{}, map[int]content.ItemReq{}, alcances, usos,
		map[int]Override{1415: {}})

	if _, ok := alcances[1415]; ok {
		t.Errorf("alcance ficou como %d em vez de sumir", alcances[1415])
	}
	if _, ok := usos[1415]; ok {
		t.Errorf("uso ficou como %d em vez de sumir", usos[1415])
	}
}

func TestItemSemOverrideNaoETocado(t *testing.T) {
	alcances := map[int]int16{1415: 5, 2000: 7}
	usos := map[int]int{1415: 58}

	Apply(map[int][]content.BaseEffect{}, map[int]content.ItemReq{}, alcances, usos,
		map[int]Override{1415: {Range: 3}})

	if alcances[2000] != 7 {
		t.Errorf("um item sem override mudou: %d", alcances[2000])
	}
}

func TestMapasNulosNaoQuebram(t *testing.T) {
	// A server booted without a content tree has no catalog to override. Apply is
	// only reached with one, but a nil map assignment panics and the guard is
	// cheaper than the assumption.
	Apply(map[int][]content.BaseEffect{}, map[int]content.ItemReq{}, nil, nil,
		map[int]Override{1415: {Range: 3, Volatile: 66}})
}
