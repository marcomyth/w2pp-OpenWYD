package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/regions"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const geloMigracao = "0110_gelo_saque.up.sql"

// efeitoDoItem lê o valor de um EF_<nome> na linha do ItemList.
func efeitoDoItem(e content.ItemEntry, nome string) (int, bool) {
	for i := 0; i+1 < len(e.Fields); i++ {
		if e.Fields[i] == nome {
			v, err := strconv.Atoi(e.Fields[i+1])
			return v, err == nil
		}
	}
	return 0, false
}

func catalogoDoGelo(t *testing.T) *content.ItemList {
	t.Helper()
	items, err := content.LoadItemList(filepath.Join(releaseDir(t), "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// A caixa do código é a Karden do Regions.txt.
func TestGeloCaixaBateComRegions(t *testing.T) {
	tab, err := regions.Load(filepath.Join(releaseDir(t), "TMsrv", "run", "Regions.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range tab {
		if r.Name != "Karden" {
			continue
		}
		if r.X1 != geloMinX || r.Y1 != geloMinY || r.X2 != geloMaxX || r.Y2 != geloMaxY {
			t.Errorf("Karden no Regions.txt é %d,%d-%d,%d; o código diz %d,%d-%d,%d",
				r.X1, r.Y1, r.X2, r.Y2, geloMinX, geloMinY, geloMaxX, geloMaxY)
		}
		return
	}
	t.Fatal("o Regions.txt não tem a caixa Karden")
}

// Toda linha da 0110 aponta para um item do ItemList e um template que existe. E
// uma regra da Mesa vale pelo nome do template: quem também nasce fora do Gelo só
// pode ter linha a 0% (o que sai do Gelo sai de lá também), nunca um saque — o
// Verid do Coliseu (bloco 104) é o caso.
func TestGeloMigracaoFicaNoGelo(t *testing.T) {
	root := releaseDir(t)
	items := catalogoDoGelo(t)
	linhas := linhasComZero(t, geloMigracao)
	for mob, l := range linhas {
		if _, _, err := npctemplate.Load(root, mob); err != nil {
			t.Errorf("%s: %v", mob, err)
		}
		for item := range l {
			if _, ok := items.Get(int(item)); !ok {
				t.Errorf("%s: item %d não existe no ItemList", mob, item)
			}
		}
	}
	gens, err := content.LoadNPCGenerators(filepath.Join(root, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range gens {
		for _, nome := range []string{g.Leader, g.Follower} {
			if _, ok := linhas[nome]; !ok {
				continue
			}
			x, y := int(g.SegX[0]), int(g.SegY[0])
			if x >= geloMinX && x <= geloMaxX && y >= geloMinY && y <= geloMaxY {
				continue
			}
			for item, c := range linhas[nome] {
				if c > 0 {
					t.Errorf("%s também nasce no bloco %d em (%d,%d), fora do Gelo, e a 0110 dá a ele o item %d a %d", nome, i, x, y, item, c)
				}
			}
		}
	}
}

// O que saiu do Gelo (pedidos de 23/09): Fenrir e as montarias altas, Andaluz, as
// montarias N (o Gelo é das brancas), a Pedra da Luz e todo item Arch
// (EF_MOBTYPE 1). Nenhuma linha da 0110 os dá, todo template da 0110 que os
// solta tem a linha a 0%, e o saque dos chefes não os tem.
func TestGeloSemOQueSaiu(t *testing.T) {
	root := releaseDir(t)
	items := catalogoDoGelo(t)
	motivo := func(item int16) string {
		switch {
		case item >= 2406 && item <= 2419, item >= 2316 && item <= 2329:
			return "Fenrir ou montaria alta"
		case item == 2400 || item == 2405 || item == 2310 || item == 2315:
			return "Andaluz"
		case item >= 2396 && item <= 2399, item >= 2306 && item <= 2309:
			return "montaria N"
		case item == 3140:
			return "Pedra da Luz"
		}
		if e, ok := items.Get(int(item)); ok {
			if mt, ok := efeitoDoItem(e, "EF_MOBTYPE"); ok && mt == 1 {
				return "Arch"
			}
		}
		return ""
	}
	for mob, l := range linhasComZero(t, geloMigracao) {
		for item, c := range l {
			if m := motivo(item); m != "" && c > 0 {
				t.Errorf("%s solta %d (%s) a %d", mob, item, m, c)
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
			if mv := motivo(it.Index); mv != "" {
				if c, ok := l[it.Index]; !ok || c != 0 {
					t.Errorf("o template de %s solta %d (%s) e a 0110 não o zera", mob, it.Index, mv)
				}
			}
		}
	}
	for _, p := range geloChefePremios {
		if m := motivo(p.item); m != "" {
			t.Errorf("o saque dos chefes dá %s (%s)", p.nome, m)
		}
	}
}

// As montarias por monstro: o macaco fraco (Símio Ancião) dá âmago e ovo B até o
// Cavalo Leve, os macacos fortes (Amon) o Cavalo Equipado B, e ninguém mais da
// tropa solta montaria pela 0110.
func TestGeloMontariasDosMacacos(t *testing.T) {
	linhas := linhasComZero(t, geloMigracao)
	quer := map[string][]int16{
		"Simio_Anciao":   {2401, 2402, 2403, 2311, 2312, 2313},
		"Soldado_Amon":   {2404, 2314},
		"Capitao_Amon":   {2404, 2314},
		"Guerreiro_Amon": {2404, 2314},
	}
	for mob, itens := range quer {
		for _, item := range itens {
			if linhas[mob][item] == 0 {
				t.Errorf("%s não solta o item %d", mob, item)
			}
		}
	}
	for mob, l := range linhas {
		for item, c := range l {
			montaria := (item >= 2300 && item <= 2330) || (item >= 2390 && item <= 2419)
			if !montaria || c == 0 {
				continue
			}
			dele := false
			for _, i := range quer[mob] {
				dele = dele || i == item
			}
			if !dele {
				t.Errorf("%s solta a montaria %d, e ela é dos macacos", mob, item)
			}
		}
	}
}

// Os Kalintz soltam as armas E que não são Arch — as dez só de Mortal — e o
// Escudo do Guardião: todo equipamento deles na 0110 é nível 5 e EF_MOBTYPE 2.
func TestGeloKalintzArmasMortal(t *testing.T) {
	items := catalogoDoGelo(t)
	linhas := linhasComZero(t, geloMigracao)
	for _, mob := range []string{"Homem_Kalintz", "Mulher_Kalintz"} {
		armas := 0
		for item, c := range linhas[mob] {
			e, _ := items.Get(int(item))
			nivel, _ := efeitoDoItem(e, "EF_ITEMLEVEL")
			if c == 0 || nivel == 0 {
				continue
			}
			mt, _ := efeitoDoItem(e, "EF_MOBTYPE")
			if nivel != 5 || mt != 2 {
				t.Errorf("%s solta %s: nível %d, EF_MOBTYPE %d; want nível 5 só de Mortal", mob, e.Name, nivel, mt)
			}
			armas++
		}
		if armas != 11 {
			t.Errorf("%s solta %d armas e escudos E, want 11 (dez armas e o Escudo do Guardião)", mob, armas)
		}
	}
}

// O sorteio dos chefes, medido nos 32.768 valores do rand() do MSVC: a Alma
// nunca passa do 1% pedido, e os cinco outros prêmios dividem o resto.
func TestGeloChefeSorteio(t *testing.T) {
	vezes := map[string]int{}
	for v := range 32768 {
		vezes[geloChefeSorteia(v%geloChefeBase).nome]++
	}
	soma := 0
	for _, p := range geloChefePremios {
		soma += p.peso
		taxa := float64(vezes[p.nome]) / 32768
		if p.item == 0 {
			if taxa > 0.01 || taxa < 0.0098 {
				t.Errorf("a Alma sai a %.3f%%, want até 1%% (e perto disso)", taxa*100)
			}
			continue
		}
		// 32768 % 500 = 268: as faixas que caem nos primeiros 268 valores saem
		// 0,3 ponto a mais que as outras. Só a Alma tem teto pedido.
		if taxa < 0.196 || taxa > 0.2 {
			t.Errorf("%s sai a %.3f%%, want perto de 19,8%%", p.nome, taxa*100)
		}
	}
	if soma != geloChefeBase {
		t.Errorf("os pesos somam %d, want %d", soma, geloChefeBase)
	}
	if geloChefePremios[len(geloChefePremios)-1].item != 0 {
		t.Error("a Alma precisa ser o último prêmio: o fim da base é onde o viés não a infla")
	}
}

// Pelo abate de verdade: a Sombra e o Verid nascidos no Gelo põem exatamente um
// prêmio na bolsa, com a Alma certa de cada um; a Sombra nascida fora do Gelo
// não paga nada disso.
func TestGeloChefeSoltaUmPremio(t *testing.T) {
	if geloChefeAlma(&world.Entity{TemplateName: "Sombra_Negra_"}) != itemAlmaDaFenix ||
		geloChefeAlma(&world.Entity{TemplateName: "Verid"}) != itemAlmaDoUnicornio ||
		geloChefeAlma(&world.Entity{TemplateName: "Troll_de_Gelo"}) != 0 {
		t.Fatal("a Alma de cada chefe está trocada")
	}
	premios := map[int16]int{itemAlmaDaFenix: 1, itemAlmaDoUnicornio: 1}
	for _, p := range geloChefePremios {
		if p.item != 0 {
			premios[p.item] = p.quantidade
		}
	}
	conta := func(e *world.Entity) (n int) {
		for _, it := range e.Carry {
			if want, ok := premios[it.Index]; ok {
				n++
				if got := itemAmount(it); got != want {
					t.Errorf("item %d com %d unidades, want %d", it.Index, got, want)
				}
			}
		}
		return n
	}
	for _, mob := range []string{"Sombra_Negra", "Verid_"} {
		for range 20 {
			d, w, killer := mobKilledWorld(t)
			m := spawnNamed(t, w, expMobTemplate(399, 0, 0), mob)
			m.SpawnX, m.SpawnY = 3817, 2880
			d.mobKilled(w, killer, m)
			if n := conta(killer); n != 1 {
				t.Fatalf("%s no Gelo: %d prêmios na bolsa, want 1", mob, n)
			}
		}
	}
	d, w, killer := mobKilledWorld(t)
	m := spawnNamed(t, w, expMobTemplate(399, 0, 0), "Sombra_Negra")
	m.SpawnX, m.SpawnY = 2635, 1725
	d.mobKilled(w, killer, m)
	if n := conta(killer); n != 0 {
		t.Errorf("a Sombra nascida fora do Gelo pagou %d prêmios do Gelo", n)
	}
}

// A Sombra Negra e o Verid do Gelo voltam 4 h depois da morte; os mesmos
// templates fora da caixa (a Sombra de outro mapa, o Verid do Coliseu) seguem as
// horas de chefe do painel.
func TestGeloChefeRenasceEm4Horas(t *testing.T) {
	bloco := func(nome string, x, y int16) *world.Generator {
		g := geradorSozinho(2_990_849, 10)
		g.LeaderName = nome
		g.SegX[0], g.SegY[0] = x, y
		return g
	}
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{
		30: bloco("Verid", 3650, 2770),
		31: bloco("Sombra_Negra_", 3817, 2880),
		32: bloco("Sombra_Negra", 2635, 1725),
		33: bloco("Verid", 2636, 1723),
	})
	d := dispatcherQuieto()
	quatro := uint32(4 * msPorHora)
	for _, idx := range []int{30, 31} {
		if got := d.esperaDoRenascimento(w, idx); got != quatro {
			t.Errorf("bloco %d volta em %d ms, want %d", idx, got, quatro)
		}
	}
	for _, idx := range []int{32, 33} {
		if d.esperaDoRenascimento(w, idx) == quatro {
			t.Errorf("bloco %d, fora do Gelo, também ganhou as 4 h", idx)
		}
	}
}

// Os blocos dos chefes do Gelo não têm período de minuto: é a fila individual,
// com a espera de esperaDoRenascimento, que os traz de volta. Um período positivo
// os traria pelo relógio do gerador e ignoraria as 4 h.
func TestGeloChefeBlocosSemPeriodo(t *testing.T) {
	gens, err := content.LoadNPCGenerators(filepath.Join(releaseDir(t), "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for i, g := range gens {
		x, y := int(g.SegX[0]), int(g.SegY[0])
		if geloChefeAlma(&world.Entity{TemplateName: g.Leader}) == 0 ||
			x < geloMinX || x > geloMaxX || y < geloMinY || y > geloMaxY {
			continue
		}
		n++
		if g.MinuteGenerate > 0 {
			t.Errorf("bloco %d (%s) tem MinuteGenerate %d: voltaria pelo relógio, não em 4 h", i, g.Leader, g.MinuteGenerate)
		}
	}
	if n != 5 {
		t.Errorf("%d blocos de chefe no Gelo, want 5 (Verid ×2, Verid_, Sombra_Negra, Sombra_Negra_)", n)
	}
}
