package siteapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/dungeon"
)

// leAgenda calls the endpoint and returns the lines.
func leAgenda(t *testing.T, c *cenario) []linhaAgenda {
	t.Helper()
	rec := c.pede("GET", "/site/v1/agenda", "")
	confereStatus(t, rec, http.StatusOK, "")
	var r respostaAgenda
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r.Agenda
}

func achaNaAgenda(t *testing.T, linhas []linhaAgenda, chave string) linhaAgenda {
	t.Helper()
	for _, l := range linhas {
		if l.Chave == chave {
			return l
		}
	}
	t.Fatalf("agenda sem %q: %+v", chave, linhas)
	return linhaAgenda{}
}

// TestAgendaCongelaOsMinutosDoJogo freezes the minutes copied from tmserver.
//
// The copy exists because those constants live under tmserver/internal, which
// this package cannot import (worldevents/tower.go and handler/pesadelo.go).
// This test is where the two versions are caught disagreeing: the day someone
// moves a window in the game, it fails here instead of the site announcing an
// event at the wrong minute.
func TestAgendaCongelaOsMinutosDoJogo(t *testing.T) {
	c := novoCenario(t)
	c.banco.eventosJogo = domain.WorldEventConfig{TowerWarEnabled: true, TowerWarHour: 20}
	linhas := leAgenda(t, c)

	torre := achaNaAgenda(t, linhas, "guerra_de_torres")
	if torre.Hora != 20 || len(torre.Minutos) != 1 || torre.Minutos[0] != 6 || torre.Duracao != 24 {
		t.Errorf("torre = %+v, want hora 20, abre no minuto 6 e dura 24 min (de :06 a :30)", torre)
	}

	// Three windows per tier, twenty minutes apart, four minutes each, starting
	// at :00 Normal, :05 Místico, :10 Arcano.
	for chave, primeiro := range map[string]int32{
		"pesadelo_normal":  0,
		"pesadelo_mistico": 5,
		"pesadelo_arcano":  10,
	} {
		p := achaNaAgenda(t, linhas, chave)
		want := []int32{primeiro, primeiro + 20, primeiro + 40}
		if len(p.Minutos) != 3 || p.Minutos[0] != want[0] || p.Minutos[1] != want[1] || p.Minutos[2] != want[2] {
			t.Errorf("%s: minutos %v, want %v", chave, p.Minutos, want)
		}
		if p.Duracao != 4 {
			t.Errorf("%s: janela de %d min, want 4", chave, p.Duracao)
		}
		if p.Hora != -1 {
			t.Errorf("%s: hora %d, want -1 (repete em toda hora)", chave, p.Hora)
		}
	}
}

// TestAgendaSegueOsInterruptores checks the two switches the staff actually
// turn: the tower war's own, and each Pesadelo door.
func TestAgendaSegueOsInterruptores(t *testing.T) {
	c := novoCenario(t)
	c.banco.eventosJogo = domain.WorldEventConfig{TowerWarEnabled: false, TowerWarHour: 20}
	// Only the Místico door is shut. A door absent from the table is open, which
	// is how the server behaved before the table existed.
	c.banco.portas = domain.DungeonGateConfig{Gates: []domain.DungeonGate{{Gate: int32(dungeon.PesadeloM), Open: false}}}

	linhas := leAgenda(t, c)
	if torre := achaNaAgenda(t, linhas, "guerra_de_torres"); torre.Ligado {
		t.Errorf("torre desligada no painel aparece ligada: %+v", torre)
	}
	if p := achaNaAgenda(t, linhas, "pesadelo_mistico"); p.Ligado {
		t.Errorf("porta fechada aparece ligada: %+v", p)
	}
	for _, chave := range []string{"pesadelo_normal", "pesadelo_arcano"} {
		if p := achaNaAgenda(t, linhas, chave); !p.Ligado {
			t.Errorf("%s: porta ausente da tabela tem que contar como aberta: %+v", chave, p)
		}
	}
}

// TestAgendaNaoPublicaOQueNaoTemRelogio guards the rule the page is built on:
// only what has a readable clock is here. "Chefes sozinhos" is a duration and
// the Água has no window, so neither may show up as a scheduled thing.
func TestAgendaNaoPublicaOQueNaoTemRelogio(t *testing.T) {
	c := novoCenario(t)
	c.banco.eventosJogo = domain.WorldEventConfig{TowerWarEnabled: true, TowerWarHour: 20, BossRespawnHours: 24}
	for _, l := range leAgenda(t, c) {
		for _, proibido := range []string{"chefe", "agua", "água"} {
			if l.Chave == proibido || l.Rotulo == proibido {
				t.Errorf("agenda publica %q, que não tem horário legível", l.Chave)
			}
		}
		if l.Hora < -1 || l.Hora > 23 {
			t.Errorf("%s: hora %d fora de 0..23 (e -1 para o que repete toda hora)", l.Chave, l.Hora)
		}
		if len(l.Minutos) == 0 {
			t.Errorf("%s: sem minuto nenhum, não é agenda", l.Chave)
		}
	}
}
