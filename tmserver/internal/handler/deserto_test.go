package handler

import (
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	itemPedraDoLugefer    = 1758
	itemAmagoEquipadoN    = 2399
	itemOvoEquipadoN      = 2309
	desertoMigracao       = "0108_deserto_saque.up.sql"
	desertoMatadoresMinim = 100 // a equipe: 1 Pedra do Lugefer a cada 100 mortes ou mais
)

// linhasDoDeserto lê as linhas (mob, item, chance) da 0108. Diferente de
// linhasDaMigracao, aceita 0%: o Cav. Lugefer tem o Andaluz B zerado de propósito.
func linhasDoDeserto(t *testing.T) map[string]map[int16]int32 {
	t.Helper()
	b, err := migrations.FS.ReadFile(desertoMigracao)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[int16]int32{}
	for _, r := range regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`).FindAllStringSubmatch(string(b), -1) {
		item, _ := strconv.Atoi(r[2])
		c, _ := strconv.Atoi(r[3])
		if !(droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(c)}).Valid() {
			t.Errorf("%v: a Mesa de Drops recusaria esta linha", r[0])
		}
		if out[r[1]] == nil {
			out[r[1]] = map[int16]int32{}
		}
		if _, dup := out[r[1]][int16(item)]; dup {
			t.Errorf("%s item %d aparece duas vezes na 0108", r[1], item)
		}
		out[r[1]][int16(item)] = int32(c)
	}
	if len(out) == 0 {
		t.Fatal("0108: nenhuma linha")
	}
	return out
}

// taxaPaga é o que o jogo entrega para uma chance da Mesa, contando os 32.768
// valores do rand() do MSVC em vez de simular.
func taxaPaga(chance int32) float64 {
	acertos := 0
	for r := 0; r < 32768; r++ {
		if droprule.Roll(chance, func(n int) int { return r % n }) {
			acertos++
		}
	}
	return float64(acertos) / 32768
}

// Toda linha da 0108 aponta para um item do ItemList e para um template que
// existe, e o arquivo não repete par.
func TestDesertoMigracaoApontaParaOQueExiste(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	for mob, linhas := range linhasDoDeserto(t) {
		if _, _, err := npctemplate.Load(root, mob); err != nil {
			t.Errorf("%s: %v", mob, err)
		}
		for item := range linhas {
			if _, ok := items.Get(int(item)); !ok {
				t.Errorf("%s: item %d não existe no ItemList", mob, item)
			}
		}
	}
}

// A Pedra do Lugefer cai a 1 a cada 100 mortes do Cav. Lugefer ou mais, nunca
// menos — medido no que o jogo PAGA, com o viés do sorteio da Mesa, e não no
// número escrito (100 escrito pagaria 1,22%, 1 a cada 82).
func TestDesertoPedraDoLugeferUmEmCem(t *testing.T) {
	c, ok := linhasDoDeserto(t)["Cav._Lugefer"][itemPedraDoLugefer]
	if !ok {
		t.Fatal("a 0108 não tem regra para a Pedra do Lugefer: o template a dá em 1 de cada 4 mortes")
	}
	paga := taxaPaga(c)
	if paga <= 0 || 1/paga < desertoMatadoresMinim {
		t.Errorf("Pedra do Lugefer a %d paga %.4f%%, 1 a cada %.1f mortes; want 1 a cada %d ou mais",
			c, paga*100, 1/paga, desertoMatadoresMinim)
	}
	// E não mais rara que o necessário: 82 já passaria do limite.
	if taxaPaga(c+1) <= 1.0/desertoMatadoresMinim {
		t.Errorf("Pedra do Lugefer a %d está abaixo do limite; %d ainda paga 1 a cada 100 ou mais", c, c+1)
	}
}

// O Cavalo Equipado N (âmago e ovo) cai só dos Lugefer — pedido da equipe — e
// dos Agmo, que o dão no pacote de evento.
func TestDesertoEquipadoSoDosLugefer(t *testing.T) {
	permitidos := map[string]bool{"Lugefer": true, "Cav._Lugefer": true, "Tauron_Agmo": true, "Verme_Agmo": true}
	linhas := linhasDoDeserto(t)
	for mob, l := range linhas {
		for _, item := range []int16{itemAmagoEquipadoN, itemOvoEquipadoN} {
			if l[item] > 0 && !permitidos[mob] {
				t.Errorf("%s solta o item %d; o Cavalo Equipado N é só dos Lugefer", mob, item)
			}
		}
	}
	for _, mob := range []string{"Lugefer", "Cav._Lugefer"} {
		for _, item := range []int16{itemAmagoEquipadoN, itemOvoEquipadoN} {
			if linhas[mob][item] == 0 {
				t.Errorf("%s não solta o item %d", mob, item)
			}
		}
	}
}

// Os dois Agmo soltam os quatro âmagos N sempre, e cada um tem pacote no código:
// uma regra a 100% sem pacote entregaria um âmago só.
func TestDesertoAgmoSempreSoltaOPacote(t *testing.T) {
	linhas := linhasDoDeserto(t)
	for _, mob := range []string{"Tauron_Agmo", "Verme_Agmo"} {
		pacote := desertoPacotes[droprule.Canonical(mob)]
		if len(pacote) != 4 {
			t.Errorf("%s: pacote com %d itens, want 4", mob, len(pacote))
		}
		for item := range pacote {
			if linhas[mob][item] != droprule.MaxChance {
				t.Errorf("%s item %d a %d na 0108, want %d", mob, item, linhas[mob][item], droprule.MaxChance)
			}
		}
		for item := range linhas[mob] {
			if pacote[item] == 0 {
				t.Errorf("%s item %d está na 0108 sem pacote no código", mob, item)
			}
		}
	}
}

// O pacote: 40 Sem Sela, 30 Fantasma, 20 Leve e 10 Equipado, só nos Agmo.
func TestDesertoPacoteDosAgmo(t *testing.T) {
	for _, tc := range []struct {
		mob  string
		item int16
		want int
	}{
		{"Tauron_Agmo", 2396, 40},
		{"Tauron_Agmo", 2397, 30},
		{"Tauron_Agmo", 2398, 20},
		{"Tauron_Agmo", 2399, 10},
		{"Verme_Agmo", 2396, 40},
		{"Verme_Agmo", 2399, 10},
		{"Tauron", 2398, 1},
		{"Lugefer", 2399, 1},
		{"Cav._Lugefer", 2399, 1},
		{"Tauron_Agmo", 1774, 1}, // a Pedra do Sábio do template não entra no pacote
	} {
		it := world.Item{Index: tc.item}
		setItemAmount(&it, 1)
		desertoFinish(&world.Entity{TemplateName: tc.mob}, &it)
		if got := itemAmount(it); got != tc.want {
			t.Errorf("%s soltou %d com %d unidades, want %d", tc.mob, tc.item, got, tc.want)
		}
	}
}

// Pelo abate de verdade: com as regras a 100%, os quatro pacotes chegam na
// bolsa de quem mata o Tauron Agmo, cada um com a sua quantidade.
func TestDesertoPacoteChegaNaBolsa(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	var rules []droprule.Rule
	for item := range desertoPacoteAgmo {
		rules = append(rules, droprule.Rule{Mob: "Tauron_Agmo", Item: item, Chance: droprule.MaxChance})
	}
	d.setDropRules(droprule.Config{Version: 1, Rules: rules})
	d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(200, 0, 0), "Tauron_Agmo"))
	for item, want := range desertoPacoteAgmo {
		it, ok := carryHas(killer, item)
		if !ok {
			t.Errorf("âmago %d não chegou na bolsa", item)
			continue
		}
		if got := itemAmount(it); got != want {
			t.Errorf("âmago %d com %d unidades, want %d", item, got, want)
		}
	}
}
