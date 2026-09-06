//go:build integration

// Integration tests for the Mesa de XP store (0030_xp_table). They require a
// real database and are excluded from the default build. Run with:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func TestXPRuleCRUD(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM xp_rule; UPDATE xp_rule_meta SET version = 0 WHERE id = TRUE`)

	s := New(pool)

	cfg, err := s.XPConfig(ctx)
	if err != nil {
		t.Fatalf("XPConfig: %v", err)
	}
	if len(cfg.Rules) != 0 || cfg.Version != 0 {
		t.Fatalf("a fresh table must read as empty at version 0, got %+v", cfg)
	}

	regra := domain.XPRule{Zone: 1, Tier: 2, RatePercent: 150, Cuts: []domain.XPCut{
		{UpTo: 200, Divisor: 1.5},
		{UpTo: 2147483647, Divisor: 4},
	}}
	antes, err := s.UpsertXPRule(ctx, regra, 0)
	if err != nil {
		t.Fatalf("UpsertXPRule: %v", err)
	}
	if antes.Cuts != nil || antes.RatePercent != 100 {
		t.Errorf("the previous value of an unedited branch must read as the neutral one, got %+v", antes)
	}

	cfg, err = s.XPConfig(ctx)
	if err != nil {
		t.Fatalf("XPConfig: %v", err)
	}
	if cfg.Version == 0 {
		t.Error("the version did not bump on a write")
	}
	if len(cfg.Rules) != 1 || len(cfg.Rules[0].Cuts) != 2 || cfg.Rules[0].Cuts[0].Divisor != 1.5 {
		t.Fatalf("read back %+v", cfg.Rules)
	}

	// The distinction the whole column exists for: '[]' is "no cuts", NULL is
	// "not edited". A round trip must not turn one into the other.
	vazia := domain.XPRule{Zone: 3, Tier: 2, RatePercent: 100, Cuts: []domain.XPCut{}}
	if _, err := s.UpsertXPRule(ctx, vazia, 0); err != nil {
		t.Fatalf("UpsertXPRule (empty): %v", err)
	}
	naoEditada := domain.XPRule{Zone: 4, Tier: 1, RatePercent: 250}
	if _, err := s.UpsertXPRule(ctx, naoEditada, 0); err != nil {
		t.Fatalf("UpsertXPRule (rate only): %v", err)
	}
	cfg, _ = s.XPConfig(ctx)
	for _, r := range cfg.Rules {
		switch {
		case r.Zone == 3 && r.Cuts == nil:
			t.Error("an empty table came back as nil — it now means «use the legacy's»")
		case r.Zone == 4 && r.Cuts != nil:
			t.Error("a rate-only edit came back with a cut table it never had")
		}
	}

	antes, err = s.DeleteXPRule(ctx, 1, 2)
	if err != nil {
		t.Fatalf("DeleteXPRule: %v", err)
	}
	if len(antes.Cuts) != 2 {
		t.Errorf("delete must report what it removed, got %+v", antes)
	}
	cfg, _ = s.XPConfig(ctx)
	for _, r := range cfg.Rules {
		if r.Zone == 1 && r.Tier == 2 {
			t.Error("the deleted branch is still there")
		}
	}
}

// TestXPRuleVersionBumpsOncePerWrite: tmServer boots on a version and the panel
// shows it, so a write that did not move it would leave both saying the game is
// current when it is not.
func TestXPRuleVersionBumpsOncePerWrite(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM xp_rule; UPDATE xp_rule_meta SET version = 0 WHERE id = TRUE`)
	s := New(pool)

	before, _ := s.XPConfigVersion(ctx)
	if _, err := s.UpsertXPRule(ctx, domain.XPRule{Zone: 0, Tier: 2, RatePercent: 120}, 0); err != nil {
		t.Fatalf("UpsertXPRule: %v", err)
	}
	mid, _ := s.XPConfigVersion(ctx)
	if mid != before+1 {
		t.Fatalf("version %d → %d, want +1", before, mid)
	}
	if _, err := s.DeleteXPRule(ctx, 0, 2); err != nil {
		t.Fatalf("DeleteXPRule: %v", err)
	}
	after, _ := s.XPConfigVersion(ctx)
	if after != mid+1 {
		t.Fatalf("version %d → %d on delete, want +1", mid, after)
	}
}
