package protocol

import "strings"

// The client renders server text as single-byte Windows-1252, not UTF-8: the
// shipped Language.txt stores "combinação" with 0xE7 for the cedilla
// (Release/TMsrv/run/Language.txt:158). Text that merely passes through from a
// content file keeps those bytes and works by accident, but any string literal
// written in Go source is UTF-8 — its accents are two bytes and reach the client
// as mojibake, which is why a message with "concluído" in it showed up as
// garbage (or not at all).
//
// ClientText converts a Go string to that encoding. Latin-1 maps one-to-one onto
// the low half of Windows-1252, so runes up to U+00FF are a direct byte write.
// The handful of punctuation runes above it that a Go author actually reaches
// for get their Windows-1252 codepoint; anything else degrades to '?' rather
// than emitting a byte the client would draw as a random glyph.
func ClientText(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x80 || (r >= 0xA0 && r <= 0xFF):
			out = append(out, byte(r))
		default:
			if b, ok := cp1252Punct[r]; ok {
				out = append(out, b)
				continue
			}
			out = append(out, '?')
		}
	}
	return out
}

// cp1252Punct is the Windows-1252 0x80-0x9F block, restricted to the runes a Go
// source string plausibly contains. Windows-1252 puts printable punctuation
// where Latin-1 has control codes, so these cannot be derived from the rune.
var cp1252Punct = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87,
	'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E,
	'‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97,
	'˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// messagePanelSize is MSG_MessagePanel.String[128] (Basedef.h:1528). The legacy
// SendClientMessage copies MESSAGE_LENGTH (96) bytes into that larger buffer and
// then NULs the last two of them (SendFunc.cpp:27), so the usable text is 94
// bytes and the rest of the 128 stays zero.
const messagePanelSize = 128

// messagePanelTextMax is how much of the panel the legacy actually fills.
const messagePanelTextMax = MessageLength - 2 // 94

// EncodeMessagePanelBody builds the MSG_MessagePanel body carrying one line of
// server text (the SendClientMessage payload). Text is encoded for the client
// and truncated to what the legacy leaves room for.
func EncodeMessagePanelBody(text string) []byte {
	body := make([]byte, messagePanelSize)
	encoded := ClientText(text)
	if len(encoded) > messagePanelTextMax {
		encoded = encoded[:messagePanelTextMax]
	}
	copy(body, encoded)
	return body
}

// TrimClientText shortens s so its encoded form fits n bytes, cutting on a rune
// boundary. Truncating the encoded bytes directly is safe for the panel (one
// byte per rune) but not for callers that build the string first.
func TrimClientText(s string, n int) string {
	if len(ClientText(s)) <= n {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		if used+len(ClientText(string(r))) > n {
			break
		}
		b.WriteRune(r)
		used += len(ClientText(string(r)))
	}
	return b.String()
}
