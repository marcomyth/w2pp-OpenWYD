package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/combatrule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// golpeFisicoEmJogador é o caminho do golpe físico de jogador em jogador, sem o
// handler: a defesa pesada pela regra, a fórmula do legado e o quarto do PvP.
// O sorteio é fixo no fundo da faixa (fator 106% com maestria 14) e a esquiva
// está em zero, para o número ser sempre o mesmo.
func golpeFisicoEmJogador(d *Dispatcher, ataque, defesa int, critico uint8) int {
	atacante := &world.Entity{ID: 1, Class: 2}
	alvo := &world.Entity{ID: 2, Class: 0, AC: int32(defesa)}
	dmg := combat.ResolveHit(fixedRand(0), combat.HitInput{
		AttackerDamage:   ataque,
		TargetAC:         d.defesaContraGolpeFisico(atacante, alvo, alvo.ID),
		TargetIsPlayer:   true,
		AttackerIsPlayer: true,
		DoubleCritical:   critico,
		Master:           14,
	})
	return perfuracao(alvo, alvo.ID, dmg, 0)
}

// TestPesoDaDefesaNoLegadoReproduzOJogo: com 150 a conta dá o que o Marco mediu
// em jogo em 01/10/2026 — nenhum físico passava da defesa de outro.
func TestPesoDaDefesaNoLegadoReproduzOJogo(t *testing.T) {
	d := &Dispatcher{combatRules: combatrule.Default()}
	casos := []struct {
		nome            string
		ataque, defesa  int
		normal, critico int // crítico parcial, ×1,3 no fundo da faixa
	}{
		{"BM 2.700 na HT de 2.100", 2700, 2100, 1, 95},
		{"BM 2.650 no TK de 2.600", 2650, 2600, 1, 1},
		{"TK 3.900 no BM de 3.500", 3900, 3500, 1, 1},
	}
	for _, c := range casos {
		if got := golpeFisicoEmJogador(d, c.ataque, c.defesa, 0); got != c.normal {
			t.Errorf("%s, normal: %d, want %d", c.nome, got, c.normal)
		}
		if got := golpeFisicoEmJogador(d, c.ataque, c.defesa, 2); got != c.critico {
			t.Errorf("%s, crítico: %d, want %d", c.nome, got, c.critico)
		}
	}
}

// TestPesoDaDefesaEm100DeixaOGolpePassar: com o peso da skill os mesmos golpes
// ferem, e quem tem mais ataque que a defesa do outro tira mais.
func TestPesoDaDefesaEm100DeixaOGolpePassar(t *testing.T) {
	d := &Dispatcher{combatRules: combatrule.Default()}
	d.combatRules.PvPMeleeArmorPct = 100
	casos := []struct {
		nome            string
		ataque, defesa  int
		normal, critico int
	}{
		{"BM 2.700 na HT de 2.100", 2700, 2100, 159, 373},
		{"BM 2.650 no TK de 2.600", 2650, 2600, 13, 224},
		{"TK 3.900 no BM de 3.500", 3900, 3500, 106, 416},
	}
	for _, c := range casos {
		if got := golpeFisicoEmJogador(d, c.ataque, c.defesa, 0); got != c.normal {
			t.Errorf("%s, normal: %d, want %d", c.nome, got, c.normal)
		}
		if got := golpeFisicoEmJogador(d, c.ataque, c.defesa, 2); got != c.critico {
			t.Errorf("%s, crítico: %d, want %d", c.nome, got, c.critico)
		}
	}
}

