package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mountrate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const itemAdultaAndaluzN = itemCriaAndaluzN + mountRowSize // 2370, come o 2400

// montariaFixture é o refineFixture do painel de montaria: um despachante com a
// curva dada (nil = nenhuma, tudo no padrão 50) e a adulta vestida viva.
type montariaFixture struct {
	d *Dispatcher
	w *world.World
	s *world.Session
	e *world.Entity
}

func newMontariaFixture(t *testing.T, curva *mountrate.Curve, nivel uint8) *montariaFixture {
	t.Helper()
	cfg := Config{
		Log:           slog.New(slog.DiscardHandler),
		ItemVolatiles: map[int]int{itemAmagoAndaluzN: volAmago},
	}
	if curva != nil {
		cfg.MountRates = mountrate.Table{itemAdultaAndaluzN: *curva}
	}
	f := &montariaFixture{
		d: New(cfg),
		w: world.New(world.Config{GridDim: 16}, slog.New(slog.DiscardHandler), nil, nil),
		s: &world.Session{Conn: 1, Mode: world.UserPlay},
		e: &world.Entity{ID: 1, HP: 100},
	}
	m := world.Item{Index: itemAdultaAndaluzN}
	putShort(&m.Effects[0], mountFedValue)
	m.Effects[1].Effect = nivel
	f.e.Equip[mountEquipSlot] = m
	return f
}

// pilhaDeAmago monta uma pilha de n itens de um índice.
func pilhaDeAmago(indice int16, n int) world.Item {
	return world.Item{Index: indice, Effects: [3]world.Effect{{Effect: efAmount, Value: uint8(n)}}}
}

// curvaToda devolve uma curva com a mesma taxa nas 6 faixas.
func curvaToda(taxa int8) *mountrate.Curve {
	c := mountrate.UnsetCurve()
	for i := range c {
		c[i] = taxa
	}
	return &c
}

func (f *montariaFixture) montaria() world.Item { return f.e.Equip[mountEquipSlot] }
func (f *montariaFixture) nivel() uint8         { return f.e.Equip[mountEquipSlot].Effects[1].Effect }

func (f *montariaFixture) lote(slot int16, pilhas uint8) protocol.MontariaResultadoBody {
	return f.d.montariaExecuta(f.w, f.s, f.e, protocol.MontariaPedeBody{SlotAmago: slot, Pilhas: pilhas})
}

// arrasta é o arrasto manual: o âmago do slot src em cima da montaria.
func (f *montariaFixture) arrasta(src int) {
	body := protocol.MsgUseItemBody{
		SourType: world.ItemPlaceCarry, SourPos: int32(src),
		DestType: 0, DestPos: mountEquipSlot,
	}
	f.d.useAmago(f.w, f.s, f.e, body, src)
}

func conferePlacarMontaria(t *testing.T, r protocol.MontariaResultadoBody) {
	t.Helper()
	if r.Usados != r.Sucessos+r.Falhas {
		t.Errorf("usados %d != sucessos %d + falhas %d", r.Usados, r.Sucessos, r.Falhas)
	}
}

