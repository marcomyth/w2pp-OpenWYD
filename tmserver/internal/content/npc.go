package content

import (
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
)

// NPCGenerator is one spawn block of NPCGener.txt. It is an alias for
// npcgener.Generator: the parser moved to the repo-root internal/ so dbServer's
// import and the webServer moderation panel can share it instead of keeping
// their own copies, and the alias keeps this package's long-standing name
// working for every caller inside tmserver.
type NPCGenerator = npcgener.Generator

// LoadNPCGenerators parses NPCGener.txt. Blocks start with '#'; lines are
// "Key:\tvalue"; '//' lines are comments.
func LoadNPCGenerators(path string) ([]NPCGenerator, error) {
	return npcgener.Load(path)
}

// LoadNPCTemplate reads one mob template (Release/TMsrv/run/npc/<name>) and
// returns it as a raw canonical 816-byte STRUCT_MOB — the legacy 756/920-byte
// layouts in that directory are widened by npctemplate.Load (data-formats.md
// §1.4.1). Legacy NPCGener names are resolved with Windows-like path
// compatibility (case-insensitive, trailing dots ignored). Templates are cached
// by the caller (many generators share a name).
func LoadNPCTemplate(dir, name string) ([]byte, error) {
	b, _, err := npctemplate.Load(dir, name)
	if err != nil {
		return nil, err
	}
	return b, nil
}
