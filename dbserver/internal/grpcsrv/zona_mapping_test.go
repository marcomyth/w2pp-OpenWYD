package grpcsrv

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// A ZONA TEM DE VOLTAR INTEIRA DO PROTO. O tmServer grava a zona toda a cada troca
// de imposto; campo que se perde aqui é gravado como zero no banco. Foi o que
// acontecia com o ponto de renascimento da guilda até 29/09/2026.
func TestZonaIdaEVoltaPeloProto(t *testing.T) {
	quando := time.Date(2026, 9, 29, 20, 0, 0, 0, time.UTC)
	z := domain.GuildZone{Zone: 2, ChargeGuild: 7, CityTax: 15, TaxVault: 1234,
		GuildSpawnX: 2453, GuildSpawnY: 2000, TaxChangedAt: quando}
	got := zoneFromProto(zoneToProto(z))
	if got.GuildSpawnX != 2453 || got.GuildSpawnY != 2000 {
		t.Errorf("o ponto de renascimento se perdeu: %d,%d", got.GuildSpawnX, got.GuildSpawnY)
	}
	if !got.TaxChangedAt.Equal(quando) {
		t.Errorf("a hora da troca do imposto virou %v", got.TaxChangedAt)
	}
	if nunca := zoneFromProto(zoneToProto(domain.GuildZone{Zone: 1})); !nunca.TaxChangedAt.IsZero() {
		t.Errorf("\"nunca mudou\" voltou como %v", nunca.TaxChangedAt)
	}
}
