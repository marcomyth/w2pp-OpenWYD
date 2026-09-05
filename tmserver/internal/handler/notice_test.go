package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
)

func testDispatcherWithLanguage(t *testing.T) (*Dispatcher, *content.Language) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "Language.txt")
	lang, err := content.LoadLanguage(path)
	if err != nil {
		t.Skipf("Language.txt unavailable: %v", err)
	}
	return New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Language: lang}), lang
}

// Every key in noticeKey must exist in the shipped table. A typo would otherwise
// be invisible: the notice would silently fall back to nothing, which is exactly
// the failure this whole path exists to remove.
func TestNoticeKeysExistInRelease(t *testing.T) {
	_, lang := testDispatcherWithLanguage(t)
	for n, key := range noticeKey {
		if _, ok := lang.Text(key); !ok {
			t.Errorf("notice %d maps to %q, which is not in Language.txt", n, key)
		}
	}
}

// The mapped notices must actually produce text against the real content tree.
// TestNoticeKeysExistInRelease proves the key resolves; this proves nothing else
// (an unnoticed format verb, say) drops the line on the way out.
func TestNoticeLineFromRelease(t *testing.T) {
	d, _ := testDispatcherWithLanguage(t)
	cases := []struct {
		n    Notice
		want string
	}{
		{NoticeRefineSuccess, "Obteve sucesso na refinação."},
		{NoticePartyLeaderOnly, "Uso restrito ao líder do grupo."},
		{NoticePesadeloClosed, "Pesadelo disponível entre 18h e 24h."},
		{NoticeNotConnected, "O jogador não está conectado."},
		{NoticeNoKey, "Você não possui uma chave."},
	}
	for _, c := range cases {
		got, ok := d.noticeLine(c.n)
		if !ok {
			t.Errorf("notice %d produced no line", c.n)
			continue
		}
		if got != c.want {
			t.Errorf("notice %d line = %q, want %q", c.n, got, c.want)
		}
	}
	for n := range noticeKey {
		if got, ok := d.noticeLine(n); !ok || strings.TrimSpace(got) == "" {
			t.Errorf("mapped notice %d produced no usable line (%q, %v)", n, got, ok)
		}
	}
}

// Without a content tree the compiled fallbacks must still reach the player:
// this is the pre-existing behaviour for refine and paint, and losing it would
// make an unmounted server quieter than before.
func TestNoticeLineFallsBackWithoutLanguage(t *testing.T) {
	d := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if got, ok := d.noticeLine(NoticeRefineSuccess); !ok || got != "Obteve sucesso na refinação." {
		t.Errorf("noticeLine(NoticeRefineSuccess)=%q,%v with no Language", got, ok)
	}
	if got, ok := d.noticeLine(NoticePaintSuccess); !ok || got != "Item pintado com sucesso." {
		t.Errorf("noticeLine(NoticePaintSuccess)=%q,%v", got, ok)
	}
	// A notice with neither a key nor a fallback stays silent, which is what the
	// legacy does on that path.
	if got, ok := d.noticeLine(NoticeReqNotMet); ok {
		t.Errorf("noticeLine(NoticeReqNotMet)=%q, want no line", got)
	}
}

// A line that interpolates arguments must never go out raw: the wire format
// carries no values to fill it, so the player would read "%d". An operator who
// edits Language.txt into a formatted string is the realistic way this happens,
// so the table here overrides two shipped keys rather than inventing new ones.
func TestNoticeLineRejectsFormatVerbs(t *testing.T) {
	// CP1252 bytes, like the shipped file: 0xEA is e-circumflex, 0xE3 a-tilde.
	src := "76 _NN_Fail_To_Refine\tFalhou por %d motivos.\n" +
		"79 _NN_No_Key\tVoc\xea n\xe3o possui uma chave.\n"
	lang, err := content.ParseLanguage(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	d := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Language: lang})

	// The formatted line is dropped, so the compiled fallback answers instead.
	if got, ok := d.noticeLine(NoticeFailToRefine); !ok || got != "Refinação falhou." {
		t.Errorf("noticeLine(NoticeFailToRefine)=%q,%v, want the compiled fallback", got, ok)
	}
	// NoticeNoKey has no fallback, so a formatted line would leave it silent —
	// here it is plain, and must come through.
	if got, ok := d.noticeLine(NoticeNoKey); !ok || got != "Você não possui uma chave." {
		t.Errorf("noticeLine(NoticeNoKey)=%q,%v", got, ok)
	}
}

func TestFormatVerb(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"Você precisa de %d Gold.", true},
		{"Número vencedor: %2d %2d", true},
		{"Imposto da cidade é %llu.", true},
		{"Conta bloqueada Grade %s, entre em contato.", true},
		{"A taxa foi ajustada para %d%%.", true},
		{"Taxa de drop reduzida em 50%. Descanse!", false},
		{"Obteve sucesso na refinação.", false},
		{"", false},
	}
	for _, c := range cases {
		if got := formatVerb.MatchString(c.text); got != c.want {
			t.Errorf("formatVerb(%q)=%v, want %v", c.text, got, c.want)
		}
	}
}
