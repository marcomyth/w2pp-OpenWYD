package itembrowser

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCommandReferenceIsWellFormed(t *testing.T) {
	bus := CommandReference()
	if bus.Entry == "" || bus.Access == "" || len(bus.Notes) == 0 {
		t.Fatalf("bus header is incomplete: %+v", bus)
	}
	seen := make(map[string]bool)
	for _, c := range bus.Commands {
		if c.Name == "" || c.Summary == "" || c.Target == "" {
			t.Errorf("command %q is missing a name, summary or target", c.Name)
		}
		for _, token := range append([]string{c.Name}, c.Aliases...) {
			if seen[token] {
				t.Errorf("token %q is listed twice", token)
			}
			seen[token] = true
		}
	}
}

// TestCommandsMatchDispatch pins the reference against the real dispatch switch
// in tmserver/internal/handler/gm.go. Documenting a command the server does not
// route — or forgetting one it does — is exactly the failure this tab must not
// have, so it fails the build instead of misleading a GM.
func TestCommandsMatchDispatch(t *testing.T) {
	const dispatch = "../../../tmserver/internal/handler/gm.go"
	raw, err := os.ReadFile(dispatch)
	if err != nil {
		t.Skip("tmserver source not present")
	}

	// Every `case "x", "y":` inside runGMCommand's switch. The file has exactly
	// one such switch; a second one would show up here as unexpected tokens.
	caseRe := regexp.MustCompile(`(?m)^\s*case ("[a-z]+"(?:,\s*"[a-z]+")*):`)
	tokenRe := regexp.MustCompile(`"([a-z]+)"`)
	routed := make(map[string]bool)
	for _, m := range caseRe.FindAllStringSubmatch(string(raw), -1) {
		for _, tok := range tokenRe.FindAllStringSubmatch(m[1], -1) {
			routed[tok[1]] = true
		}
	}
	if len(routed) == 0 {
		t.Fatalf("no case tokens parsed from %s; the switch layout changed", dispatch)
	}

	documented := make(map[string]bool)
	for _, c := range CommandReference().Commands {
		documented[c.Name] = true
		for _, a := range c.Aliases {
			documented[a] = true
		}
	}

	for tok := range routed {
		if !documented[tok] {
			t.Errorf("/gm %s is routed by the server but missing from the reference", tok)
		}
	}
	for tok := range documented {
		if !routed[tok] {
			t.Errorf("/gm %s is documented but the server does not route it", tok)
		}
	}
}

// TestCommandArgsUseAngleBrackets keeps the syntax column consistent: required
// arguments in <>, optional ones in [].
func TestCommandArgsUseAngleBrackets(t *testing.T) {
	for _, c := range CommandReference().Commands {
		if c.Args == "" {
			continue
		}
		if !strings.ContainsAny(c.Args, "<[") {
			t.Errorf("/gm %s args %q do not mark their placeholders", c.Name, c.Args)
		}
	}
}