// O teste de paridade: com a mesma semente, o lote e o arrasto (10 por vez) chegam
// na mesma montaria, gastam o mesmo âmago e deixam o gerador no mesmo ponto.
func TestMontariaLoteParidadeComArrasto(t *testing.T) {
	for _, n := range []int{10, 40, 90} {
		lote := newMontariaFixture(t, nil, 30)
		lote.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, n)
		r := lote.lote(0, 1)
		conferePlacarMontaria(t, r)
		if int(r.Usados) != n || r.Motivo != protocol.MontariaAcabouPacs || r.Pilhas != 1 {
			t.Fatalf("n=%d: %+v, want os %d gastos e ACABOU_PACS", n, r, n)
		}
		if r.Falhas == 0 {
			t.Fatalf("n=%d: nenhuma falha a 50%% — o teste não prova nada", n)
		}

		manual := newMontariaFixture(t, nil, 30)
		manual.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, n)
		for i := 0; i < n/amagoBatch; i++ {
			manual.arrasta(0)
		}

		if lote.montaria() != manual.montaria() {
			t.Errorf("n=%d: lote %+v, arrasto %+v", n, lote.montaria(), manual.montaria())
		}
		if lote.e.Carry[0] != manual.e.Carry[0] {
			t.Errorf("n=%d: pilha lote %+v, arrasto %+v", n, lote.e.Carry[0], manual.e.Carry[0])
		}
		if int(r.NivelFinal) != int(lote.nivel()) || r.NivelInicial != 30 {
			t.Errorf("n=%d: resposta %d→%d, montaria em %d", n, r.NivelInicial, r.NivelFinal, lote.nivel())
		}
		if int(r.NivelFinal) != 30+int(r.Sucessos)-int(r.Quedas) {
			t.Errorf("n=%d: final %d != 30 + %d subiu - %d voltou", n, r.NivelFinal, r.Sucessos, r.Quedas)
		}
		if a, b := lote.w.Rand().Intn(1000), manual.w.Rand().Intn(1000); a != b {
			t.Errorf("n=%d: o gerador descasou (%d vs %d)", n, a, b)
		}
	}
}

func TestMontariaLoteTaxaCemSobeTudo(t *testing.T) {
	f := newMontariaFixture(t, curvaToda(100), 0)
	f.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, 20)

	r := f.lote(0, 1)

	if r.Motivo != protocol.MontariaAcabouPacs || r.Sucessos != 20 || r.Falhas != 0 || r.NivelFinal != 20 {
		t.Fatalf("resultado %+v, want 20 sucessos até o 20", r)
	}
	if !f.e.Carry[0].Empty() {
		t.Errorf("a pilha devia ter acabado: %+v", f.e.Carry[0])
	}
	// O âmago alimenta mesmo quando falha; aqui basta ver que alimentou.
	if amagoHunger(f.montaria()) != mountFedValue {
		t.Errorf("HP da montaria = %d, want %d", amagoHunger(f.montaria()), mountFedValue)
	}
}

// Taxa 0 ainda é 1% (Intn(101) > 0), e o nível nunca passa abaixo de 0.
func TestMontariaLoteTaxaZeroNaoPassaDeZero(t *testing.T) {
	f := newMontariaFixture(t, curvaToda(0), 0)
	f.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, 200)

	r := f.lote(0, 0)

	conferePlacarMontaria(t, r)
	if r.Motivo != protocol.MontariaSemAmago || r.Usados != 200 {
		t.Fatalf("resultado %+v, want SEM_AMAGO com os 200", r)
	}
	if r.Falhas < 150 {
		t.Errorf("falhas = %d; a taxa 0 devia falhar quase sempre", r.Falhas)
	}
	if int(r.NivelFinal) > int(r.Sucessos) {
		t.Errorf("final %d acima dos %d sucessos", r.NivelFinal, r.Sucessos)
	}
}

// Um Pac é uma pilha, gasta inteira, seja do tamanho que for. A escolhida vai
// primeiro, depois as outras em ordem de slot.
func TestMontariaLotePacsSaoPilhasInteiras(t *testing.T) {
	f := newMontariaFixture(t, curvaToda(0), 0)
	f.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, 35)
	f.e.Carry[5] = pilhaDeAmago(itemAmagoAndaluzN, 200)
	f.e.Carry[9] = pilhaDeAmago(itemAmagoAndaluzN, 70)

	r := f.lote(5, 2)

	if r.Motivo != protocol.MontariaAcabouPacs || r.Pilhas != 2 || r.Usados != 235 {
		t.Fatalf("resultado %+v, want 2 pilhas (200 da escolhida + 35 da 0)", r)
	}
	if !f.e.Carry[5].Empty() || !f.e.Carry[0].Empty() {
		t.Errorf("a escolhida (5) e depois a 0 deviam ter acabado: %+v %+v", f.e.Carry[5], f.e.Carry[0])
	}
	if got := itemAmount(f.e.Carry[9]); got != 70 {
		t.Errorf("pilha 9 = %d, want 70 intacta", got)
	}

	// "Todos": o que sobra acaba e o motivo é a falta de âmago.
	r = f.lote(9, 0)
	if r.Motivo != protocol.MontariaSemAmago || r.Usados != 70 || r.Pilhas != 1 {
		t.Errorf("resultado %+v, want SEM_AMAGO com 70", r)
	}
}

