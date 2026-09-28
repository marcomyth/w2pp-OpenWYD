package handler

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// doisBMsNoGrupo põe dois BMs (conn 0 e conn 1) no mesmo grupo, com os templates
// de verdade. O líder é um mob avulso, como em TestTrocarDeCriaturaComOsTemplatesDeVerdade.
// Desde 26/09 o bando de cada um mora nele (world.Entity.Evocacoes), fora do
// grupo; o que estes testes prendem é que um bando não mexe no outro.
func doisBMsNoGrupo(t *testing.T) (*Dispatcher, *world.World, *world.Entity, [2]*world.Session, [2]*world.Entity, [][]byte) {
	t.Helper()
	templates, _, err := content.LoadBaseSummons(filepath.Join("..", "..", "..", "Release"))
	if err != nil {
		t.Skipf("árvore BaseSummon indisponível: %v", err)
	}
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log, SummonMobs: templates})
	w := world.New(world.Config{GridDim: 64}, log, nil, d.Handle)

	liderID := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Lider"), X: 20, Y: 20, GenIndex: -1})
	if liderID < 0 {
		t.Fatal("não consegui criar o líder")
	}
	var ss [2]*world.Session
	var bms [2]*world.Entity
	for i := range bms {
		bms[i] = &world.Entity{
			ID: i, Mode: world.MobUser, X: 20 + int16(i), Y: 22, HP: 1000, MaxHP: 1000, Level: 50, Int: 100,
			Leader: liderID, BaseSpecial: [4]int16{0, 0, 320, 0}, Special: [4]int16{0, 0, 320, 0},
		}
		ss[i] = &world.Session{Conn: i, Mode: world.UserPlay}
	}
	return d, w, w.Entity(liderID), ss, bms, templates
}

func petsDe(w *world.World, dono *world.Entity) []int {
	var out []int
	for _, id := range bandoDe(dono) {
		if pet := w.Entity(id); pet != nil && pet.Summoner == dono.ID {
			out = append(out, id)
		}
	}
	return out
}

// TestTrocaDeUmBMNaoApagaOsPetsDoOutro: o BM 1 troca para Dragão e os Gorilas
// do BM 0, no mesmo grupo, continuam em campo.
func TestTrocaDeUmBMNaoApagaOsPetsDoOutro(t *testing.T) {
	const gorila, dragao = 5, 6
	d, w, _, ss, bms, _ := doisBMsNoGrupo(t)

	if !d.generateSummon(w, ss[0], bms[0], gorila, 3) {
		t.Fatal("os Gorilas do BM 0 não saíram")
	}
	antes := petsDe(w, bms[0])
	if !d.generateSummon(w, ss[1], bms[1], dragao, 2) {
		t.Fatal("os Dragões do BM 1 não saíram")
	}

	depois := petsDe(w, bms[0])
	if len(depois) != len(antes) {
		t.Errorf("o BM 0 tinha %d Gorilas e ficou com %d depois que o BM 1 evocou Dragão", len(antes), len(depois))
	}
	if n := len(petsDe(w, bms[1])); n != 2 {
		t.Errorf("o BM 1 ficou com %d Dragões, esperado 2", n)
	}
}

// TestCadaBMTemOProprioBando: no legado o teto de uma criatura era do GRUPO
// (GenerateSummon, Server.cpp:2991-2997) e dois BMs juntos dividiam um bando. Com
// o bando fora do grupo (decisão de 26/09), cada BM tem o seu inteiro, e o
// segundo evocar a mesma criatura não tira nada do primeiro.
func TestCadaBMTemOProprioBando(t *testing.T) {
	const gorila = 5
	d, w, lider, ss, bms, _ := doisBMsNoGrupo(t)

	if !d.generateSummon(w, ss[0], bms[0], gorila, 6) {
		t.Fatal("os Gorilas do BM 0 não saíram")
	}
	if !d.generateSummon(w, ss[1], bms[1], gorila, 6) {
		t.Fatal("o BM 1 ficou sem Gorilas porque o BM 0 já tinha os dele")
	}
	for i, bm := range bms {
		if n := len(petsDe(w, bm)); n != 6 {
			t.Errorf("o BM %d ficou com %d Gorilas, esperado 6", i, n)
		}
	}
	if n := partyMemberCount(lider); n != 0 {
		t.Errorf("os bandos ocuparam %d vagas no grupo", n)
	}
}
