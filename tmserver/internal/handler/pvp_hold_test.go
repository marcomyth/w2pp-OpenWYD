package handler

import (
	"math"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Na arena PvP de Armia (os tiles 0x40 em 2124-2147 x 2140-2155, dentro do
// retângulo da cidade) e nas arenas de guerra, a morte não custa experiência nem
// ponto de caos a ninguém. O abate ainda conta. Foi o relato de 29/09/2026: o
// EvocaBixo perdeu 125.000 de experiência morrendo na arena de Armia.
func TestPvpNaArenaNaoCustaNada(t *testing.T) {
	casos := []struct {
		nome string
		x, y int16
	}{
		{"arena PvP de Armia", 2130, 2145},
		{"arena de guerra de Armia", 200, 220},
		{"campo PvP de Azran", 2600, 1740},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			d, w, killer, victim := pvpKilledWorld(t)
			victim.X, victim.Y = c.x, c.y
			victim.PKPoint = 60

			d.pvpKilled(w, killer, victim)

			if victim.Exp != 10_000 || victim.Hold != 0 {
				t.Errorf("Exp/Hold = %d/%d, queria 10000/0", victim.Exp, victim.Hold)
			}
			if killer.PKPoint != 75 || victim.PKPoint != 60 {
				t.Errorf("PKPoint matador/vítima = %d/%d, queria 75/60", killer.PKPoint, victim.PKPoint)
			}
			if killer.CurKill != 1 || killer.TotKill != 1 {
				t.Errorf("abates = %d/%d, queria 1/1", killer.CurKill, killer.TotKill)
			}
		})
	}
}

// O caso do relato fora da arena: um Arch 353 perdia 150.000×5÷6 =
// 125.000 direto da barra. O legado corta em 30.000 e manda para o Hold.
func TestPvpNoCampoVaiAoHoldComTeto(t *testing.T) {
	d, w, killer, victim := pvpKilledWorld(t)
	victim.Level, victim.ClassMaster = 353, classMasterArch
	victim.Exp = 1_843_036_412

	d.pvpKilled(w, killer, victim)

	if victim.Exp != 1_843_036_412 {
		t.Errorf("Exp = %d, a barra não pode baixar", victim.Exp)
	}
	if victim.Hold != pvpExpLossCap {
		t.Errorf("Hold = %d, queria o teto %d", victim.Hold, pvpExpLossCap)
	}
}

// O Hold nunca passa da experiência que o personagem tem
// (MobKilled.cpp:3256: MOB.Exp > deltaexp ? deltaexp : MOB.Exp).
func TestPvpHoldNaoPassaDaExperiencia(t *testing.T) {
	d, w, killer, victim := pvpKilledWorld(t)
	victim.Exp = 7

	d.pvpKilled(w, killer, victim)

	if victim.Hold != 7 {
		t.Errorf("Hold = %d, queria 7", victim.Hold)
	}
}

// Mortal abaixo do FREEEXP (35) não perde nada; evoluído perde em qualquer nível.
func TestPvpTravaDeNivel(t *testing.T) {
	d, w, killer, victim := pvpKilledWorld(t)
	victim.Level = pvpFreeExpLevel - 1
	d.pvpKilled(w, killer, victim)
	if victim.Hold != 0 {
		t.Errorf("Mortal %d: Hold = %d, queria 0", victim.Level, victim.Hold)
	}

	d, w, killer, victim = pvpKilledWorld(t)
	victim.Level, victim.ClassMaster = 10, classMasterArch
	d.pvpKilled(w, killer, victim)
	if victim.Hold == 0 {
		t.Error("Arch nível 10: Hold = 0, o evoluído perde em qualquer nível")
	}
}

func TestSomaHoldNaoDaAVolta(t *testing.T) {
	casos := []struct {
		hold uint32
		add  int64
		want uint32
	}{
		{0, 30000, 30000},
		{100, 0, 100},
		{100, -5, 100},
		{math.MaxUint32 - 10, 30000, math.MaxUint32},
	}
	for _, c := range casos {
		if got := somaHold(c.hold, c.add); got != c.want {
			t.Errorf("somaHold(%d, %d) = %d, queria %d", c.hold, c.add, got, c.want)
		}
	}
}

func TestPayHold(t *testing.T) {
	casos := []struct {
		nome       string
		hold       uint32
		ganho      int64
		sobra      int64
		holdDepois uint32
	}{
		{"sem dívida passa inteiro", 0, 425, 425, 0},
		{"dívida menor: paga e sobra", 300, 425, 125, 0},
		{"dívida igual: zera as duas", 425, 425, 0, 0},
		{"dívida maior: come tudo", 1000, 425, 0, 575},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			e := &world.Entity{Hold: c.hold}
			if got := payHold(e, c.ganho); got != c.sobra || e.Hold != c.holdDepois {
				t.Errorf("payHold = %d (Hold %d), queria %d (Hold %d)", got, e.Hold, c.sobra, c.holdDepois)
			}
		})
	}
}

// A experiência do abate paga o Hold antes de encher a barra. O abate do
// TestMobKilledGrantsExp vale 425.
func TestAbateDeMonstroPagaOHold(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	killer.Hold = 300
	d.mobKilled(w, killer, w.Entity(w.SpawnMob(expMobTemplate(1, 1000, 0), 6, 5)))
	if killer.Exp != 125 || killer.Hold != 0 {
		t.Errorf("Exp/Hold = %d/%d, queria 125/0", killer.Exp, killer.Hold)
	}

	d, w, killer = mobKilledWorld(t)
	killer.Hold = 1000
	d.mobKilled(w, killer, w.Entity(w.SpawnMob(expMobTemplate(1, 1000, 0), 6, 5)))
	if killer.Exp != 0 || killer.Hold != 575 {
		t.Errorf("Exp/Hold = %d/%d, queria 0/575", killer.Exp, killer.Hold)
	}
}

func TestArenaDeGuerra(t *testing.T) {
	casos := []struct {
		x, y int16
		want int
	}{
		{197, 213, 0}, {238, 230, 0}, {200, 150, 1}, {150, 220, 2}, {150, 150, 3},
		{196, 213, -1}, {2130, 2145, -1}, {4005, 4005, 4},
	}
	for _, c := range casos {
		if got := world.Arena(c.x, c.y); got != c.want {
			t.Errorf("Arena(%d,%d) = %d, queria %d", c.x, c.y, got, c.want)
		}
	}
}
