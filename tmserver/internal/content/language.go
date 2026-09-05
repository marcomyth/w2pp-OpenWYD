package content

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// Language is the shipped client string table (TMsrv/run/Language.txt), the
// source of every line the legacy server sends through SendClientMessage. Lines
// are "<id> <_NN_Key>\t<text>" in Windows-1252.
//
// It is keyed by the _NN_* name rather than by the numeric id: the ids are dense
// and unlabelled, so a mis-typed number silently yields a plausible but wrong
// sentence, while a mis-typed name yields nothing and is caught by the loader
// test. The file ships 547 keys with no duplicates.
type Language struct {
	byKey map[string]string
	byID  map[int]string
}

// LoadLanguage reads the client string table. It is optional content: the caller
// warns and keeps the compiled fallbacks when the file is not mounted.
func LoadLanguage(path string) (*Language, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("content: open Language: %w", err)
	}
	defer f.Close()
	return ParseLanguage(f)
}

// ParseLanguage reads the table from r. Exported so callers can build one from
// an embedded or test fixture instead of a file.
func ParseLanguage(r io.Reader) (*Language, error) {
	l := &Language{byKey: make(map[string]string), byID: make(map[int]string)}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		// Decode before any string handling: the bytes are Windows-1252, and
		// treating them as UTF-8 would corrupt every accented line.
		line := protocol.FromClientText(sc.Bytes())
		head, text, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		// "<id> <key>", with at least one line carrying a trailing space before
		// the tab (_NN_TP_GELO), so the key is trimmed rather than sliced.
		fields := strings.Fields(head)
		if len(fields) != 2 {
			continue
		}
		id, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		text = strings.TrimRight(text, "\r\n ")
		if text == "" {
			continue
		}
		l.byKey[fields[1]] = text
		l.byID[id] = text
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("content: read Language: %w", err)
	}
	if len(l.byKey) == 0 {
		return nil, fmt.Errorf("content: Language has no usable lines")
	}
	return l, nil
}

// Text returns the line for a _NN_* key. A nil Language reports nothing found,
// so callers can hold one unconditionally and fall back on the miss.
func (l *Language) Text(key string) (string, bool) {
	if l == nil {
		return "", false
	}
	s, ok := l.byKey[key]
	return s, ok
}

// TextByID returns the line for a numeric Language.txt id.
func (l *Language) TextByID(id int) (string, bool) {
	if l == nil {
		return "", false
	}
	s, ok := l.byID[id]
	return s, ok
}

// Len returns the loaded line count (for the boot log and tests).
func (l *Language) Len() int {
	if l == nil {
		return 0
	}
	return len(l.byKey)
}
