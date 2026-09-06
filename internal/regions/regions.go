// Package regions reads Release/TMsrv/run/Regions.txt, the game's own list of
// named rectangles on the single global grid, and answers "what is this place
// called" for a position.
//
// It exists alongside internal/mapzones rather than inside it because the two
// answer different questions from different sources. mapzones classifies by
// distance to a settlement center, which is the right model for players and
// merchant NPCs: towns are points people gather around. Regions.txt is the
// content tree's own rectangle table, and it is the only place in the repo that
// names the instanced dungeons — Água, Cubo, Kefra, Lan, Submundo, the Deserto
// belt and the Coliseu are all absent from mapzones, which knows five cities
// and the three Pesadelo blocks.
//
// For "where does this monster come from" the rectangles are the better answer:
// a spawn point either is or is not inside Agua_M, and no radius has to be
// calibrated for it.
package regions

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Region is one named rectangle. X1,Y1 is always the lower corner: the file has
// at least one row written with the corners swapped (Reino_Blue, "1814, 1815,
// 1785, 1913"), and a containment test against those bounds as written matches
// nothing at all.
type Region struct {
	X1, Y1, X2, Y2 int32
	// Name is the raw file label (Agua_M, Dugeon_1_Andar_Hidra). Label() is the
	// version fit to show somebody.
	Name string
}

// Contains reports whether a position falls inside the rectangle, edges included.
func (r Region) Contains(x, y int32) bool {
	return x >= r.X1 && x <= r.X2 && y >= r.Y1 && y <= r.Y2
}

// Table is the file in order. Order matters: rows overlap (Armia and Erion each
// appear twice, with the second row a superset of the first), and the first
// match is the tighter one.
type Table []Region

// At names the region holding a position, or "" when none does — most of the
// 4096x4096 grid is open field that Regions.txt says nothing about.
func (t Table) At(x, y int32) string {
	for _, r := range t {
		if r.Contains(x, y) {
			return r.Name
		}
	}
	return ""
}

// Load parses Regions.txt. Lines are "x1, y1, x2, y2 = Name"; anything that
// does not fit that shape is skipped rather than failing the load, because this
// file is content, not configuration: one malformed row must not cost the
// caller the other fifty.
func Load(path string) (Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("regions: open Regions: %w", err)
	}
	defer f.Close()

	var out Table
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		r, ok := parseLine(sc.Text())
		if ok {
			out = append(out, r)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("regions: read Regions: %w", err)
	}
	return out, nil
}

// parseLine reads one "x1, y1, x2, y2 = Name" row. The file's last line has no
// trailing newline and its numbers are spaced inconsistently ("1272, 1427,1403,
// 1532"), so every field is trimmed independently.
func parseLine(line string) (Region, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "//") {
		return Region{}, false
	}
	nums, name, ok := strings.Cut(line, "=")
	if !ok {
		return Region{}, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Region{}, false
	}
	parts := strings.Split(nums, ",")
	if len(parts) != 4 {
		return Region{}, false
	}
	var v [4]int32
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return Region{}, false
		}
		v[i] = int32(n)
	}
	r := Region{X1: v[0], Y1: v[1], X2: v[2], Y2: v[3], Name: name}
	if r.X1 > r.X2 {
		r.X1, r.X2 = r.X2, r.X1
	}
	if r.Y1 > r.Y2 {
		r.Y1, r.Y2 = r.Y2, r.Y1
	}
	return r, true
}

// rotulos renames the file labels a moderator would otherwise have to decode.
// Only entries whose raw form is wrong or ambiguous are listed — a label that
// merely uses underscores is handled by Label's fallback, and one that already
// reads correctly (Coliseu, Cassino, Karden) is absent on purpose.
//
// "Dugeon" is the content tree's own typo, kept in the key so the lookup still
// matches the file.
var rotulos = map[string]string{
	"Pesadelo_N":             "Pesadelo Normal",
	"Pesadelo_M":             "Pesadelo Místico",
	"Pesadelo_A":             "Pesadelo Arcano",
	"Agua_N":                 "Água Normal",
	"Agua_M":                 "Água Místico",
	"Agua_A":                 "Água Arcano",
	"Cubo_N":                 "Cubo Normal",
	"Cubo_M":                 "Cubo Místico",
	"Cubo_A":                 "Cubo Arcano",
	"Lan_N":                  "Lan Normal",
	"Lan_M":                  "Lan Místico",
	"Lan_A":                  "Lan Arcano",
	"Guerra_de_Cidades":      "Guerra de Cidades",
	"Monster_City":           "Monster City",
	"Portao_Infernal":        "Portão Infernal",
	"Vale_Escondido":         "Vale Escondido",
	"Dugeon_1_Andar_Hidra":   "Dungeon 1º Andar (Hidra)",
	"Dungeon_1_Andar_Kaizen": "Dungeon 1º Andar (Kaizen)",
	"Dungeon_2_Andar":        "Dungeon 2º Andar",
	"Dungeon_3_Andar":        "Dungeon 3º Andar",
	"Campo_de_Treino":        "Campo de Treino",
	"Entrada_Dungeon":        "Entrada da Dungeon",
	"Zona_Neutra":            "Zona Neutra",
	"Nova_Guerra_Noatun":     "Nova Guerra de Noatun",
	"Kefra_City":             "Kefra City",
	"Castelo_Zakun":          "Castelo Zakun",
}

// Label is the region name fit to show somebody: a curated translation where
// one exists, otherwise the raw label with its underscores opened up. The
// fallback is deliberate — a region added to Regions.txt tomorrow still reads
// as words instead of disappearing behind an empty string.
func Label(name string) string {
	if r, ok := rotulos[name]; ok {
		return r
	}
	return strings.ReplaceAll(name, "_", " ")
}
