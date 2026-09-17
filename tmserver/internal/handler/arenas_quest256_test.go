package handler

import (
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// arenaQuest256Vida é a vida de cada monstro das arenas do Kaizen e das Hidras.
// Em 14/09/2026 a equipe cortou o Kaizen pela metade (eram 8.100 e 17.100: a
// arena castigava quem chegava com os itens da quest) e a Hidra Dourada também
// (era 23.000); a Hidra Imortal segue a do legado. Mudar um número aqui é mudar
// o balanceamento, e de propósito.
var arenaQuest256Vida = map[string]int32{
	"Cav._Kaizen":   4050,
	"Cav._Servo":    8550,
	"Hidra_Dourada": 11500,
	"Hidra_Imortal": 7200,
}

// Os líderes e quem os segue: o líder paga pelo menos o que o seguidor paga.
var arenaQuest256Lideres = map[string]string{
	"Cav._Kaizen":   "Cav._Servo",
	"Hidra_Dourada": "Hidra_Imortal",
}

func TestArenasQuest256VidaDosTemplates(t *testing.T) {
	root := releaseDir(t)
	for file, hp := range arenaQuest256Vida {
		b, _, err := npctemplate.Load(root, file)
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		m, err := savefmt.DecodeMob(b)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for nome, s := range map[string]savefmt.Score{"BaseScore": m.BaseScore, "CurrentScore": m.CurrentScore} {
			if s.MaxHp != hp || s.Hp != hp {
				t.Errorf("%s: %s hp %d/%d, want %d", file, nome, s.Hp, s.MaxHp, hp)
			}
		}
	}
}

// Cada monstro das duas arenas solta os dois Restos e os quatro Âmagos pedidos,
// toda linha é uma que a Mesa aceita, e o líder nunca paga menos que o seguidor.
func TestArenasQuest256MigracaoDeDrops(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := migrations.FS.ReadFile("0065_arenas_kaizen_hidra.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`).FindAllStringSubmatch(string(b), -1)
	if len(rows) == 0 {
		t.Fatal("nenhuma linha na migração")
	}
	chance := map[string]map[int16]int32{}
	for _, r := range rows {
		item, _ := strconv.Atoi(r[2])
		c, _ := strconv.Atoi(r[3])
		rule := droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(c)}
		if !rule.Valid() || c == 0 {
			t.Errorf("%v: a Mesa de Drops recusaria esta linha, ou ela não solta nada", r[0])
		}
		if _, ok := items.Get(item); !ok {
			t.Errorf("item %d não existe no ItemList", item)
		}
		if _, ok := arenaQuest256Vida[r[1]]; !ok {
			t.Errorf("%s não é monstro das arenas", r[1])
		}
		if chance[r[1]] == nil {
			chance[r[1]] = map[int16]int32{}
		}
		chance[r[1]][int16(item)] = int32(c)
	}
	pedidos := []int16{419, 420, 2392, 2393, 2394, 2395}
	for file := range arenaQuest256Vida {
		for _, item := range pedidos {
			if chance[file][item] == 0 {
				t.Errorf("%s não solta o item %d", file, item)
			}
		}
	}
	for lider, seguidor := range arenaQuest256Lideres {
		for _, item := range pedidos {
			if chance[lider][item] < chance[seguidor][item] {
				t.Errorf("item %d: %s paga %d, menos que %s (%d)", item, lider, chance[lider][item], seguidor, chance[seguidor][item])
			}
		}
	}
}

// O pacote vale só para os Restos do líder de cada arena; o seguidor, o Âmago e
// a Hidra do mundo saem com uma unidade.
func TestArenasQuest256PacoteDoLider(t *testing.T) {
	for _, tc := range []struct {
		mob  string
		item int16
		want int
	}{
		{"Cav._Kaizen", 419, 3},
		{"Cav._Kaizen", 420, 2},
		{"Hidra_Dourada", 419, 3},
		{"Hidra_Dourada", 420, 2},
		{"Mestre_Elfo", 419, 3},
		{"Mestre_Elfo", 420, 2},
		{"Servo_Elfo", 419, 1},
		{"Cav._Servo", 419, 1},
		{"Hidra_Imortal", 420, 1},
		{"Cav._Kaizen", 2392, 1},
		{"Hidra", 419, 1},
	} {
		it := world.Item{Index: tc.item}
		setItemAmount(&it, 1)
		arenaQuest256Finish(&world.Entity{TemplateName: tc.mob}, &it)
		if got := itemAmount(it); got != tc.want {
			t.Errorf("%s soltou %d com %d unidades, want %d", tc.mob, tc.item, got, tc.want)
		}
	}
}

// Pelo abate de verdade: a regra da Mesa a 100% entrega o pacote na bolsa de
// quem mata o Cav. Kaizen.
func TestArenasQuest256PacoteChegaNaBolsa(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	d.setDropRules(droprule.Config{Version: 1, Rules: []droprule.Rule{
		{Mob: "Cav._Kaizen", Item: 419, Chance: droprule.MaxChance},
	}})
	d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(241, 0, 0), "Cav._Kaizen"))
	it, ok := carryHas(killer, 419)
	if !ok {
		t.Fatal("o Resto de Oriharucon não chegou na bolsa")
	}
	if got := itemAmount(it); got != 3 {
		t.Errorf("Resto de Oriharucon com %d unidades, want 3", got)
	}
}