// TestPesoDaDefesaSoValeEntreJogadores: em 150 a defesa volta intacta (o golpe
// é o do legado, bit a bit), monstro nunca é pesado, e o peso se soma à
// perfuração da classe em vez de substituí-la.
func TestPesoDaDefesaSoValeEntreJogadores(t *testing.T) {
	atacante := &world.Entity{ID: 1, Class: 2}
	jogador := &world.Entity{ID: 2, AC: 3000}
	monstro := &world.Entity{ID: world.MaxUser + 5, AC: 3000}

	d := &Dispatcher{combatRules: combatrule.Default()}
	if got := d.defesaContraGolpeFisico(atacante, jogador, jogador.ID); got != 3000 {
		t.Errorf("no legado (150): defesa %d, want 3000 intacta", got)
	}
	d.combatRules.PvPMeleeArmorPct = 100
	if got := d.defesaContraGolpeFisico(atacante, jogador, jogador.ID); got != 2000 {
		t.Errorf("em 100: defesa %d, want 2000 (3.000 × 100/150)", got)
	}
	if got := d.defesaContraGolpeFisico(atacante, monstro, monstro.ID); got != 3000 {
		t.Errorf("monstro: defesa %d, want 3000 — o peso é só de PvP", got)
	}
	d.combatRules.PvPMeleeArmorPct = 0
	if got := d.defesaContraGolpeFisico(atacante, jogador, jogador.ID); got != 0 {
		t.Errorf("em 0: defesa %d, want 0", got)
	}
}

// TestPesoDaDefesaPeloServidorInteiro passa pelo servidor inteiro: dois
// personagens iguais, de ~1.870 de Ataque (FOR 3.000) e ~1.200 de defesa nas
// peças. A ficha vem dos atributos e do equipamento, e não de Damage/AC
// forçados, porque o refreshScore do login recalcula os dois. Cada servidor
// começa com o mesmo sorteio, então o golpe é o mesmo nos três.
//
// No legado quase nada passa (1.200 × 1,5 = 1.800 de desconto). Em 100 o
// golpe sai várias vezes maior, e em 0 passa inteiro.
func TestPesoDaDefesaPeloServidorInteiro(t *testing.T) {
	golpe := func(peso int32) int {
		db := newDB()
		db.loadResult = world.CharacterState{
			Slot: 0, Name: "Hero", X: 5, Y: 5,
			HP: 100000, MaxHP: 100000, Level: 200, Str: 3000,
		}
		for slot := 1; slot <= 2; slot++ {
			db.loadResult.Equip[slot] = world.Item{Index: int16(1000 + slot),
				Effects: [3]world.Effect{{Effect: efAc, Value: 250}, {Effect: efAc, Value: 250}}}
		}
		regra := *regraSemEscala()
		regra.PvPMeleeArmorPct = peso
		addr, stop := startServerClockRegra(t, db, regra)
		defer stop()
		atacante := enterWorld(t, addr)
		defer atacante.Close()
		alvo := enterWorld(t, addr)
		defer alvo.Close()

		send(t, atacante, protocol.MsgPKMode, protocol.EncodeStandardParm(1))
		attackFrame(t, atacante, serverTime, 2, 0)
		ty, payload, ok := readMaybe(t, alvo)
		if !ok || ty != protocol.MsgAttack {
			t.Fatalf("alvo recebeu %#x ok=%v, want o MsgAttack", ty, ok)
		}
		var got protocol.MsgAttackBody
		if err := got.Decode(payload); err != nil {
			t.Fatal(err)
		}
		if len(got.Dam) != 1 || got.Dam[0].TargetID != 2 {
			t.Fatalf("Dam = %+v", got.Dam)
		}
		return int(got.Dam[0].Damage)
	}
	semDefesa := golpe(0)
	if semDefesa < 300 {
		t.Fatalf("golpe sem defesa = %d: pequeno demais para medir o peso", semDefesa)
	}
	legado := golpe(combatrule.LegacyPvPMeleeArmorPct)
	if legado < 1 || legado > semDefesa/10 {
		t.Errorf("no legado (150) o golpe saiu %d, want no máximo um décimo de %d — a defesa come o golpe", legado, semDefesa)
	}
	if d := golpe(100); d < 3*legado || d < 100 || d >= semDefesa {
		t.Errorf("em 100 o golpe saiu %d, want pelo menos 100, o triplo do legado (%d) e menos que sem defesa (%d)", d, legado, semDefesa)
	}
}
