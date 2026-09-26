package npctemplate

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
)

// Corpo is the body a template is drawn with: Equip[0].sIndex. The client picks
// a monster's mesh by it, so it is what decides whether the players' clients can
// draw that monster at all.
//
// WHY IT MATTERS HERE: a block recipe (0165_receita_de_bloco) can put any of the
// ~2.000 files in npc/ on the map while the server runs, and most of them have
// never been born on this server. A body whose mesh the client cannot load closes
// the client of everyone who comes near — the item-side version of that already
// happened (the MSAPROT meshes). A body that some monster of NPCGener.txt already
// wears has been drawn by every player's client, so it is the evidence we have.
func Corpo(raw []byte) (int16, error) {
	m, _, err := savefmt.DecodeMobAny(raw)
	if err != nil {
		return 0, fmt.Errorf("npctemplate: corpo: %w", err)
	}
	return m.Equip[0].Index, nil
}

// Corpos maps each body the server already draws to one template that wears it —
// the name a refusal gives as the example.
type Corpos map[int16]string

// Add registers the body of raw under name. A template that does not decode adds
// nothing: it cannot be born either, so it proves no body.
func (c Corpos) Add(name string, raw []byte) {
	corpo, err := Corpo(raw)
	if err != nil {
		return
	}
	if _, ok := c[corpo]; !ok {
		c[corpo] = name
	}
}

// Conhece reports whether raw wears a body the server already draws. It answers
// the body either way, so a refusal can name it.
func (c Corpos) Conhece(raw []byte) (corpo int16, ok bool, err error) {
	corpo, err = Corpo(raw)
	if err != nil {
		return 0, false, err
	}
	_, ok = c[corpo]
	return corpo, ok, nil
}

// CorposDoArquivo collects the bodies of the templates NPCGener.txt names — the
// leaders and followers of its blocks — reading them from contentDir. A name that
// does not load is skipped, the way the boot skips it.
func CorposDoArquivo(contentDir string, nomes []string) Corpos {
	c := make(Corpos, 256)
	vistos := make(map[string]bool, len(nomes))
	for _, n := range nomes {
		if n == "" || vistos[n] {
			continue
		}
		vistos[n] = true
		raw, res, err := Load(contentDir, n)
		if err != nil {
			continue
		}
		c.Add(res.Name, raw)
	}
	return c
}