// linhasDaMigracao lê as linhas (mob, item, chance) de uma migração da Mesa.
func linhasDaMigracao(t *testing.T, nome string) map[string]map[int16]int32 {
	t.Helper()
	b, err := migrations.FS.ReadFile(nome)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[int16]int32{}
	for _, r := range regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`).FindAllStringSubmatch(string(b), -1) {
		item, _ := strconv.Atoi(r[2])
		c, _ := strconv.Atoi(r[3])
		rule := droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(c)}
		if !rule.Valid() || c == 0 {
			t.Errorf("%s %v: a Mesa de Drops recusaria esta linha, ou ela não solta nada", nome, r[0])
		}
		if out[r[1]] == nil {
			out[r[1]] = map[int16]int32{}
		}
		out[r[1]][int16(item)] = int32(c)
	}
	if len(out) == 0 {
		t.Fatalf("%s: nenhuma linha", nome)
	}
	return out
}

// A arena dos Elfos paga como a das Hidras, papel por papel (pedido de
// 17/09/2026): o Mestre Elfo como a Hidra Dourada, o Servo Elfo como a Imortal.
func TestArenaElfosPagaComoAsHidras(t *testing.T) {
	hidras := linhasDaMigracao(t, "0065_arenas_kaizen_hidra.up.sql")
	elfos := linhasDaMigracao(t, "0075_arena_elfos_e_chave_orc.up.sql")
	for elfo, hidra := range map[string]string{"Mestre_Elfo": "Hidra_Dourada", "Servo_Elfo": "Hidra_Imortal"} {
		for _, item := range []int16{419, 420, 2392, 2393, 2394, 2395} {
			if got, want := elfos[elfo][item], hidras[hidra][item]; got != want || got == 0 {
				t.Errorf("%s item %d a %d, a %s paga %d", elfo, item, got, hidra, want)
			}
		}
	}
}

// A Chave do Rei Orc cai dos quatro monstros das duas arenas, e o líder nunca
// dá menos que o seguidor.
func TestChaveOrcNasArenasDasHidrasEDosElfos(t *testing.T) {
	linhas := linhasDaMigracao(t, "0075_arena_elfos_e_chave_orc.up.sql")
	for lider, seguidor := range map[string]string{"Hidra_Dourada": "Hidra_Imortal", "Mestre_Elfo": "Servo_Elfo"} {
		l, s := linhas[lider][itemChaveCasteloOrc], linhas[seguidor][itemChaveCasteloOrc]
		if l == 0 || s == 0 {
			t.Errorf("chave: %s %d, %s %d — as duas arenas precisam dar a chave", lider, l, seguidor, s)
		}
		if l < s {
			t.Errorf("chave: %s (%d) paga menos que %s (%d)", lider, l, seguidor, s)
		}
	}
}

// Pelo abate de verdade: com o '*' a 0% da 0053 ainda na mesa, a linha nomeada
// do Servo Elfo entrega a chave na bolsa de quem mata.
func TestChaveOrcCaiDoServoElfo(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	d.setDropRules(droprule.Config{Version: 1, Rules: []droprule.Rule{
		{Mob: droprule.AllMobs, Item: itemChaveCasteloOrc, Chance: 0},
		{Mob: "Servo_Elfo", Item: itemChaveCasteloOrc, Chance: droprule.MaxChance},
	}})
	d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(330, 0, 0), "Servo_Elfo"))
	if _, ok := carryHas(killer, itemChaveCasteloOrc); !ok {
		t.Fatal("a Chave do Rei Orc não chegou na bolsa")
	}
}
