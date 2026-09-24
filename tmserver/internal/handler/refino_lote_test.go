package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// pilhaDe monta uma pilha de n poeiras de um índice.
func pilhaDe(indice int16, n int) world.Item {
	return world.Item{Index: indice, Effects: [3]world.Effect{{Effect: efAmount, Value: uint8(n)}}}
}

// lote pede ao painel: o item do carry 1, a poeira do slot slotPoeira, até +alvo.
func (f *refineFixture) lote(slotPoeira int16, alvo uint8, maxPoeiras uint16) protocol.RefinoResultadoBody {
	return f.d.refinoExecuta(f.w, f.s, f.e, protocol.RefinoPedeBody{
		Lugar: world.ItemPlaceCarry, Slot: 1, SlotPoeira: slotPoeira, Alvo: alvo, MaxPoeiras: maxPoeiras,
	})
}

func conferePlacar(t *testing.T, r protocol.RefinoResultadoBody) {
	t.Helper()
	if r.Usadas != r.Sucessos+r.Falhas {
		t.Errorf("usadas %d != sucessos %d + falhas %d", r.Usadas, r.Sucessos, r.Falhas)
	}
}

func TestRefinoLoteLacAteNove(t *testing.T) {
	f := newRefineFixture(t, alwaysRate(50), nil)
	f.e.Carry[0] = pilhaDe(itemPoeiraLac, 200)
	f.e.Carry[1] = world.Item{Index: itemArmor}

	r := f.lote(0, 9, 0)

	if r.Motivo != protocol.RefinoChegou {
		t.Fatalf("motivo = %d, want CHEGOU", r.Motivo)
	}
	if got := refine.Level(f.target()); got != 9 || r.NivelFinal != 9 || r.NivelInicial != 0 {
		t.Errorf("nível = %d (resposta %d→%d), want 0→9", got, r.NivelInicial, r.NivelFinal)
	}
	conferePlacar(t, r)
	if r.Sucessos != 9 {
		t.Errorf("sucessos = %d, want 9 (a tabela de grau é toda 1)", r.Sucessos)
	}
	if got := itemAmount(f.e.Carry[0]); got != 200-int(r.Usadas) {
		t.Errorf("pilha = %d, want %d (200 - usadas)", got, 200-int(r.Usadas))
	}
}

// O teste de paridade: com a mesma semente, o lote e N arrastos manuais chegam no
// mesmo item — nível, pity e bytes — e gastam a mesma poeira.
func TestRefinoLoteParidadeComArrasto(t *testing.T) {
	for _, alvo := range []uint8{3, 7, 9} {
		lote := newRefineFixture(t, alwaysRate(40), nil)
		lote.e.Carry[0] = pilhaDe(itemPoeiraLac, 250)
		lote.e.Carry[1] = world.Item{Index: itemArmor}
		r := lote.lote(0, alvo, 0)
		conferePlacar(t, r)
		if r.Usadas == 0 || r.Falhas == 0 {
			t.Fatalf("alvo %d: usadas %d falhas %d — a taxa de 40%% devia falhar ao menos uma vez", alvo, r.Usadas, r.Falhas)
		}

		manual := newRefineFixture(t, alwaysRate(40), nil)
		manual.e.Carry[1] = world.Item{Index: itemArmor}
		for i := 0; i < int(r.Usadas); i++ {
			manual.refine(itemPoeiraLac)
		}

		if lote.target() != manual.target() {
			t.Errorf("alvo %d: lote %+v, arrasto %+v", alvo, lote.target(), manual.target())
		}
		if refine.Pity(lote.target()) != refine.Pity(manual.target()) {
			t.Errorf("alvo %d: pity lote %d, arrasto %d", alvo, refine.Pity(lote.target()), refine.Pity(manual.target()))
		}
		if got := itemAmount(lote.e.Carry[0]); got != 250-int(r.Usadas) {
			t.Errorf("alvo %d: pilha = %d, want %d", alvo, got, 250-int(r.Usadas))
		}
		// O próximo sorteio dos dois tem de ser o mesmo: o lote não gastou rand() a mais.
		if a, b := lote.w.Rand().Intn(1000), manual.w.Rand().Intn(1000); a != b {
			t.Errorf("alvo %d: o gerador descasou (%d vs %d)", alvo, a, b)
		}
	}
}

func TestRefinoLoteOriParaNoSeis(t *testing.T) {
	f := newRefineFixture(t, alwaysRate(100), nil)
	f.e.Carry[0] = pilhaDe(itemPoeiraOri, 50)
	f.e.Carry[1] = world.Item{Index: itemArmor}

	r := f.lote(0, 8, 0)

	if r.Motivo != protocol.RefinoMuro || r.NivelFinal != 6 {
		t.Fatalf("motivo %d em +%d, want MURO em +6", r.Motivo, r.NivelFinal)
	}
	if r.Usadas != 6 || itemAmount(f.e.Carry[0]) != 44 {
		t.Errorf("usadas %d, pilha %d; want 6 e 44 (o muro não gasta poeira)", r.Usadas, itemAmount(f.e.Carry[0]))
	}
}

