package grpcsrv

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestSetKefraStateGravaComAFonteDoJogo: a gravação pedida pelo tmServer é a do
// jogo — fonte "jogo", sem conta de moderador — e devolve a versão nova.
func TestSetKefraStateGravaComAFonteDoJogo(t *testing.T) {
	st := &fakeWorldEventStore{version: 4}
	resp, err := NewWorldEventConfig(st).SetKefraState(context.Background(), &dbv1.SetKefraStateRequest{Live: true, GuildId: 12})
	if err != nil {
		t.Fatalf("SetKefraState: %v", err)
	}
	if st.kefraCalls != 1 || !st.kefraLive || st.kefraGuild != 12 || st.kefraFonte != "jogo" || st.kefraConta != 0 {
		t.Errorf("gravou calls=%d live=%v guilda=%d fonte=%q conta=%d; want 1 true 12 jogo 0",
			st.kefraCalls, st.kefraLive, st.kefraGuild, st.kefraFonte, st.kefraConta)
	}
	if resp.GetVersion() != 5 {
		t.Errorf("versão = %d, want 5", resp.GetVersion())
	}
}

func TestSetKefraStateRecusaGuildaNegativa(t *testing.T) {
	st := &fakeWorldEventStore{}
	_, err := NewWorldEventConfig(st).SetKefraState(context.Background(), &dbv1.SetKefraStateRequest{Live: true, GuildId: -1})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("erro = %v, want InvalidArgument", err)
	}
	if st.kefraCalls != 0 {
		t.Error("gravou com guilda negativa")
	}
}

// TestSnapshotLevaAGuildaDoKefra: a guilda que matou vai sempre presente, até
// zerada, como os outros campos opcionais.
func TestSnapshotLevaAGuildaDoKefra(t *testing.T) {
	for _, guilda := range []int32{0, 3} {
		st := &fakeWorldEventStore{cfg: domain.WorldEventConfig{KefraLiveEnabled: guilda != 0, KefraGuildID: guilda}}
		resp, err := NewWorldEventConfig(st).GetWorldEventConfig(context.Background(), &dbv1.GetWorldEventConfigRequest{})
		if err != nil {
			t.Fatalf("GetWorldEventConfig: %v", err)
		}
		if g := resp.GetConfig().KefraGuildId; g == nil || *g != guilda {
			t.Errorf("kefra_guild_id = %v, want presente e %d", g, guilda)
		}
	}
}
