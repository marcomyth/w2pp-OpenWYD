package grpcsrv

import (
	"context"
	"testing"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestRenascimentoDosChefesVaiPresente: o campo sai sempre marcado como
// enviado — é a presença que diz ao tmServer que este dbServer já conhece a
// migração 0056.
func TestRenascimentoDosChefesVaiPresente(t *testing.T) {
	st := &fakeWorldEventStore{cfg: domain.WorldEventConfig{BossRespawnHours: 36}}
	resp, err := NewWorldEventConfig(st).GetWorldEventConfig(context.Background(), &dbv1.GetWorldEventConfigRequest{})
	if err != nil {
		t.Fatalf("GetWorldEventConfig: %v", err)
	}
	cfg := resp.GetConfig()
	if cfg.BossRespawnHours == nil || *cfg.BossRespawnHours != 36 {
		t.Errorf("boss_respawn_hours = %v, want presente e 36", cfg.BossRespawnHours)
	}
}

// TestTetoDaRodadaVaiComCincoValores: as duas listas saem sempre com as cinco
// faixas — o tamanho é o que diz ao tmServer que este dbServer conhece a 0073.
func TestTetoDaRodadaVaiComCincoValores(t *testing.T) {
	st := &fakeWorldEventStore{cfg: domain.DefaultWorldEventConfig()}
	resp, err := NewWorldEventConfig(st).GetWorldEventConfig(context.Background(), &dbv1.GetWorldEventConfigRequest{})
	if err != nil {
		t.Fatalf("GetWorldEventConfig: %v", err)
	}
	cfg := resp.GetConfig()
	if len(cfg.RoundXpCap) != 5 || len(cfg.RoundXpCapDouble) != 5 || cfg.RoundXpCap[2] != 500454 || cfg.RoundXpCapDouble[2] != 1000908 {
		t.Errorf("round_xp_cap = %v / %v, want as cinco faixas", cfg.RoundXpCap, cfg.RoundXpCapDouble)
	}
}
