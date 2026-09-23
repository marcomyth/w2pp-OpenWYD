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

// O Fragmento de Alma cai a 1 a cada 150-180 mortes (pedido da equipe de
// 23/09), medido no que o jogo paga, em todo monstro do Deserto que o solte.
func TestDesertoFragmentoUmEm150a180(t *testing.T) {
	const itemFragmentoDeAlma = 3224
	achou := 0
	for mob, l := range linhasDoDeserto(t) {
		c, ok := l[itemFragmentoDeAlma]
		if !ok || c == 0 {
			continue
		}
		achou++
		if mortes := 1 / taxaPaga(c); mortes < 150 || mortes > 180 {
			t.Errorf("%s: Fragmento de Alma a %d sai 1 a cada %.1f mortes, want 150 a 180", mob, c, mortes)
		}
	}
	if achou == 0 {
		t.Error("nenhum monstro da 0108 solta o Fragmento de Alma")
	}
}

// O Cavalo Equipado N: o ovo cai só dos Lugefer, e o âmago também do Ladrão e
// do Assassino, no lugar do Fenrir (pedidos da equipe de 23/09).
func TestDesertoCavaloEquipado(t *testing.T) {
	quemDa := map[int16]map[string]bool{
		itemOvoEquipadoN:   {"Lugefer": true, "Cav._Lugefer": true},
		itemAmagoEquipadoN: {"Lugefer": true, "Cav._Lugefer": true, "Ladrao_Tauron": true, "Taron_Assassino": true},
	}
	linhas := linhasDoDeserto(t)
	for item, pode := range quemDa {
		for mob, l := range linhas {
			if l[item] > 0 && !pode[mob] {
				t.Errorf("%s solta o item %d, e ele não é dele", mob, item)
			}
		}
		for mob := range pode {
			if linhas[mob][item] == 0 {
				t.Errorf("%s não solta o item %d", mob, item)
			}
		}
	}
}

// Sem Fenrir no Deserto ("Fenrir ainda não precisamos"): nenhuma linha da 0108
// dá Fenrir, e todo monstro da 0108 cujo TEMPLATE solta Fenrir tem a linha a 0%
// que o tira — senão o template continua soltando por baixo da Mesa.
func TestDesertoSemFenrir(t *testing.T) {
	fenrir := map[int16]bool{2406: true, 2316: true, 2408: true, 2318: true} // âmagos e ovos, normal e das Sombras
	root := releaseDir(t)
	for mob, l := range linhasDoDeserto(t) {
		for item, c := range l {
			if fenrir[item] && c > 0 {
				t.Errorf("%s solta Fenrir (%d) a %d", mob, item, c)
			}
		}
		b, _, err := npctemplate.Load(root, mob)
		if err != nil {
			t.Fatalf("%s: %v", mob, err)
		}
		m, _, err := savefmt.DecodeMobAny(b)
		if err != nil {
			t.Fatalf("%s: %v", mob, err)
		}
		for _, it := range m.Carry {
			if !fenrir[it.Index] {
				continue
			}
			if c, ok := l[it.Index]; !ok || c != 0 {
				t.Errorf("o template de %s solta o Fenrir %d e a 0108 não o zera", mob, it.Index)
			}
		}
	}
}

// Os Agmo não têm linha na Mesa: o âmago deles sai do código, exatamente um.
func TestDesertoAgmoForaDaMesa(t *testing.T) {
	linhas := linhasDoDeserto(t)
	for _, mob := range []string{"Tauron_Agmo", "Verme_Agmo"} {
		if len(linhas[mob]) > 0 {
			t.Errorf("%s tem %d linhas na 0108; a Mesa rola cada item sozinho e daria 0 ou vários âmagos", mob, len(linhas[mob]))
		}
	}
}

// Os pesos do sorteio do Agmo, contados sobre a base inteira: 40 Sem Sela, 30
// Fantasma, 20 Cavalo Leve e 10 Cavalo Equipado.
func TestDesertoAgmoPesos(t *testing.T) {
	got := map[int16]int{}
	for r := range agmoPesoTotal {
		got[agmoSorteia(r)]++
	}
	want := map[int16]int{2396: 40, 2397: 30, 2398: 20, 2399: 10}
	for item, n := range want {
		if got[item] != n {
			t.Errorf("âmago %d sai em %d de %d, want %d", item, got[item], agmoPesoTotal, n)
		}
	}
	if len(got) != len(want) {
		t.Errorf("o sorteio escolhe %d itens, want %d", len(got), len(want))
	}
}

// Pelo abate de verdade: cada morte de um Agmo põe exatamente um âmago N, com
// uma unidade, na bolsa de quem mata; um Tauron comum não ganha nada disso.
func TestDesertoAgmoSoltaUmAmago(t *testing.T) {
	amagoNaBolsa := func(e *world.Entity) (n, unidades int) {
		for _, a := range agmoAmagos {
			for _, it := range e.Carry {
				if it.Index == a.item {
					n++
					unidades += itemAmount(it)
				}
			}
		}
		return n, unidades
	}
	for _, mob := range []string{"Tauron_Agmo", "Verme_Agmo"} {
		for range 5 {
			d, w, killer := mobKilledWorld(t)
			d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(200, 0, 0), mob))
			if n, u := amagoNaBolsa(killer); n != 1 || u != 1 {
				t.Fatalf("%s: %d âmagos N na bolsa, %d unidades; want 1 e 1", mob, n, u)
			}
		}
	}
	d, w, killer := mobKilledWorld(t)
	d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(351, 0, 0), "Tauron"))
	if n, _ := amagoNaBolsa(killer); n != 0 {
		t.Errorf("um Tauron comum deixou %d âmagos do sorteio do Agmo", n)
	}
}
