package protocol

import "testing"

// The client reads single-byte Windows-1252 (Language.txt:158 stores the cedilla
// of "combinação" as 0xE7). A Go literal is UTF-8, so without this conversion
// every accented server message reaches the player as mojibake.
func TestClientText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []byte
	}{
		{"ascii passes through", "Level up", []byte("Level up")},
		{"cedilla is one byte", "ção", []byte{0xE7, 0xE3, 'o'}},
		{"the Language.txt line", "conclu\u00eddo", []byte{'c', 'o', 'n', 'c', 'l', 'u', 0xED, 'd', 'o'}},
		{"em dash maps into the 0x80 block", "a—b", []byte{'a', 0x97, 'b'}},
		{"unmappable degrades to ?", "日", []byte{'?'}},
		{"empty", "", []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClientText(tt.in)
			if string(got) != string(tt.want) {
				t.Errorf("ClientText(%q) = % x, want % x", tt.in, got, tt.want)
			}
		})
	}
}

// A UTF-8 accent is two bytes; the panel must not ship those, and must not
// exceed the 94 bytes the legacy leaves usable inside String[128].
func TestEncodeMessagePanelBody(t *testing.T) {
	body := EncodeMessagePanelBody("Processo de combinação foi concluído.")
	if len(body) != messagePanelSize {
		t.Fatalf("body = %d bytes, want %d", len(body), messagePanelSize)
	}
	// "combina" + 0xE7 — one byte, not the 0xC3 0xA7 of UTF-8.
	if got := body[19]; got != 0xE7 {
		t.Errorf("cedilla byte = %#x, want 0xE7 (UTF-8 leaked into the panel)", got)
	}
	long := ""
	for range 200 {
		long += "á"
	}
	body = EncodeMessagePanelBody(long)
	if len(body) != messagePanelSize {
		t.Fatalf("oversized body = %d bytes, want %d", len(body), messagePanelSize)
	}
	if body[messagePanelTextMax] != 0 {
		t.Errorf("text ran past byte %d — the legacy NULs the tail", messagePanelTextMax)
	}
}

func TestFromClientText(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"ascii", []byte("Refine"), "Refine"},
		{"latin1", []byte{'R', 'e', 'f', 'i', 'n', 'a', 0xE7, 0xE3, 'o'}, "Refinação"},
		{"cp1252 punct", []byte{'a', 0x97, 'b'}, "a—b"},
		{"unassigned byte", []byte{'a', 0x81, 'b'}, "a?b"},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FromClientText(c.in); got != c.want {
				t.Errorf("FromClientText(%v)=%q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Text that survives ClientText must come back unchanged: this is the round trip
// a Language.txt line takes on its way to the player.
func TestClientTextRoundTrip(t *testing.T) {
	for _, s := range []string{
		"Obteve sucesso na refinação.",
		"Uso restrito ao líder do grupo.",
		"Não é possível durante a auto venda.",
		"8ª Skill pode ser somente da 1ª Classe.",
		"plain ascii",
	} {
		if got := FromClientText(ClientText(s)); got != s {
			t.Errorf("round trip of %q gave %q", s, got)
		}
	}
}
