package handler

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestOScoreMontadoLevaAMoldura é a outra metade do defeito.
//
// O teste do encoder prova que o byte vai no lugar certo; este prova que ALGUÉM o
// coloca lá. Sem o campo no computeScore, o encoder continuaria correto e a moldura
// continuaria desaparecendo — o pacote sairia com o byte zerado porque ninguém o
// preencheu, e nenhum teste de protocolo veria isso.
//
// O UpdateScore sai a cada troca de equipamento, buff ou nível. Antes disto, o
// primeiro deles depois do spawn apagava a moldura de quem pagou, para ele e para
// todos em volta.
func TestOScoreMontadoLevaAMoldura(t *testing.T) {
	d := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), CombatRules: regraSemEscala()})
	e := &world.Entity{Level: 50, HP: 1000, MaxHP: 1000, PasseNivel: 3}

	sc := d.computeScore(e)
	if sc.PasseNivel != 3 {
		t.Fatalf("PasseNivel no score = %d, queria 3", sc.PasseNivel)
	}
	// E até o fim do fio: o pacote que sai carrega o byte.
	if b := protocol.EncodeUpdateScore(sc); b[15] != 3 {
		t.Errorf("byte 15 do pacote = %d, queria 3", b[15])
	}
}

// TestSemPasseOScoreMontadoVaiZerado: quem não tem passe produz exatamente o pacote
// de antes.
func TestSemPasseOScoreMontadoVaiZerado(t *testing.T) {
	d := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), CombatRules: regraSemEscala()})
	e := &world.Entity{Level: 50, HP: 1000, MaxHP: 1000}

	if sc := d.computeScore(e); sc.PasseNivel != 0 {
		t.Errorf("PasseNivel = %d sem passe, queria 0", sc.PasseNivel)
	}
}
