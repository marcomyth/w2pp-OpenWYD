package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A Força Elemental acerta o 3x3 que o tooltip promete — e o Trovão continua com
// a varredura grande do legado. As duas dividem a função e não podem dividir a
// forma.
func TestForcaElementalAcertaSoO3x3(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	caster := &world.Entity{ID: 1, X: 5, Y: 5, Clan: 7}

	// Dentro do 3x3 (x e y de 4 a 6) e fora dele, mas dentro do 5x5 do Trovão.
	dentro := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Dentro"), X: 4, Y: 4, GenIndex: -1})
	fora := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Fora"), X: 2, Y: 2, GenIndex: -1})

	forca := d.thunderTargets(w, caster, varreduraDaForcaElemental)
	if len(forca) != 1 || forca[0].ID != dentro {
		t.Errorf("Força Elemental pegou %d alvos (%v), queria só o de (4,4)", len(forca), forca)
	}

	trovao := d.thunderTargets(w, caster, varreduraDoTrovao)
	if len(trovao) != 2 {
		t.Errorf("o Trovão tem de continuar pegando os dois, pegou %d", len(trovao))
	}
	_ = fora
}

// O teto de alvos é 6, e a Força Elemental não passa dele nem com o 3x3 cheio.
func TestForcaElementalParaEmSeisAlvos(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	caster := &world.Entity{ID: 1, X: 5, Y: 5, Clan: 7}
	// As oito casas do 3x3 em volta do lançador.
	for _, p := range [][2]int16{{4, 4}, {5, 4}, {6, 4}, {4, 5}, {6, 5}, {4, 6}, {5, 6}, {6, 6}} {
		w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("M"), X: p[0], Y: p[1], GenIndex: -1})
	}
	if got := len(d.thunderTargets(w, caster, varreduraDaForcaElemental)); got != varreduraDaForcaElemental.maxAlvos {
		t.Errorf("com o 3x3 cheio pegou %d alvos, want %d", got, varreduraDaForcaElemental.maxAlvos)
	}
}

// O que dá para provar SEM sessão: o nil.
//
// As guardas de verdade (modo PK, cidade, caos, grupo, sessão) estão em
// TestForcaElementalGuardasNoFio, e têm de estar lá. tiqueAlcancaJogador termina
// num gate de sessão em jogo, e sem sessão a resposta é sempre "não alcança" —
// um teste aqui passaria com qualquer uma delas removida, porque nunca chegaria
// a executá-las. Cinco sabotagens seguidas confirmaram isso antes de o teste de
// socket existir.
func TestForcaElementalNaoAlcancaNil(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 64}, log, nil, nil)
	alvo := &world.Entity{ID: 2, X: 41, Y: 40, HP: 1000, Mode: world.MobUser}
	caster := &world.Entity{ID: 1, X: 40, Y: 40, PKMode: true}

	if d.tiqueAlcancaJogador(w, nil, alvo) || d.tiqueAlcancaJogador(w, caster, nil) {
		t.Error("nil não pode alcançar nem ser alcançado")
	}
}

// O Trovão da Black continua FORA do PvP. Foi uma trava deliberada do port, e
// abrir a Força Elemental não podia levá-la junto.
func TestTrovaoContinuaForaDoPvP(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 64}, log, nil, nil)
	caster := &world.Entity{ID: 1, X: 40, Y: 40, PKMode: true}
	alvo := &world.Entity{ID: 2, X: 41, Y: 40, HP: 1000, Mode: world.MobUser}

	if d.validThunderTarget(w, caster, alvo, varreduraDoTrovao.emJogador) {
		t.Error("o Trovão não pode ferir jogador")
	}
	if varreduraDoTrovao.emJogador {
		t.Error("a varredura do Trovão está marcada como PvP")
	}
	if !varreduraDaForcaElemental.emJogador {
		t.Error("a varredura da Força Elemental tem de estar marcada como PvP")
	}
}