// A pilha escolhida vai primeiro, depois as outras em ordem de slot. E o teto do
// jogador para o lote.
func TestRefinoLotePilhasETeto(t *testing.T) {
	f := newRefineFixture(t, alwaysRate(0), nil)
	f.e.Carry[0] = pilhaDe(itemPoeiraLac, 2)
	f.e.Carry[5] = pilhaDe(itemPoeiraLac, 3)
	f.e.Carry[9] = pilhaDe(itemPoeiraLac, 4)
	f.e.Carry[1] = world.Item{Index: itemArmor}

	r := f.lote(5, 9, 5)

	if r.Motivo != protocol.RefinoTeto || r.Usadas != 5 || r.Falhas != 5 {
		t.Fatalf("resultado %+v, want TETO com 5 falhas", r)
	}
	if !f.e.Carry[5].Empty() || !f.e.Carry[0].Empty() {
		t.Errorf("a escolhida (5) e depois a 0 deviam ter acabado: %+v %+v", f.e.Carry[5], f.e.Carry[0])
	}
	if got := itemAmount(f.e.Carry[9]); got != 4 {
		t.Errorf("pilha 9 = %d, want 4 intacta", got)
	}

	// Sem teto, o que sobra acaba e o motivo é a falta de poeira.
	r = f.lote(9, 9, 0)
	if r.Motivo != protocol.RefinoSemPoeira || r.Usadas != 4 || !f.e.Carry[9].Empty() {
		t.Errorf("resultado %+v, want SEM_POEIRA com 4 usadas", r)
	}
}

func TestRefinoLoteRecusasNaoMexemEmNada(t *testing.T) {
	casos := []struct {
		nome   string
		prep   func(f *refineFixture)
		alvo   uint8
		motivo uint8
	}{
		{"troca aberta", func(f *refineFixture) { f.s.Trade.Active = true }, 5, protocol.RefinoOcupado},
		{"tintura", func(f *refineFixture) { f.e.Carry[1] = world.Item{Index: tinturaLo} }, 5, protocol.RefinoNaoServe},
		{"ovo", func(f *refineFixture) { f.e.Carry[1] = world.Item{Index: eggLo} }, 5, protocol.RefinoNaoServe},
		{"consumível", func(f *refineFixture) { f.e.Carry[1] = world.Item{Index: itemPoeiraOri} }, 5, protocol.RefinoNaoServe},
		{"alvo acima de +11", func(*refineFixture) {}, 12, protocol.RefinoInvalido},
		{"alvo zero", func(*refineFixture) {}, 0, protocol.RefinoInvalido},
		{"item vazio", func(f *refineFixture) { f.e.Carry[1] = world.Item{} }, 5, protocol.RefinoInvalido},
		{"poeira que não é poeira", func(f *refineFixture) { f.e.Carry[0] = world.Item{Index: itemArmor} }, 5, protocol.RefinoInvalido},
	}
	for _, c := range casos {
		f := newRefineFixture(t, alwaysRate(100), nil)
		f.e.Carry[0] = pilhaDe(itemPoeiraLac, 10)
		f.e.Carry[1] = world.Item{Index: itemArmor}
		c.prep(f)
		antes := f.e.Carry

		r := f.lote(0, c.alvo, 0)

		if r.Motivo != c.motivo {
			t.Errorf("%s: motivo %d, want %d", c.nome, r.Motivo, c.motivo)
		}
		if r.Usadas != 0 || f.e.Carry != antes {
			t.Errorf("%s: o lote mexeu no inventário (usadas %d)", c.nome, r.Usadas)
		}
	}
}

// Alvo igual ou abaixo do nível atual é pedido inválido, não "chegou".
func TestRefinoLoteAlvoJaAlcancado(t *testing.T) {
	f := newRefineFixture(t, alwaysRate(100), nil)
	f.e.Carry[0] = pilhaDe(itemPoeiraLac, 10)
	f.e.Carry[1] = world.Item{Index: itemArmor}
	refine.Bootstrap(&f.e.Carry[1])
	refine.Set(&f.e.Carry[1], 4, 0)

	r := f.lote(0, 4, 0)

	if r.Motivo != protocol.RefinoInvalido || r.NivelInicial != 4 || r.Usadas != 0 {
		t.Errorf("resultado %+v, want INVALIDO em +4 sem gastar", r)
	}
}
