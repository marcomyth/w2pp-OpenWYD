//go:build integration

// Testes de integração da guilda que fica sem ninguém: o último a sair leva a
// guilda junto, e só ele.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
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

// TestGuildaVaziaComCidadeSaiESoltaACidade: "0 players = apagada" não tem
// exceção. A guilda dona de cidade, da torre e do Kefra sai, e as três ficam sem
// dono — senão a próxima guilda com o mesmo número herdaria tudo. Foi o caso da
// preview em 28/09/2026: a dona de Armia continuou existindo com zero membros.
func TestGuildaVaziaComCidadeSaiESoltaACidade(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "cidade_lider")
	guildaComLider(ctx, t, s, "Cidade", 4400, conta, 9)
	for _, q := range []string{
		`INSERT INTO guild_zone (zone, charge_guild, challenge_guild, challenge_money) VALUES (0, 4400, 0, 0)
		 ON CONFLICT (zone) DO UPDATE SET charge_guild = 4400`,
		`INSERT INTO guild_zone (zone, charge_guild, challenge_guild, challenge_money) VALUES (1, 0, 4400, 5000)
		 ON CONFLICT (zone) DO UPDATE SET challenge_guild = 4400, challenge_money = 5000`,
		`INSERT INTO guild_tower_state (id, owner_guild, updated_at_unix) VALUES (1, 4400, 0)
		 ON CONFLICT (id) DO UPDATE SET owner_guild = 4400`,
		`UPDATE world_event_config SET kefra_guild_id = 4400`,
	} {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			t.Fatalf("preparando %q: %v", q, err)
		}
	}

	apagada, err := s.LeaveGuild(ctx, conta, 0)
	if err != nil || apagada != 4400 {
		t.Fatalf("apagada=%d err=%v, queria 4400 e nil", apagada, err)
	}
	if guildaExiste(ctx, t, s, 4400) {
		t.Fatal("a guilda dona de cidade continuou com zero membros")
	}
	var refs int
	if err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM guild_zone WHERE charge_guild = 4400 OR challenge_guild = 4400)
		     + (SELECT count(*) FROM guild_zone WHERE zone = 1 AND challenge_money <> 0)
		     + (SELECT count(*) FROM guild_tower_state WHERE owner_guild = 4400)
		     + (SELECT count(*) FROM world_event_config WHERE kefra_guild_id = 4400)`).Scan(&refs); err != nil {
		t.Fatalf("contando referências: %v", err)
	}
	if refs != 0 {
		t.Fatalf("sobraram %d referências à guilda apagada", refs)
	}
}

// TestExpulsarOffline: o expulsar pelo banco segue as regras de cargo do online.
func TestExpulsarOffline(t *testing.T) {
	s, ctx := freshStore(t)
	lider := contaPix(ctx, t, s, "exp_lider")
	bot := contaPix(ctx, t, s, "exp_bot")
	sub := contaPix(ctx, t, s, "exp_sub")
	guildaComLider(ctx, t, s, "Bots", 4500, lider, 9)
	entraNaGuilda(ctx, t, s, bot, "Bot01", 4500)
	guildaComLider(ctx, t, s, "Outra", 4501, sub, 9)

	// Nome que não é desta guilda: recusa, sem tocar em ninguém.
	if err := s.KickOfflineGuildMember(ctx, 4500, lider, 0, "Outra_p"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("membro de outra guilda: erro = %v, queria ErrNotFound", err)
	}
	// Membro comum não expulsa ninguém (cargo 0 não é maior que 0).
	if err := s.KickOfflineGuildMember(ctx, 4500, bot, 0, "Bots_p"); !errors.Is(err, ErrConflict) {
		t.Fatalf("membro comum expulsando: erro = %v, queria ErrConflict", err)
	}
	if err := s.KickOfflineGuildMember(ctx, 4500, lider, 0, "Bot01"); err != nil {
		t.Fatalf("líder expulsando o bot: %v", err)
	}
	var guilda, membros int
	if err := s.pool.QueryRow(ctx, `SELECT guild_id FROM character WHERE name = 'Bot01'`).Scan(&guilda); err != nil {
		t.Fatalf("lendo o bot: %v", err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM guild_member WHERE guild_id = 4500`).Scan(&membros); err != nil {
		t.Fatalf("contando membros: %v", err)
	}
	if guilda != 0 || membros != 1 {
		t.Fatalf("depois de expulsar: guild_id do bot = %d, membros = %d; queria 0 e 1", guilda, membros)
	}
}
