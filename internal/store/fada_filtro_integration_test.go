//go:build integration

// Testes de integração do filtro de drop das fadas (tabela fada_filtro, 0183).
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"errors"
	"slices"
	"testing"
)

// Gravar, ler junto com o personagem, e gravar por cima: a lista inteira troca.
func TestFadaFiltroGravaEVoltaComOPersonagem(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "fada_dona")
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO character (account_id, slot, name, class, level) VALUES ($1, 2, 'Fadinha', 1, 50)`, conta); err != nil {
		t.Fatalf("criando o personagem: %v", err)
	}

	// Sem linha: desligado e vazio.
	ch, err := s.LoadCharacter(ctx, conta, 2)
	if err != nil {
		t.Fatalf("LoadCharacter: %v", err)
	}
	if ch.FadaFiltroLigado || len(ch.FadaFiltroItens) != 0 {
		t.Fatalf("sem linha veio %v %v", ch.FadaFiltroLigado, ch.FadaFiltroItens)
	}

	if err := s.SaveFadaFiltro(ctx, conta, 2, true, []int16{412, 2316, 2405}); err != nil {
		t.Fatalf("SaveFadaFiltro: %v", err)
	}
	ch, _ = s.LoadCharacter(ctx, conta, 2)
	if !ch.FadaFiltroLigado || !slices.Equal(ch.FadaFiltroItens, []int16{412, 2316, 2405}) {
		t.Fatalf("voltou %v %v", ch.FadaFiltroLigado, ch.FadaFiltroItens)
	}

	if err := s.SaveFadaFiltro(ctx, conta, 2, false, nil); err != nil {
		t.Fatalf("SaveFadaFiltro vazio: %v", err)
	}
	ch, _ = s.LoadCharacter(ctx, conta, 2)
	if ch.FadaFiltroLigado || len(ch.FadaFiltroItens) != 0 {
		t.Errorf("depois de esvaziar voltou %v %v", ch.FadaFiltroLigado, ch.FadaFiltroItens)
	}
}

// As recusas: ligado sem item, lista acima do teto, e personagem que não existe.
func TestFadaFiltroRecusas(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "fada_recusa")
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO character (account_id, slot, name, class, level) VALUES ($1, 0, 'Recusada', 1, 50)`, conta); err != nil {
		t.Fatalf("criando o personagem: %v", err)
	}
	if err := s.SaveFadaFiltro(ctx, conta, 0, true, nil); err == nil {
		t.Error("gravou ligado com a lista vazia")
	}
	if err := s.SaveFadaFiltro(ctx, conta, 0, false, make([]int16, FadaFiltroMax+1)); err == nil {
		t.Error("gravou lista acima do teto")
	}
	if err := s.SaveFadaFiltro(ctx, conta, 3, false, []int16{412}); !errors.Is(err, ErrNotFound) {
		t.Errorf("slot vazio: err = %v, quero ErrNotFound", err)
	}
	// O CHECK do banco segura quem gravar por outro caminho.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO fada_filtro (character_id, ligado, itens)
		SELECT id, TRUE, '{}' FROM character WHERE account_id = $1 AND slot = 0`, conta); err == nil {
		t.Error("o banco aceitou ligado com a lista vazia")
	}
}

// Apagar o personagem leva o filtro junto.
func TestFadaFiltroSomeComOPersonagem(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "fada_some")
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO character (account_id, slot, name, class, level) VALUES ($1, 1, 'Passageira', 1, 50)`, conta); err != nil {
		t.Fatalf("criando o personagem: %v", err)
	}
	if err := s.SaveFadaFiltro(ctx, conta, 1, true, []int16{412}); err != nil {
		t.Fatalf("SaveFadaFiltro: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM character WHERE account_id = $1 AND slot = 1`, conta); err != nil {
		t.Fatalf("apagando o personagem: %v", err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM fada_filtro`).Scan(&n); err != nil || n != 0 {
		t.Errorf("sobraram %d linhas (err %v)", n, err)
	}
}