// No 120 o lote para no meio da pilha, e o resto fica.
func TestMontariaLoteParaNoCentoEVinte(t *testing.T) {
	f := newMontariaFixture(t, curvaToda(100), 115)
	f.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, 50)

	r := f.lote(0, 0)

	if r.Motivo != protocol.MontariaNoMaximo || r.NivelFinal != adultMaxLevel || r.Usados != 5 {
		t.Fatalf("resultado %+v, want NO_MAXIMO no 120 com 5", r)
	}
	if got := itemAmount(f.e.Carry[0]); got != 45 || r.Pilhas != 0 {
		t.Errorf("pilha = %d (pilhas %d), want 45 e nenhuma acabada", got, r.Pilhas)
	}
}

// A Alquimia da HT soma: a curva 98 com +2 vira 100 e não falha nunca.
func TestMontariaLoteSomaAlquimia(t *testing.T) {
	f := newMontariaFixture(t, curvaToda(98), 0)
	f.e.Class = 3
	f.e.LearnedSkill = alquimiaSkillBit
	f.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, 200)

	r := f.lote(0, 0)

	if r.Motivo != protocol.MontariaNoMaximo || r.Falhas != 0 || r.Usados != adultMaxLevel {
		t.Errorf("resultado %+v, want 120 sucessos seguidos com a taxa em 100", r)
	}
}

func TestMontariaLoteRecusasNaoMexemEmNada(t *testing.T) {
	casos := []struct {
		nome   string
		prep   func(f *montariaFixture)
		motivo uint8
	}{
		{"troca aberta", func(f *montariaFixture) { f.s.Trade.Active = true }, protocol.MontariaOcupado},
		{"sem montaria", func(f *montariaFixture) { f.e.Equip[mountEquipSlot] = world.Item{} }, protocol.MontariaNaoAdulta},
		{"cria", func(f *montariaFixture) { f.e.Equip[mountEquipSlot].Index = itemCriaAndaluzN }, protocol.MontariaNaoAdulta},
		{"morta", func(f *montariaFixture) { putShort(&f.e.Equip[mountEquipSlot].Effects[0], 0) }, protocol.MontariaMorta},
		{"no 120", func(f *montariaFixture) { f.e.Equip[mountEquipSlot].Effects[1].Effect = adultMaxLevel }, protocol.MontariaNoMaximo},
		{"âmago de outra linhagem", func(f *montariaFixture) { f.e.Carry[0] = pilhaDeAmago(2401, 10) }, protocol.MontariaInvalido},
		{"não é âmago", func(f *montariaFixture) { f.e.Carry[0] = pilhaDeAmago(itemPoeiraLac, 10) }, protocol.MontariaInvalido},
		{"slot vazio", func(f *montariaFixture) { f.e.Carry[0] = world.Item{} }, protocol.MontariaInvalido},
	}
	for _, c := range casos {
		f := newMontariaFixture(t, curvaToda(100), 30)
		f.e.Carry[0] = pilhaDeAmago(itemAmagoAndaluzN, 10)
		c.prep(f)
		antesCarry, antesMontaria := f.e.Carry, f.montaria()

		r := f.lote(0, 0)

		if r.Motivo != c.motivo {
			t.Errorf("%s: motivo %d, want %d", c.nome, r.Motivo, c.motivo)
		}
		if r.Usados != 0 || f.e.Carry != antesCarry || f.montaria() != antesMontaria {
			t.Errorf("%s: o lote mexeu em algo (usados %d)", c.nome, r.Usados)
		}
	}
	// Slot fora da mochila.
	f := newMontariaFixture(t, nil, 30)
	if r := f.lote(-1, 0); r.Motivo != protocol.MontariaInvalido {
		t.Errorf("slot -1: motivo %d, want INVALIDO", r.Motivo)
	}
}
