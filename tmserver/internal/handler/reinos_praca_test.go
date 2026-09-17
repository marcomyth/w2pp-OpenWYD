package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func fragmentos(n int) world.Item {
	it := world.Item{Index: itemFragmentoDeAlma}
	setItemAmount(&it, n)
	return it
}

func contaAlmas(e *world.Entity) int {
	return contaNaBolsa(e, itemAlmaDoUnicornio) + contaNaBolsa(e, itemAlmaDaFenix)
}

// Cada 10 Fragmentos, de quantas pilhas forem, viram uma Alma; o resto fica.
func TestDragaoDeArmiaTrocaFragmentosPorAlmas(t *testing.T) {
	d, w, s, e, _ := jeffiFixture(t, 0)
	npc := &world.Entity{ID: world.MaxUser, Merchant: merchantDragaoDeArmia}
	e.Carry[0] = fragmentos(18)
	e.Carry[5] = fragmentos(7)
	if !ehDragaoDeArmia(npc) || ehLendaPassada(npc) {
		t.Fatal("o Dragão não é reconhecido pelo Merchant 36")
	}
	d.dragaoDeArmia(w, s, e, npc)
	if got := contaAlmas(e); got != 2 {
		t.Errorf("Almas = %d, want 2 (25 Fragmentos)", got)
	}
	if got := contaNaBolsa(e, itemFragmentoDeAlma); got != 5 {
		t.Errorf("Fragmentos = %d, want 5", got)
	}
}

func TestDragaoDeArmiaSemFragmentosNaoTroca(t *testing.T) {
	d, w, s, e, _ := jeffiFixture(t, 0)
	npc := &world.Entity{ID: world.MaxUser, Merchant: merchantDragaoDeArmia}
	e.Carry[0] = fragmentos(9)
	d.dragaoDeArmia(w, s, e, npc)
	if contaAlmas(e) != 0 || contaNaBolsa(e, itemFragmentoDeAlma) != 9 {
		t.Error("9 Fragmentos viraram Alma, ou sumiram")
	}
}

// Com a bolsa cheia os Fragmentos não somem: a troca não acontece.
func TestDragaoDeArmiaBolsaCheiaNaoComeFragmentos(t *testing.T) {
	d, w, s, e, _ := jeffiFixture(t, 0)
	npc := &world.Entity{ID: world.MaxUser, Merchant: merchantDragaoDeArmia}
	for i := range activeCarryLimit(e) {
		e.Carry[i] = world.Item{Index: 1100}
	}
	e.Carry[0] = fragmentos(15)
	d.dragaoDeArmia(w, s, e, npc)
	if contaAlmas(e) != 0 || contaNaBolsa(e, itemFragmentoDeAlma) != 15 {
		t.Errorf("Almas %d, Fragmentos %d; want 0 e 15", contaAlmas(e), contaNaBolsa(e, itemFragmentoDeAlma))
	}
	// Uma pilha de exatamente 10 libera o próprio espaço para a Alma.
	e.Carry[0] = fragmentos(10)
	d.dragaoDeArmia(w, s, e, npc)
	if contaAlmas(e) != 1 || contaNaBolsa(e, itemFragmentoDeAlma) != 0 {
		t.Errorf("Almas %d, Fragmentos %d; want 1 e 0", contaAlmas(e), contaNaBolsa(e, itemFragmentoDeAlma))
	}
}

// Ao longo de muitas trocas o sorteio dá as duas Almas.
func TestDragaoDeArmiaSorteiaAsDuasAlmas(t *testing.T) {
	d, w, s, e, _ := jeffiFixture(t, 0)
	npc := &world.Entity{ID: world.MaxUser, Merchant: merchantDragaoDeArmia}
	e.Carry[0] = fragmentos(120)
	e.Carry[1] = fragmentos(120)
	d.dragaoDeArmia(w, s, e, npc)
	if u, f := contaNaBolsa(e, itemAlmaDoUnicornio), contaNaBolsa(e, itemAlmaDaFenix); u == 0 || f == 0 || u+f != 24 {
		t.Errorf("Unicórnio %d, Fênix %d; want as duas, somando 24", u, f)
	}
}

func TestLendaPassadaReconhecidaPeloGrau(t *testing.T) {
	lenda := &world.Entity{Merchant: 100, Grade: gradeLendaPassada}
	coveiro := &world.Entity{Merchant: 100, Grade: 0}
	if !ehLendaPassada(lenda) || ehLendaPassada(coveiro) || ehDragaoDeArmia(lenda) {
		t.Error("a Lenda tem de ser só o Merchant 100 de grau 42")
	}
}

// cliqueNaPraca loga um jogador com a bolsa bolsa ao lado de um NPC de template
// tmpl e clica nele; devolve se o aviso texto chegou.
func cliqueNaPraca(t *testing.T, tmpl []byte, bolsa []world.Item, texto string) bool {
	t.Helper()
	db := skillCombatDB(0)
	db.loadResult.X, db.loadResult.Y = 2066, 2068
	copy(db.loadResult.Carry[:], bolsa)
	addr, stop, _ := startServerSkillsTargetMobAt(t, db, tmpl, 4096, 2067, 2068)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()
	questFrame(t, c, world.MaxUser)
	for {
		h, payload, ok := readMaybeHeader(t, c)
		if !ok {
			return false
		}
		if h.Type == protocol.MsgMessagePanel && temAviso([][]byte{payload}, texto) {
			return true
		}
	}
}

func TestPracaCliqueChegaNoDragaoENaLenda(t *testing.T) {
	dragao := targetMobWithClan("Dragao_Dourado", 3, merchantDragaoDeArmia, 10000)
	if !cliqueNaPraca(t, dragao, []world.Item{fragmentos(10)}, "Os Fragmentos se uniram") {
		t.Error("o clique no Dragão não trocou os Fragmentos")
	}
	lenda := targetMobWithClan("Lenda_Passada_", 6, 100, 10000)
	lenda[140+2], lenda[140+3] = 100, gradeLendaPassada // EF_GRADE0 no rosto
	if !cliqueNaPraca(t, lenda, nil, msgLendaPassada) {
		t.Error("o clique na Lenda não falou")
	}
}
