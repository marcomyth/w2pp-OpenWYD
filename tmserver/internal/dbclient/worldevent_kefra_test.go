package dbclient

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
)

// TestSetKefraStateVaiAoDbServer: o cliente manda o estado e a guilda como
// vieram e devolve a versão que o dbServer respondeu.
func TestSetKefraStateVaiAoDbServer(t *testing.T) {
	api := &fakeWorldEventAPI{}
	v, err := (&WorldEventConfig{api: api}).SetKefraState(context.Background(), true, 21)
	if err != nil {
		t.Fatalf("SetKefraState: %v", err)
	}
	if api.kefraReq == nil || !api.kefraReq.GetLive() || api.kefraReq.GetGuildId() != 21 {
		t.Errorf("pedido = %+v, want live=true guild=21", api.kefraReq)
	}
	if v != 9 {
		t.Errorf("versão = %d, want 9", v)
	}
}

// TestSnapshotLeAGuildaDoKefra: presente vale como veio; ausente (dbServer antes
// da 0067) é 0, sem guilda.
func TestSnapshotLeAGuildaDoKefra(t *testing.T) {
	for _, c := range []struct {
		nome string
		cfg  *dbv1.WorldEventConfig
		quer int32
	}{
		{"presente", &dbv1.WorldEventConfig{KefraLiveEnabled: true, KefraGuildId: proto.Int32(5)}, 5},
		{"ausente", &dbv1.WorldEventConfig{KefraLiveEnabled: true}, 0},
	} {
		t.Run(c.nome, func(t *testing.T) {
			api := &fakeWorldEventAPI{snapshotResp: &dbv1.GetWorldEventConfigResponse{Version: 1, Config: c.cfg}}
			snap, err := (&WorldEventConfig{api: api}).Snapshot(context.Background())
			if err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
			if snap.Event.KefraGuildID != c.quer {
				t.Errorf("KefraGuildID = %d, want %d", snap.Event.KefraGuildID, c.quer)
			}
		})
	}
}
