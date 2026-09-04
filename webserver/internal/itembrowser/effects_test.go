package itembrowser

import (
	"os"
	"regexp"
	"testing"
)

// TestEveryIDHasAMeaning keeps the two legend tables in step: an id without a
// label would render as a blank row in the UI.
func TestEveryIDHasAMeaning(t *testing.T) {
	for name := range effectIDs {
		def, ok := effectDefs[name]
		if !ok {
			t.Errorf("%s has an id but no description", name)
			continue
		}
		if def[0] == "" {
			t.Errorf("%s has an empty label", name)
		}
	}
	for name := range effectDefs {
		if _, ok := effectIDs[name]; !ok {
			t.Errorf("%s is described but has no id", name)
		}
	}
}

// TestFlagListsReferenceKnownTokens catches a typo in the score/refine lists,
// which would otherwise silently mark nothing.
func TestFlagListsReferenceKnownTokens(t *testing.T) {
	for _, list := range []struct {
		name  string
		items map[string]bool
	}{{"scored", scored}, {"noRefine", noRefine}} {
		for name := range list.items {
			if _, ok := effectIDs[name]; !ok {
				t.Errorf("%s lists unknown token %s", list.name, name)
			}
		}
	}
}

func TestEffectTableFlags(t *testing.T) {
	table := EffectTable()

	// EF_DAMAGE is a plain magnitude: it feeds the score model and refine scales it.
	if got := table["EF_DAMAGE"]; got.ID != 2 || !got.Score || !got.Refine {
		t.Errorf("EF_DAMAGE = %+v; want id 2, score, refine", got)
	}
	// EF_RANGE is explicitly exempt from the refine multiplier (Basedef.cpp:1854)
	// and is not a score stat.
	if got := table["EF_RANGE"]; got.ID != 27 || got.Score || got.Refine {
		t.Errorf("EF_RANGE = %+v; want id 27, no score, no refine", got)
	}
	// EF_VOLATILE routes the use-item handler; it is neither.
	if got := table["EF_VOLATILE"]; got.ID != 38 || got.Score || got.Refine {
		t.Errorf("EF_VOLATILE = %+v; want id 38, no score, no refine", got)
	}
}

// TestIDsMatchLegacyHeader pins every id against Source/Code/ItemEffect.h, the
// authority. It skips when the legacy tree is absent (the header is reference
// material, not a build input). Commented-out defines are ignored: EF_NONE and
// EF_PRICE are disabled there but still worth describing in the UI.
func TestIDsMatchLegacyHeader(t *testing.T) {
	const header = "../../../Source/Code/ItemEffect.h"
	raw, err := os.ReadFile(header)
	if err != nil {
		t.Skip("legacy Source tree not present")
	}
	// The file is CP949; only the ASCII #define part is read, so a byte-wise
	// regex is enough and avoids pulling in an encoding dependency.
	re := regexp.MustCompile(`(?m)^[ \t]*#define[ \t]+(EF_\w+)[ \t]+(\d+)`)
	found := 0
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		name, want := m[1], m[2]
		id, ok := effectIDs[name]
		if !ok {
			t.Errorf("%s is defined in ItemEffect.h but missing from the legend", name)
			continue
		}
		if got := itoa(id); got != want {
			t.Errorf("%s id = %s, ItemEffect.h says %s", name, got, want)
		}
		found++
	}
	if found < 90 {
		t.Errorf("only %d defines parsed from %s; the header layout probably changed", found, header)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
