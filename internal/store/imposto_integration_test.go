//go:build integration

// Testes de integração da hora da troca de imposto (guild_zone.tax_changed_at).
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func zonaDoBanco(t *testing.T, s *Store, zona int) domain.GuildZone {
	t.Helper()
	zs, err := s.LoadGuildZones(t.Context())
	if err != nil {
		t.Fatalf("LoadGuildZones: %v", err)
	}
	for _, z := range zs {
		if z.Zone == zona {
			return z
		}
	}
	t.Fatalf("zona %d não voltou do banco", zona)
	return domain.GuildZone{}
}

// A hora da troca e o ponto de renascimento sobrevivem à gravação, e "nunca" é NULL.
func TestImpostoHoraDaTrocaNoBanco(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "imposto_lider")
	guildaComLider(ctx, t, s, "Imposto", 4210, conta, 9)

	// Primeiro a guilda vira dona (no jogo isso vem antes de qualquer troca de
	// imposto); trocar de dono na mesma gravação zera a espera, de propósito.
	z := domain.GuildZone{Zone: 3, ChargeGuild: 4210, CityTax: 0, GuildSpawnX: 3652, GuildSpawnY: 3122}
	if err := s.SaveGuildZone(ctx, z); err != nil {
		t.Fatalf("SaveGuildZone dono: %v", err)
	}
	quando := time.Date(2026, 9, 29, 20, 0, 0, 0, time.UTC)
	z.CityTax, z.TaxChangedAt = 12, quando
	if err := s.SaveGuildZone(ctx, z); err != nil {
		t.Fatalf("SaveGuildZone: %v", err)
	}
	got := zonaDoBanco(t, s, 3)
	if !got.TaxChangedAt.Equal(quando) || got.CityTax != 12 || got.GuildSpawnX != 3652 {
		t.Fatalf("voltou %+v", got)
	}

	// Mesmo dono, sem troca nova: a hora continua.
	z.CityTax = 12
	if err := s.SaveGuildZone(ctx, z); err != nil {
		t.Fatalf("SaveGuildZone de novo: %v", err)
	}
	if got := zonaDoBanco(t, s, 3); !got.TaxChangedAt.Equal(quando) {
		t.Errorf("a hora mudou sem troca: %v", got.TaxChangedAt)
	}
}

// Dono novo começa sem a espera do anterior.
func TestImpostoDonoNovoNaoHerdaAEspera(t *testing.T) {
	s, ctx := freshStore(t)
	a := contaPix(ctx, t, s, "imposto_a")
	b := contaPix(ctx, t, s, "imposto_b")
	guildaComLider(ctx, t, s, "ImpostoA", 4211, a, 9)
	guildaComLider(ctx, t, s, "ImpostoB", 4212, b, 9)

	quando := time.Date(2026, 9, 29, 20, 0, 0, 0, time.UTC)
	if err := s.SaveGuildZone(ctx, domain.GuildZone{Zone: 4, ChargeGuild: 4211, CityTax: 10, TaxChangedAt: quando}); err != nil {
		t.Fatalf("SaveGuildZone A: %v", err)
	}
	if err := s.SaveGuildZone(ctx, domain.GuildZone{Zone: 4, ChargeGuild: 4212, CityTax: 10, TaxChangedAt: quando}); err != nil {
		t.Fatalf("SaveGuildZone B: %v", err)
	}
	if got := zonaDoBanco(t, s, 4); !got.TaxChangedAt.IsZero() {
		t.Errorf("a guilda nova herdou a espera: %v", got.TaxChangedAt)
	}
}

// O dono trocado À MÃO (painel, UPDATE direto) também zera a espera: a regra é do
// gatilho do banco, não só do Go.
func TestImpostoDonoTrocadoAMaoZeraAEspera(t *testing.T) {
	s, ctx := freshStore(t)
	a := contaPix(ctx, t, s, "imposto_mao_a")
	b := contaPix(ctx, t, s, "imposto_mao_b")
	guildaComLider(ctx, t, s, "ImpostoMaoA", 4213, a, 9)
	guildaComLider(ctx, t, s, "ImpostoMaoB", 4214, b, 9)

	if err := s.SaveGuildZone(ctx, domain.GuildZone{Zone: 1, ChargeGuild: 4213}); err != nil {
		t.Fatalf("SaveGuildZone dono: %v", err)
	}
	quando := time.Date(2026, 10, 3, 23, 0, 0, 0, time.UTC)
	if err := s.SaveGuildZone(ctx, domain.GuildZone{Zone: 1, ChargeGuild: 4213, CityTax: 9, TaxChangedAt: quando}); err != nil {
		t.Fatalf("SaveGuildZone imposto: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE guild_zone SET charge_guild = 4214 WHERE zone = 1`); err != nil {
		t.Fatalf("UPDATE à mão: %v", err)
	}
	if got := zonaDoBanco(t, s, 1); !got.TaxChangedAt.IsZero() {
		t.Errorf("o dono novo herdou a espera: %v", got.TaxChangedAt)
	}
	// Outra coluna mudando não mexe na hora.
	if err := s.SaveGuildZone(ctx, domain.GuildZone{Zone: 1, ChargeGuild: 4214, CityTax: 3, TaxChangedAt: quando}); err != nil {
		t.Fatalf("SaveGuildZone B: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE guild_zone SET tax_vault = 500 WHERE zone = 1`); err != nil {
		t.Fatalf("UPDATE do cofre: %v", err)
	}
	if got := zonaDoBanco(t, s, 1); !got.TaxChangedAt.Equal(quando) {
		t.Errorf("mexer no cofre zerou a hora: %v", got.TaxChangedAt)
	}
}
