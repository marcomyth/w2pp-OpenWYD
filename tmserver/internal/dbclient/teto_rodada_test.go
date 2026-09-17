package dbclient

import (
	"context"
	"testing"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestTetoDaRodadaPresencaNoFio: cinco valores valem como vieram; nenhum (um
// dbServer de antes da 0073) ou um tamanho errado roda os tetos decididos, nunca
// zeros, que desligariam o teto em todas as faixas durante um deploy.
func TestTetoDaRodadaPresencaNoFio(t *testing.T) {
	cinco := []int64{1, 2, 3, 4, 5}
	tests := []struct {
		nome            string
		cfg             *dbv1.WorldEventConfig
		teto, tetoDobro [5]int64
	}{
		{"ausente é o padrão", &dbv1.WorldEventConfig{NoticeEnabled: true}, domain.DefaultRoundXPCap, domain.DefaultRoundXPCapDouble},
		{"sem config é o padrão", nil, domain.DefaultRoundXPCap, domain.DefaultRoundXPCapDouble},
		{"tamanho errado é o padrão", &dbv1.WorldEventConfig{RoundXpCap: []int64{1, 2}}, domain.DefaultRoundXPCap, domain.DefaultRoundXPCapDouble},
		{"presente vale como veio", &dbv1.WorldEventConfig{RoundXpCap: cinco, RoundXpCapDouble: []int64{0, 0, 0, 0, 9}},
			[5]int64{1, 2, 3, 4, 5}, [5]int64{0, 0, 0, 0, 9}},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			c := &WorldEventConfig{api: &fakeWorldEventAPI{snapshotResp: &dbv1.GetWorldEventConfigResponse{
				Version: 1, Config: tt.cfg,
			}}}
			snap, err := c.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
			if snap.Event.RoundXPCap != tt.teto || snap.Event.RoundXPCapDouble != tt.tetoDobro {
				t.Errorf("teto %v / %v, want %v / %v", snap.Event.RoundXPCap, snap.Event.RoundXPCapDouble, tt.teto, tt.tetoDobro)
			}
		})
	}
}
