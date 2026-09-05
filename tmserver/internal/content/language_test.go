package content

import (
	"strings"
	"testing"
)

func TestParseLanguage(t *testing.T) {
	// Byte 0xE7 is the CP1252 cedilla and 0xE3 the a-tilde: the loader must decode
	// them, not carry them as invalid UTF-8.
	src := "74 _NN_Only_To_Equips\tPoss\xedvel somente com armas.\n" +
		"176 _NN_Refine_Success\tObteve sucesso na refina\xe7\xe3o.\n" +
		"547 _NN_TP_GELO \tTeleportado para Gelo.\n" +
		"\n" +
		"nao-e-uma-linha\n"

	l, err := ParseLanguage(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parseLanguage: %v", err)
	}
	if l.Len() != 3 {
		t.Fatalf("Len=%d, want 3", l.Len())
	}
	cases := []struct{ key, want string }{
		{"_NN_Only_To_Equips", "Possível somente com armas."},
		{"_NN_Refine_Success", "Obteve sucesso na refinação."},
		// The one line in the shipped file whose key carries a trailing space.
		{"_NN_TP_GELO", "Teleportado para Gelo."},
	}
	for _, c := range cases {
		got, ok := l.Text(c.key)
		if !ok {
			t.Errorf("Text(%q) not found", c.key)
			continue
		}
		if got != c.want {
			t.Errorf("Text(%q)=%q, want %q", c.key, got, c.want)
		}
	}
	if _, ok := l.Text("_NN_Nao_Existe"); ok {
		t.Error("Text of an unknown key reported found")
	}
	if got, ok := l.TextByID(176); !ok || got != "Obteve sucesso na refinação." {
		t.Errorf("TextByID(176)=%q,%v", got, ok)
	}
}

// A nil table must answer like an empty one: the server runs without -content,
// and every notify() would otherwise panic.
func TestLanguageNil(t *testing.T) {
	var l *Language
	if _, ok := l.Text("_NN_Refine_Success"); ok {
		t.Error("nil Language reported a hit")
	}
	if _, ok := l.TextByID(1); ok {
		t.Error("nil Language reported a hit by id")
	}
	if l.Len() != 0 {
		t.Errorf("nil Language Len=%d, want 0", l.Len())
	}
}

func TestParseLanguageEmpty(t *testing.T) {
	if _, err := ParseLanguage(strings.NewReader("# nada aqui\n")); err == nil {
		t.Fatal("parseLanguage accepted a file with no usable lines")
	}
}

func TestLoadLanguageFromRelease(t *testing.T) {
	l, err := LoadLanguage(release(t, "TMsrv", "run", "Language.txt"))
	if err != nil {
		t.Skipf("Language.txt unavailable: %v", err)
	}
	if l.Len() < 500 {
		t.Errorf("Len=%d, want the shipped file's ~547 lines", l.Len())
	}
	// Spot-check the accents survived the CP1252 decode end to end.
	if got, _ := l.Text("_NN_Refine_Success"); got != "Obteve sucesso na refinação." {
		t.Errorf("_NN_Refine_Success=%q", got)
	}
	if got, _ := l.Text("_NN_Party_Leader_Only"); got != "Uso restrito ao líder do grupo." {
		t.Errorf("_NN_Party_Leader_Only=%q", got)
	}
	// U+FFFD is what a UTF-8 read of this file would leave behind.
	for _, key := range []string{"_NN_Cant_Refine_More", "_NN_Not_Enough_Money", "_NN_CANT_USE_NIGHTMARE"} {
		got, ok := l.Text(key)
		if !ok {
			t.Errorf("%s missing from the shipped table", key)
			continue
		}
		if strings.ContainsRune(got, '�') {
			t.Errorf("%s=%q contains a replacement rune (decoded as UTF-8?)", key, got)
		}
	}
}
