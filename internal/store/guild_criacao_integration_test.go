//go:build integration

// Testes de integração da criação de guilda: cada recusa tem de chegar separada,
// e o que ficou no banco depois de criar tem de bater.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// personagemComOuro cria o personagem já com um saldo, que é o que a criação de
// guilda confere.
func personagemComOuro(ctx context.Context, t *testing.T, s *Store, conta int64, slot int, nome string, ouro int32) {
	t.Helper()
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO character (account_id, slot, name, class, level, coin)
		VALUES ($1, $2, $3, 1, 50, $4)`, conta, slot, nome, ouro); err != nil {
		t.Fatalf("criando o personagem %s: %v", nome, err)
	}
}

// TestCriarGuildaSeparaAsRecusas: "não tem ouro" e "já tem guilda" eram o mesmo
// ErrConflict, e o jogo dizia a frase do nome repetido para as duas. Foi isso que
// escondeu o defeito de ouro de 25/09/2026 por horas.
//
// O caminho feliz também é conferido RECARREGANDO do Postgres: guilda, líder e
// saldo. Cobrar sem criar, ou criar sem cobrar, é o tipo de erro que só aparece
// depois do reinício.
func TestCriarGuildaSeparaAsRecusas(t *testing.T) {
	s, ctx := freshStore(t)
	const custo = int32(100_000_000)

	t.Run("sem ouro", func(t *testing.T) {
		conta := contaPix(ctx, t, s, "guilda_sem_ouro")
		personagemComOuro(ctx, t, s, conta, 0, "Pobre", custo-1)
		_, err := s.CreateGuild(ctx, conta, 0, "Pobre", "GuildaPobre", 7, 1, 0, custo)
		if !errors.Is(err, ErrSemOuro) {
			t.Fatalf("erro = %v, queria ErrSemOuro", err)
		}
		// Recusa não cobra: um centavo a menos aqui seria ouro sumido.
		var ouro int32
		if err := s.pool.QueryRow(ctx,
			`SELECT coin FROM character WHERE account_id = $1 AND slot = 0`, conta).Scan(&ouro); err != nil {
			t.Fatal(err)
		}
		if ouro != custo-1 {
			t.Errorf("ouro depois da recusa = %d, queria %d", ouro, custo-1)
		}
	})

	t.Run("ja tem guilda", func(t *testing.T) {
		conta := contaPix(ctx, t, s, "guilda_ja_tem")
		personagemComOuro(ctx, t, s, conta, 0, "Lider", custo*2)
		if _, err := s.CreateGuild(ctx, conta, 0, "Lider", "PrimeiraGuilda", 7, 1, 0, custo); err != nil {
			t.Fatalf("a primeira criação tinha de passar: %v", err)
		}
		_, err := s.CreateGuild(ctx, conta, 0, "Lider", "SegundaGuilda", 7, 1, 0, custo)
		if !errors.Is(err, ErrJaTemGuilda) {
			t.Fatalf("erro = %v, queria ErrJaTemGuilda", err)
		}
	})

	t.Run("cria e sobrevive ao reinicio", func(t *testing.T) {
		conta := contaPix(ctx, t, s, "guilda_ok")
		personagemComOuro(ctx, t, s, conta, 0, "Chefe", custo+777)
		nome := fmt.Sprintf("Lobos%d", conta)
		g, err := s.CreateGuild(ctx, conta, 0, "Chefe", nome, 7, 1, 0, custo)
		if err != nil {
			t.Fatalf("CreateGuild: %v", err)
		}
		if g.ID == 0 {
			t.Fatal("a guilda saiu com número 0")
		}

		// "Reiniciar" é ler de novo pelo caminho do boot do tmServer.
		guildas, err := s.ListGuilds(ctx)
		if err != nil {
			t.Fatalf("ListGuilds: %v", err)
		}
		achou := false
		for _, lida := range guildas {
			if lida.ID == g.ID {
				achou = true
				if lida.Name != nome {
					t.Errorf("nome depois do reinício = %q, queria %q", lida.Name, nome)
				}
			}
		}
		if !achou {
			t.Error("a guilda criada não voltou da lista")
		}

		var guildID int
		var nivel int
		var ouro int32
		if err := s.pool.QueryRow(ctx,
			`SELECT guild_id, guild_level, coin FROM character WHERE account_id = $1 AND slot = 0`,
			conta).Scan(&guildID, &nivel, &ouro); err != nil {
			t.Fatal(err)
		}
		if guildID != int(g.ID) {
			t.Errorf("guilda do personagem = %d, queria %d", guildID, g.ID)
		}
		if nivel != 9 {
			t.Errorf("nível do criador = %d, queria 9 (líder)", nivel)
		}
		if ouro != 777 {
			t.Errorf("ouro depois de criar = %d, queria 777 (cobrou uma vez só)", ouro)
		}

		// O guild_member tem de espelhar, senão a guilda nasce sem gente.
		var membros int
		if err := s.pool.QueryRow(ctx,
			`SELECT count(*) FROM guild_member WHERE guild_id = $1 AND guild_level = 9`, g.ID).Scan(&membros); err != nil {
			t.Fatal(err)
		}
		if membros != 1 {
			t.Errorf("líderes em guild_member = %d, queria 1", membros)
		}
	})

	// O nome vem do cliente, então o store recusa por NotFound em vez de criar
	// uma guilda sem dono.
	t.Run("personagem que nao existe", func(t *testing.T) {
		conta := contaPix(ctx, t, s, "guilda_sem_personagem")
		_, err := s.CreateGuild(ctx, conta, 0, "Fantasma", "GuildaFantasma", 7, 1, 0, 0)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("erro = %v, queria ErrNotFound", err)
		}
	})
}
