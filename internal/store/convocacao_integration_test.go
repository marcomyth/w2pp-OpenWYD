//go:build integration

// Testes de integração da escalação de cidade: convocar para uma cidade tira a
// pessoa das outras.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"sort"
	"testing"
)

// escalacao devolve os nomes escalados em cada zona, em ordem.
func escalacao(ctx context.Context, t *testing.T, s *Store, guilda uint16) map[int][]string {
	t.Helper()
	sqs, err := s.ListGuildSquads(ctx, guilda)
	if err != nil {
		t.Fatalf("ListGuildSquads: %v", err)
	}
	out := map[int][]string{}
	for _, sq := range sqs {
		nomes := append([]string(nil), sq.Names...)
		sort.Strings(nomes)
		out[sq.Zone] = nomes
	}
	return out
}

// TestConvocarMudaDeCidade: o caso de 29/09/2026 — escalado em Armia, mandado
// para Azran, ele sai de Armia e quem ficou em Armia continua lá.
func TestConvocarMudaDeCidade(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "convoca_lider")
	guildaComLider(ctx, t, s, "Convoca", 4200, conta, 9)

	if err := s.SetGuildSquad(ctx, 4200, 0, []string{"Fulano", "Beltrano"}); err != nil {
		t.Fatalf("escalar em Armia: %v", err)
	}
	// A caixa diferente de propósito: o nome do personagem é o mesmo.
	if err := s.SetGuildSquad(ctx, 4200, 1, []string{"fulano"}); err != nil {
		t.Fatalf("escalar em Azran: %v", err)
	}

	got := escalacao(ctx, t, s, 4200)
	if len(got[0]) != 1 || got[0][0] != "Beltrano" {
		t.Fatalf("Armia = %v, queria só Beltrano", got[0])
	}
	if len(got[1]) != 1 || got[1][0] != "fulano" {
		t.Fatalf("Azran = %v, queria fulano", got[1])
	}
}

// TestConvocarNaoMexeEmOutraGuilda: o mesmo nome em outra guilda não é a mesma
// escalação, e não pode ser apagado por esta.
func TestConvocarNaoMexeEmOutraGuilda(t *testing.T) {
	s, ctx := freshStore(t)
	a := contaPix(ctx, t, s, "convoca_a")
	b := contaPix(ctx, t, s, "convoca_b")
	guildaComLider(ctx, t, s, "ConvocaA", 4201, a, 9)
	guildaComLider(ctx, t, s, "ConvocaB", 4202, b, 9)

	if err := s.SetGuildSquad(ctx, 4202, 0, []string{"Fulano"}); err != nil {
		t.Fatalf("escalar na guilda B: %v", err)
	}
	if err := s.SetGuildSquad(ctx, 4201, 1, []string{"Fulano"}); err != nil {
		t.Fatalf("escalar na guilda A: %v", err)
	}
	if got := escalacao(ctx, t, s, 4202); len(got[0]) != 1 {
		t.Fatalf("a guilda B perdeu a escalação de Armia: %v", got)
	}
}

// TestLimparCidadeNaoMexeNasOutras: a lista vazia limpa só a cidade pedida.
func TestLimparCidadeNaoMexeNasOutras(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "convoca_limpa")
	guildaComLider(ctx, t, s, "ConvocaL", 4203, conta, 9)

	if err := s.SetGuildSquad(ctx, 4203, 0, []string{"Fulano"}); err != nil {
		t.Fatalf("escalar em Armia: %v", err)
	}
	if err := s.SetGuildSquad(ctx, 4203, 2, nil); err != nil {
		t.Fatalf("limpar a zona 2: %v", err)
	}
	if got := escalacao(ctx, t, s, 4203); len(got[0]) != 1 {
		t.Fatalf("Armia = %v, queria Fulano", got[0])
	}
}
