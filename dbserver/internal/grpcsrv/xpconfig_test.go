package grpcsrv

import (
	"context"
	"testing"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

type fakeXPStore struct {
	version int64
	cfg     domain.XPConfig
}

func (f *fakeXPStore) XPConfigVersion(context.Context) (int64, error) { return f.version, nil }
func (f *fakeXPStore) XPConfig(context.Context) (domain.XPConfig, error) {
	return f.cfg, nil
}

// TestXPConfigServerKeepsEmptyApartFromUnedited is the mapping that carries the
// whole meaning of the table: a branch edited to have no cuts and a branch never
// touched are opposite configurations, and proto3 cannot tell an absent repeated
// field from an empty one. has_cuts is what keeps them apart on the wire.
func TestXPConfigServerKeepsEmptyApartFromUnedited(t *testing.T) {
	st := &fakeXPStore{cfg: domain.XPConfig{
		Version: 7,
		Rules: []domain.XPRule{
			{Zone: 0, Tier: 2, RatePercent: 150, Cuts: nil},
			{Zone: 3, Tier: 2, RatePercent: 100, Cuts: []domain.XPCut{}},
			{Zone: 4, Tier: 1, RatePercent: 100, Cuts: []domain.XPCut{{UpTo: 200, Divisor: 1.5}}},
		},
	}}
	s := NewXPConfig(st)

	resp, err := s.GetXPConfig(context.Background(), &dbv1.GetXPConfigRequest{})
	if err != nil {
		t.Fatalf("GetXPConfig: %v", err)
	}
	if resp.GetVersion() != 7 {
		t.Errorf("version = %d, want 7", resp.GetVersion())
	}
	if len(resp.GetRules()) != 3 {
		t.Fatalf("got %d rules, want 3", len(resp.GetRules()))
	}
	if resp.GetRules()[0].GetHasCuts() {
		t.Error("a nil cut table must cross the wire as has_cuts=false (keep the legacy table)")
	}
	if !resp.GetRules()[1].GetHasCuts() || len(resp.GetRules()[1].GetCuts()) != 0 {
		t.Error("an edited-but-empty table must cross as has_cuts=true with no cuts")
	}
	third := resp.GetRules()[2]
	if !third.GetHasCuts() || len(third.GetCuts()) != 1 ||
		third.GetCuts()[0].GetUpTo() != 200 || third.GetCuts()[0].GetDivisor() != 1.5 {
		t.Errorf("third rule = %+v", third)
	}
}

func TestXPConfigVersionIsServed(t *testing.T) {
	s := NewXPConfig(&fakeXPStore{version: 42})
	got, err := s.XPConfigVersion(context.Background(), &dbv1.XPConfigVersionRequest{})
	if err != nil || got.GetVersion() != 42 {
		t.Fatalf("XPConfigVersion = (%v, %v), want 42", got, err)
	}
}

// TestXPConfigEmptyIsTheLegacy: a fresh server has no rows, and the reply has to
// say so plainly rather than by omission — tmServer reads an empty list as "run
// the legacy tables".
func TestXPConfigEmptyIsTheLegacy(t *testing.T) {
	s := NewXPConfig(&fakeXPStore{})
	resp, err := s.GetXPConfig(context.Background(), &dbv1.GetXPConfigRequest{})
	if err != nil {
		t.Fatalf("GetXPConfig: %v", err)
	}
	if len(resp.GetRules()) != 0 || resp.GetVersion() != 0 {
		t.Fatalf("resp = %+v, want empty at version 0", resp)
	}
}
