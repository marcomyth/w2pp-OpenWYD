package handler

import (
	"io"
	"log/slog"
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
	desertoMigracao       = "0109_deserto_saque.up.sql"
	desertoMatadoresMinim = 100 // a equipe: 1 Pedra do Lugefer a cada 100 mortes ou mais
)

// linhasDoDeserto lê as linhas (mob, item, chance) da 0109. Diferente de
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
			t.Errorf("%s item %d aparece duas vezes na 0109", r[1], item)
		}
		out[r[1]][int16(item)] = int32(c)
	}
	if len(out) == 0 {
		t.Fatal("0109: nenhuma linha")
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

// Toda linha da 0109 aponta para um item do ItemList e para um template que
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
		t.Fatal("a 0109 não tem regra para a Pedra do Lugefer: o template a dá em 1 de cada 4 mortes")
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
		t.Error("nenhum monstro da 0109 solta o Fragmento de Alma")
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

// semNoDeserto cobra que nenhum item de proibidos caia no Deserto: nenhuma linha
// da 0109 o dá, todo monstro da 0109 cujo TEMPLATE o solta tem a linha a 0% que o
// tira — senão o template continua soltando por baixo da Mesa —, e os sorteios
// que moram no código (Agmo, Boss Mantícora) não o têm.
func semNoDeserto(t *testing.T, oque string, proibidos map[int16]bool) {
	t.Helper()
	root := releaseDir(t)
	for mob, l := range linhasDoDeserto(t) {
		for item, c := range l {
			if proibidos[item] && c > 0 {
				t.Errorf("%s solta %s (%d) a %d", mob, oque, item, c)
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
			if !proibidos[it.Index] {
				continue
			}
			if c, ok := l[it.Index]; !ok || c != 0 {
				t.Errorf("o template de %s solta %s (%d) e a 0109 não o zera", mob, oque, it.Index)
			}
		}
	}
	for _, a := range agmoAmagos {
		if proibidos[a.item] {
			t.Errorf("o sorteio do Agmo dá %s (%d)", oque, a.item)
		}
	}
	for _, p := range bossManticoraPremios {
		if proibidos[p.itemN] || proibidos[p.itemB] {
			t.Errorf("o sorteio do Boss Mantícora dá %s (%s)", oque, p.nome)
		}
	}
}

// Sem Fenrir no Deserto ("Fenrir ainda não precisamos", 23/09): âmagos e ovos,
// o normal e o das Sombras.
func TestDesertoSemFenrir(t *testing.T) {
	semNoDeserto(t, "Fenrir", map[int16]bool{2406: true, 2316: true, 2408: true, 2318: true})
}

// Sem Andaluz no Deserto, por enquanto (23/09): âmagos e ovos, N e B.
func TestDesertoSemAndaluz(t *testing.T) {
	semNoDeserto(t, "Andaluz", map[int16]bool{2400: true, 2405: true, 2310: true, 2315: true})
}

// Os Agmo não têm linha na Mesa: o âmago deles sai do código, exatamente um.
func TestDesertoAgmoForaDaMesa(t *testing.T) {
	linhas := linhasDoDeserto(t)
	for _, mob := range []string{"Tauron_Agmo", "Verme_Agmo"} {
		if len(linhas[mob]) > 0 {
			t.Errorf("%s tem %d linhas na 0109; a Mesa rola cada item sozinho e daria 0 ou vários âmagos", mob, len(linhas[mob]))
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

// O sorteio do Boss Mantícora, medido no que o jogo entrega: os 32.768 valores
// do rand() do MSVC passados por Intn(bossManticoraBase). A pedra nunca passa dos
// 10% pedidos, e os outros quatro prêmios dividem o resto em partes iguais.
func TestBossManticoraSorteio(t *testing.T) {
	vezes := map[string]int{}
	for v := range 32768 {
		vezes[bossManticoraSorteia(v%bossManticoraBase).nome]++
	}
	soma := 0
	for _, p := range bossManticoraPremios {
		soma += p.peso
		taxa := float64(vezes[p.nome]) / 32768
		if p.itemN == itemPedraDeManticora {
			if taxa > 0.10 || taxa < 0.099 {
				t.Errorf("Pedra de Mantícora sai a %.3f%%, want até 10%% (e perto disso)", taxa*100)
			}
			continue
		}
		if taxa < 0.224 || taxa > 0.227 {
			t.Errorf("%s sai a %.3f%%, want 22,5%%", p.nome, taxa*100)
		}
	}
	if soma != bossManticoraBase {
		t.Errorf("os pesos somam %d, want %d", soma, bossManticoraBase)
	}
	if last := bossManticoraPremios[len(bossManticoraPremios)-1]; last.itemN != itemPedraDeManticora {
		t.Errorf("a pedra precisa ser o último prêmio (o fim da base é onde o viés não a infla); o último é %s", last.nome)
	}
}

// Os prêmios existem no catálogo, os pacotes cabem numa pilha, e N e B são os
// âmagos pedidos: 20 Cavalo Equipado, 40 Cavalo Leve, 60 Fantasma.
func TestBossManticoraPremios(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	pacotes := map[[2]int16]int{{2399, 2404}: 20, {2398, 2403}: 40, {2397, 2402}: 60}
	for _, p := range bossManticoraPremios {
		for _, item := range []int16{p.itemN, p.itemB} {
			if item == 0 {
				continue
			}
			if _, ok := items.Get(int(item)); !ok {
				t.Errorf("%s: item %d não existe no ItemList", p.nome, item)
			}
		}
		if p.itemB == 0 {
			if p.quantidade != 1 {
				t.Errorf("%s sai com %d unidades, want 1", p.nome, p.quantidade)
			}
			continue
		}
		if want := pacotes[[2]int16{p.itemN, p.itemB}]; p.quantidade != want {
			t.Errorf("%s: pacote de %d, want %d", p.nome, p.quantidade, want)
		}
		if !isSplittable(p.itemN) || !isSplittable(p.itemB) || p.quantidade > 120 {
			t.Errorf("%s: pacote de %d não cabe numa pilha", p.nome, p.quantidade)
		}
		delete(pacotes, [2]int16{p.itemN, p.itemB})
	}
	if len(pacotes) != 0 {
		t.Errorf("pacotes pedidos que faltam: %v", pacotes)
	}
}

// Pelo abate de verdade: cada morte do Boss Mantícora põe exatamente um prêmio na
// bolsa de quem mata, com a quantidade do pacote.
func TestBossManticoraSoltaUmPremio(t *testing.T) {
	quantos := map[int16]int{}
	for _, p := range bossManticoraPremios {
		quantos[p.itemN] = p.quantidade
		if p.itemB != 0 {
			quantos[p.itemB] = p.quantidade
		}
	}
	for range 20 {
		d, w, killer := mobKilledWorld(t)
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(399, 0, 0), bossManticoraTemplate))
		achou := 0
		for _, it := range killer.Carry {
			want, ok := quantos[it.Index]
			if !ok {
				continue
			}
			achou++
			if got := itemAmount(it); got != want {
				t.Errorf("item %d com %d unidades, want %d", it.Index, got, want)
			}
		}
		if achou != 1 {
			t.Fatalf("%d prêmios do Boss Mantícora na bolsa, want 1", achou)
		}
	}
}

// O bloco do Boss Mantícora volta 5 h depois da morte; um chefe sozinho qualquer
// segue nas horas do painel.
func TestBossManticoraRenasceEm5Horas(t *testing.T) {
	boss := geradorSozinho(2_990_849, 10)
	boss.LeaderName = bossManticoraTemplate
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: geradorSozinho(2_990_849, 12), 31: boss})
	d := dispatcherQuieto()
	if got, want := d.esperaDoRenascimento(w, 31), uint32(5*msPorHora); got != want {
		t.Errorf("Boss Mantícora volta em %d ms, want %d", got, want)
	}
	if got := d.esperaDoRenascimento(w, 30); got == uint32(5*msPorHora) {
		t.Error("um chefe sozinho qualquer também ganhou as 5 h")
	}
}

// O template: o nível e o divisor do Cav. Lugefer, o dobro da vida real dele,
// mais dano que ele e que a Mantícora, o corpo da Mantícora, e nada no Carry —
// o saque é o sorteio do código, e um Carry cheio somaria o saque da Mantícora.
func TestBossManticoraTemplate(t *testing.T) {
	root := releaseDir(t)
	ler := func(nome string) savefmt.Mob {
		b, _, err := npctemplate.Load(root, nome)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		m, _, err := savefmt.DecodeMobAny(b)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		return m
	}
	boss, cav, mant := ler(bossManticoraTemplate), ler("Cav._Lugefer"), ler("Manticora")
	vidaReal := func(m savefmt.Mob) int64 {
		div := int64(1)
		v := int64(m.Equip[13].Effects[0].Value)
		if v == 0 {
			v = 2
		}
		switch m.Equip[13].Index {
		case 786:
			div = v
		case 1936:
			div = v * 10
		case 1937:
			div = v * 1000
		}
		return int64(m.CurrentScore.MaxHp) * div
	}
	if boss.CurrentScore.Level != cav.CurrentScore.Level {
		t.Errorf("nível %d, o Cav. Lugefer é %d", boss.CurrentScore.Level, cav.CurrentScore.Level)
	}
	if vidaReal(boss) < 2*vidaReal(cav) {
		t.Errorf("vida real %d, want o dobro do Cav. Lugefer (%d) ou mais", vidaReal(boss), 2*vidaReal(cav))
	}
	if boss.CurrentScore.Damage <= cav.CurrentScore.Damage || boss.CurrentScore.Damage < 3*mant.CurrentScore.Damage {
		t.Errorf("dano %d: want mais que o Cav. Lugefer (%d) e muito mais que a Mantícora (%d)",
			boss.CurrentScore.Damage, cav.CurrentScore.Damage, mant.CurrentScore.Damage)
	}
	if boss.Equip[0].Index != mant.Equip[0].Index {
		t.Errorf("corpo %d, a Mantícora é %d", boss.Equip[0].Index, mant.Equip[0].Index)
	}
	for i, it := range boss.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio", i, it.Index)
		}
	}
	if boss.BaseScore.MaxHp != boss.CurrentScore.MaxHp {
		t.Errorf("BaseScore e CurrentScore com vida diferente: %d e %d", boss.BaseScore.MaxHp, boss.CurrentScore.MaxHp)
	}
}

// A Pedra de Mantícora saiu da tropa: nenhum monstro da 0109 a solta, e a
// Mantícora comum, cujo template a tem, fica com a linha a 0%.
func TestDesertoPedraDeManticoraSoDoChefe(t *testing.T) {
	linhas := linhasDoDeserto(t)
	for mob, l := range linhas {
		if l[itemPedraDeManticora] > 0 {
			t.Errorf("%s solta a Pedra de Mantícora a %d; ela é do Boss", mob, l[itemPedraDeManticora])
		}
	}
	if c, ok := linhas["Manticora"][itemPedraDeManticora]; !ok || c != 0 {
		t.Error("a Mantícora comum precisa da linha a 0% da pedra: o template dela a solta")
	}
}
