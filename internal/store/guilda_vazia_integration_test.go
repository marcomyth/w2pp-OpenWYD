//go:build integration

// Testes de integração da guilda que fica sem ninguém: o último a sair leva a
// guilda junto, e só ele.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
)

// entraNaGuilda põe mais um personagem de uma conta nova na guilda.
func entraNaGuilda(ctx context.Context, t *testing.T, s *Store, conta int64, nome string, guilda int) {
	t.Helper()
	var charID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO character (account_id, slot, name, class, level, guild_id, guild_level)
		VALUES ($1, 0, $2, 1, 50, $3, 0) RETURNING id`,
		conta, nome, guilda).Scan(&charID); err != nil {
		t.Fatalf("criando o personagem %s: %v", nome, err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO guild_member (guild_id, character_id, account_id, slot, name, guild_level)
		VALUES ($1, $2, $3, 0, $4, 0)`, guilda, charID, conta, nome); err != nil {
		t.Fatalf("criando o membro %s: %v", nome, err)
	}
}

func guildaExiste(ctx context.Context, t *testing.T, s *Store, id int) bool {
	t.Helper()
	var existe bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM guild WHERE id = $1)`, id).Scan(&existe); err != nil {
		t.Fatalf("consultando a guilda %d: %v", id, err)
	}
	return existe
}

// TestUltimoASairApagaAGuilda: o caso de produção de 28/09/2026 — a criadora sai
// e a guilda some, levando o que pendia dela.
func TestUltimoASairApagaAGuilda(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "vazia_lider")
	guildaComLider(ctx, t, s, "Teste", 4100, conta, 9)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO guild_buff (guild_id, buff_type, expires_at) VALUES (4100, 1, now() + interval '1 hour')`); err != nil {
		t.Fatalf("criando o buff: %v", err)
	}

	apagada, err := s.LeaveGuild(ctx, conta, 0)
	if err != nil {
		t.Fatalf("LeaveGuild: %v", err)
	}
	if apagada != 4100 {
		t.Fatalf("apagada = %d, queria 4100", apagada)
	}
	if guildaExiste(ctx, t, s, 4100) {
		t.Fatal("a guilda continuou no banco sem ninguém")
	}
	var buffs int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM guild_buff WHERE guild_id = 4100`).Scan(&buffs); err != nil {
		t.Fatalf("contando buffs: %v", err)
	}
	if buffs != 0 {
		t.Fatalf("sobraram %d buffs da guilda apagada", buffs)
	}
	// O nome fica livre: o /create consegue de novo.
	if _, err := s.CreateGuild(ctx, conta, 0, "Teste_p", "Teste", 7, 1, 1, 0); err != nil {
		t.Fatalf("recriar a guilda com o mesmo nome: %v", err)
	}
}

// TestGuildaComGenteFica: só o ÚLTIMO apaga. Quem sai antes deixa a guilda de pé.
func TestGuildaComGenteFica(t *testing.T) {
	s, ctx := freshStore(t)
	lider := contaPix(ctx, t, s, "fica_lider")
	membro := contaPix(ctx, t, s, "fica_membro")
	guildaComLider(ctx, t, s, "Ficam", 4200, lider, 9)
	entraNaGuilda(ctx, t, s, membro, "Ficam_m", 4200)

	apagada, err := s.LeaveGuild(ctx, lider, 0)
	if err != nil || apagada != 0 {
		t.Fatalf("saída do líder: apagada=%d err=%v, queria 0 e nil", apagada, err)
	}
	if !guildaExiste(ctx, t, s, 4200) {
		t.Fatal("a guilda sumiu com um membro dentro")
	}
	apagada, err = s.LeaveGuild(ctx, membro, 0)
	if err != nil || apagada != 4200 {
		t.Fatalf("saída do último: apagada=%d err=%v, queria 4200 e nil", apagada, err)
	}
}

// TestSaveAntesDaSaidaNaoEscondeAGuilda: o tmServer salva o personagem com a
// guilda já zerada ANTES de pedir a saída, e o save pode chegar primeiro. A
// guilda tem de ser achada por guild_member mesmo assim.
func TestSaveAntesDaSaidaNaoEscondeAGuilda(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "save_primeiro")
	guildaComLider(ctx, t, s, "Corrida", 4300, conta, 9)
	if _, err := s.pool.Exec(ctx,
		`UPDATE character SET guild_id = 0, guild_level = 0 WHERE account_id = $1`, conta); err != nil {
		t.Fatalf("simulando o save: %v", err)
	}

	apagada, err := s.LeaveGuild(ctx, conta, 0)
	if err != nil || apagada != 4300 {
		t.Fatalf("apagada=%d err=%v, queria 4300 e nil", apagada, err)
	}
}

// TestGuildaVaziaComCidadeFica: guilda dona de cidade não é apagada, porque a
// cidade guarda o id sem chave estrangeira e o número seria reaproveitado.
func TestGuildaVaziaComCidadeFica(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "cidade_lider")
	guildaComLider(ctx, t, s, "Cidade", 4400, conta, 9)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO guild_zone (zone, charge_guild) VALUES (0, 4400)
		ON CONFLICT (zone) DO UPDATE SET charge_guild = 4400`); err != nil {
		t.Fatalf("dando a cidade: %v", err)
	}

	apagada, err := s.LeaveGuild(ctx, conta, 0)
	if err != nil || apagada != 0 {
		t.Fatalf("apagada=%d err=%v, queria 0 e nil", apagada, err)
	}
	if !guildaExiste(ctx, t, s, 4400) {
		t.Fatal("apagou a guilda que é dona de cidade")
	}
}
