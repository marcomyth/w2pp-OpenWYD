package handler

import (
	"math"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestControleDeManaDeMonstro é a conta do lado do MONSTRO (Server.cpp:10041):
// a mana paga METADE do golpe e o divisor 80 deixa passar ~20,6%, contra o golpe
// inteiro e os 30% do lado do jogador.
func TestControleDeManaDeMonstro(t *testing.T) {
	alvo := func(mp int32) *world.Entity {
		e := &world.Entity{ID: 1, MP: mp, MaxMP: 8000}
		e.Affect[0] = world.Affect{Type: 18, Time: 600}
		return e
	}

	e := alvo(8000)
	dmg, gasto, ok := manaControlDeMonstro(e, 1000)
	if !ok {
		t.Fatal("o Controle de Mana tinha de pegar")
	}
	// (1000>>1 + 1000<<4) / 80 = 16500/80 = 206.
	if dmg != 206 || gasto != 500 || e.MP != 7500 {
		t.Errorf("golpe de monstro = %d, mana %d, MP %d; quero 206/500/7500", dmg, gasto, e.MP)
	}

	// O lado do jogador não mudou: golpe inteiro na mana, 30% na vida.
	if d, g, _ := manaControlDamage(alvo(8000), 1000, false); d != 300 || g != 1000 {
		t.Errorf("golpe de jogador = %d, mana %d; quero 300/1000", d, g)
	}

	// Abaixo de 10% da mana o escudo desliga e o golpe entra inteiro na vida.
	if d, _, ok := manaControlDeMonstro(alvo(100), 1000); ok || d != 1000 {
		t.Errorf("com a mana no chão = %d ok=%v; quero 1000 e false", d, ok)
	}

	// Sem o afeto não há escudo nenhum: é o estado em que a skill 46 ficou
	// enquanto o caminho do monstro não existia.
	if d, _, ok := manaControlDeMonstro(&world.Entity{ID: 1, MP: 8000, MaxMP: 8000}, 1000); ok || d != 1000 {
		t.Errorf("sem o afeto 18 = %d ok=%v; quero 1000 e false", d, ok)
	}
}

// TestGolpeDeMonstroPassaPeloControleDeMana é o FIO, não a conta: o golpe de
// monstro de verdade, no mundo servindo, contra o mesmo jogador com e sem o
// afeto 18. Enquanto a chamada não existia em mobai.go este teste falhava com o
// golpe saindo igual nos dois e a mana intacta.
//
// A prova não compara as duas rodadas entre si — o sorteio do golpe difere. Ela
// olha DENTRO da rodada com o buff: cada golpe tira dmg/2 de mana e devolve
// dmg×16,5/80 de dano, então a razão dano/mana é 0,4125 qualquer que seja o
// sorteio.
func TestGolpeDeMonstroPassaPeloControleDeMana(t *testing.T) {
	addr, stop, d, w := startServerRegra(t, nil)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	const golpes = 200
	medir := func(comBuff bool) (int, int32) {
		var soma int
		var mana int32
		noLaco(t, w, func(w *world.World) {
			alvo := w.Entity(1)
			alvo.MaxHP, alvo.HP = 1_000_000, 1_000_000
			alvo.MaxMP, alvo.MP = 4_000_000, 4_000_000
			alvo.Affect[0] = world.Affect{}
			if comBuff {
				alvo.Affect[0] = world.Affect{Type: 18, Time: 600}
			}
			mob := &world.Entity{ID: world.MaxUser + 7, Damage: 6000}
			antes := alvo.MP
			for range golpes {
				soma += d.danoDoGolpeDeMonstro(w, mob, alvo)
			}
			mana = antes - alvo.MP
		})
		drena(t, c)
		return soma, mana
	}

	sem, manaSem := medir(false)
	com, manaCom := medir(true)

	if sem <= golpes {
		t.Fatalf("o golpe sem o buff somou %d em %d golpes: o teste não mede nada", sem, golpes)
	}
	if manaSem != 0 {
		t.Errorf("sem o afeto 18 a mana caiu %d; não devia sair nada", manaSem)
	}
	if manaCom <= 0 {
		t.Fatal("com o afeto 18 a mana não saiu: o golpe de monstro não passa pelo Controle de Mana")
	}
	if razao := float64(com) / float64(manaCom); math.Abs(razao-0.4125) > 0.01 {
		t.Errorf("dano/mana = %.4f (dano %d, mana %d); quero 0,4125", razao, com, manaCom)
	}
	if com >= sem {
		t.Errorf("com o buff o golpe somou %d e sem ele %d: o escudo tem de tirar dano", com, sem)
	}
}
