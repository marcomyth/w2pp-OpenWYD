package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A quest paga em saque e nunca em XP — e o Troll de sempre, com outro nome de
// arquivo, continua pagando.
func TestAcampamentoTrollNaoDaXP(t *testing.T) {
	tmpl := expMobTemplate(1, 1000, 0)

	d, w, killer := mobKilledWorld(t)
	d.mobKilled(w, killer, spawnNamed(t, w, tmpl, "ATroll_Insano"))
	if killer.Exp != 0 {
		t.Errorf("matar ATroll_Insano deu %d de XP, want 0", killer.Exp)
	}

	d, w, killer = mobKilledWorld(t)
	d.mobKilled(w, killer, spawnNamed(t, w, tmpl, "Troll_Insano"))
	if killer.Exp == 0 {
		t.Error("o Troll_Insano do mundo parou de dar XP")
	}
}

// As tabelas são o design (16/09/2026): a mesma escada para todo monstro, o raro
// cada vez mais raro, teto de 63 de dano e 32 de magia; e todo valor é um degrau
// que o bônus de drop do legado já dá (dano de 9 em 9, magia de 4 em 4, skill de
// 3 em 3).
func TestAcampamentoTrollTabelasDoDesign(t *testing.T) {
	valores := func(tab []addArma, efeito uint8) []int {
		var out []int
		for _, l := range tab {
			if l.efeito != efeito || l.peso <= 0 {
				t.Errorf("linha %+v: efeito %d ou peso inválido", l, l.efeito)
			}
			out = append(out, l.valor)
		}
		return out
	}
	iguais := func(nome string, got, want []int) {
		if len(got) != len(want) {
			t.Errorf("%s = %v, want %v", nome, got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s = %v, want %v", nome, got, want)
				return
			}
		}
	}
	iguais("física dos monstros", valores(addTrollFisicaMob, efDamage), []int{27, 36, 45, 54, 63})
	iguais("física do Enigma", valores(addTrollFisicaBoss, efDamage), []int{36, 45, 54, 63})
	iguais("mágica dos monstros", valores(addTrollMagicaMob, efMagic), []int{12, 16, 20, 24, 28, 32})
	iguais("mágica do Enigma", valores(addTrollMagicaBoss, efMagic), []int{20, 24, 28, 32})

	for _, tab := range [][]addArma{addTrollFisicaMob, addTrollFisicaBoss} {
		for _, l := range tab {
			if l.valor%9 != 0 {
				t.Errorf("dano %d fora do degrau de 9", l.valor)
			}
		}
	}
	for _, tab := range [][]addArma{addTrollMagicaMob, addTrollMagicaBoss} {
		for _, l := range tab {
			if l.valor%4 != 0 {
				t.Errorf("magia %d fora do degrau de 4", l.valor)
			}
		}
	}
	if len(skillTroll) != 3 || skillTroll[0] != 15 || skillTroll[1] != 18 || skillTroll[2] != 21 {
		t.Errorf("skill = %v, want 15, 18 e 21", skillTroll)
	}
	if addTrollSkillBoss <= 0 || addTrollSkillBoss >= 100 {
		t.Errorf("chance de skill no Enigma = %d%%, want entre 1 e 99", addTrollSkillBoss)
	}
	// Quanto maior o add, mais raro: nos monstros comuns o peso só cai.
	for _, tab := range [][]addArma{addTrollFisicaMob, addTrollMagicaMob} {
		for i := 1; i < len(tab); i++ {
			if tab[i].peso >= tab[i-1].peso {
				t.Errorf("%+v pesa tanto quanto o add menor %+v", tab[i], tab[i-1])
			}
		}
	}
	// O Enigma pende para o alto: o topo dele pesa mais que o topo dos outros.
	for _, par := range [][2][]addArma{{addTrollFisicaBoss, addTrollFisicaMob}, {addTrollMagicaBoss, addTrollMagicaMob}} {
		boss, mob := par[0], par[1]
		if boss[len(boss)-1].peso*100/pesoTotal(boss) <= mob[len(mob)-1].peso*100/pesoTotal(mob) {
			t.Errorf("o topo do Enigma %+v não pesa mais que o dos monstros %+v", boss[len(boss)-1], mob[len(mob)-1])
		}
	}
}

func pesoTotal(tab []addArma) int {
	n := 0
	for _, l := range tab {
		n += l.peso
	}
	return n
}

// Toda arma que um monstro da quest solta sai refinável e com um add da tabela do
// monstro; a skill só sai do Troll Enigma, e nele nem sempre.
func TestAcampamentoTrollArmaSaiComAddDoDesign(t *testing.T) {
	for _, tc := range []struct {
		nome   string
		mob    string
		item   int16
		tabela []addArma
		boss   bool
	}{
		{"Gram da tropa", "ATroll_Insano", 869, addTrollFisicaMob, false},
		{"Arco Élfico do guardião", "ATroll_Caos", 824, addTrollFisicaMob, false},
		{"Luna do boss", "ATroll_Enigma", 910, addTrollFisicaBoss, true},
		{"Gungnir do seguidor", "ATroll_Mago", 854, addTrollMagicaMob, false},
		{"Cajado de Âmbar do boss", "ATroll_Enigma", 902, addTrollMagicaBoss, true},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			d, w, killer := mobKilledWorld(t)
			d.dropRules = droprule.NewTable([]droprule.Rule{{Mob: tc.mob, Item: tc.item, Chance: droprule.MaxChance}})
			naTabela := map[world.Effect]bool{}
			for _, l := range tc.tabela {
				naTabela[world.Effect{Effect: l.efeito, Value: uint8(l.valor)}] = true
			}
			visto := map[world.Effect]bool{}
			comSkill, semSkill := 0, 0
			const armas = 200
			for kill := 0; kill < armas; kill++ {
				for i := range killer.Carry {
					killer.Carry[i] = world.Item{}
				}
				d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(300, 0, 0), tc.mob))
				it, ok := carryHas(killer, tc.item)
				if !ok {
					t.Fatalf("%s não derrubou a arma a 100%%", tc.mob)
				}
				if it.Effects[0].Effect != efSanc || it.Effects[0].Value > 2 {
					t.Errorf("slot 0 = %+v, want EF_SANC de +0 a +2", it.Effects[0])
				}
				if !naTabela[it.Effects[1]] {
					t.Errorf("add %+v fora da tabela do design", it.Effects[1])
				}
				switch v := it.Effects[2]; {
				case v == (world.Effect{}):
					semSkill++
				case v.Effect == efSpecialAll && (v.Value == 15 || v.Value == 18 || v.Value == 21):
					comSkill++
				default:
					t.Errorf("slot 2 = %+v, want vazio ou EF_SPECIALALL 15, 18 ou 21", v)
				}
				visto[it.Effects[1]] = true
			}
			if len(visto) != len(tc.tabela) {
				t.Errorf("%d armas e só %d dos %d adds da tabela: %v", armas, len(visto), len(tc.tabela), visto)
			}
			if !tc.boss && comSkill > 0 {
				t.Errorf("%s soltou %d armas com skill; a skill é só do Enigma", tc.mob, comSkill)
			}
			if tc.boss && (comSkill == 0 || semSkill == 0) {
				t.Errorf("Enigma: %d com skill e %d sem, want as duas", comSkill, semSkill)
			}
		})
	}
}

// Fora da quest nada muda: a mesma arma de outro monstro e um item que não é arma
// de um monstro da quest passam como vieram.
func TestAcampamentoTrollNaoMexeEmOutroDrop(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	arma := world.Item{Index: 869, Effects: [3]world.Effect{{Effect: efSanc, Value: 1}, {Effect: 26, Value: 3}, {Effect: efDamage, Value: 9}}}
	antes := arma
	d.acampamentoTrollFinish(w, spawnNamed(t, w, expMobTemplate(60, 0, 0), "Troll_Insano"), &arma)
	if arma != antes {
		t.Errorf("Gram do Troll_Insano do mundo virou %+v", arma)
	}
	repletion := world.Item{Index: 4019}
	d.acampamentoTrollFinish(w, spawnNamed(t, w, expMobTemplate(300, 0, 0), "ATroll_Insano"), &repletion)
	if repletion != (world.Item{Index: 4019}) {
		t.Errorf("o Repletion da quest ganhou efeitos %+v", repletion.Effects)
	}
}

// O Troll Caos solta os âmagos em pacote (20 de s/ Sela, 10 de Fantasma) e o
// Enigma os Pergaminhos da Água em pacote de 5; o mesmo item de outro monstro da
// quest, ou do Orc, cai um só.
func TestAcampamentoTrollPacotes(t *testing.T) {
	for _, c := range []struct {
		mob  string
		item int16
		want int
	}{
		{"ATroll_Caos", 2396, 20},
		{"ATroll_Caos", 2401, 20},
		{"ATroll_Caos", 2397, 10},
		{"ATroll_Caos", 2402, 10},
		{"ATroll_Enigma", 3173, 5},
		{"ATroll_Mago", 2396, 1},
		{"ATroll_Mago", 2397, 1},
		{"ATroll_Insano", 2401, 1},
		{"ATroll_Caos", 3173, 1},
		{"ATroll_Enigma", 2396, 1},
	} {
		d, w, killer := mobKilledWorld(t)
		d.dropRules = droprule.NewTable([]droprule.Rule{{Mob: c.mob, Item: c.item, Chance: droprule.MaxChance}})
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(330, 0, 0), c.mob))
		it, ok := carryHas(killer, c.item)
		if !ok {
			t.Errorf("%s não soltou o %d a 100%%", c.mob, c.item)
			continue
		}
		if got := itemAmount(it); got != c.want {
			t.Errorf("%s soltou %d de %d, want %d", c.mob, got, c.item, c.want)
		}
	}
	for mob, packs := range acampamentoTrollPacks {
		for item := range packs {
			if !isSplittable(item) {
				t.Errorf("%s: pacote de %d, que não empilha", mob, item)
			}
		}
	}
}

// A 0069 tira arma do Troll Caos, dá a ele os quatro âmagos que saem em pacote e
// dá ao Enigma o Pergaminho da Água; cada linha é de monstro da quest, de item do
// catálogo, e o pacote de cada uma está no código.
func TestAcampamentoTrollMigracaoDosPacotes(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := migrations.FS.ReadFile("0069_acampamento_troll_pacotes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	type chave struct {
		mob  string
		item int16
	}
	got := map[chave]int32{}
	for _, r := range regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`).FindAllStringSubmatch(string(b), -1) {
		item, _ := strconv.Atoi(r[2])
		chance, _ := strconv.Atoi(r[3])
		if rule := (droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(chance)}); !rule.Valid() || chance == 0 {
			t.Errorf("%v: a Mesa de Drops recusaria esta linha, ou ela não solta nada", r[0])
		}
		if _, ok := items.Get(item); !ok {
			t.Errorf("item %d não existe no ItemList", item)
		}
		if !acampamentoTrollTemplates[droprule.Canonical(r[1])] {
			t.Errorf("%s não é monstro da quest", r[1])
		}
		got[chave{r[1], int16(item)}] = int32(chance)
	}
	want := map[chave]int32{
		{"ATroll_Caos", 2396}:   1500,
		{"ATroll_Caos", 2401}:   1000,
		{"ATroll_Caos", 2397}:   1500,
		{"ATroll_Caos", 2402}:   1000,
		{"ATroll_Enigma", 3173}: 3000,
	}
	for item := range armasTrollFisicas {
		want[chave{"ATroll_Caos", item}] = 250
	}
	for item := range armasTrollMagicas {
		want[chave{"ATroll_Caos", item}] = 250
	}
	if len(got) != len(want) {
		t.Errorf("%d linhas na 0069, want %d", len(got), len(want))
	}
	for k, chance := range want {
		if got[k] != chance {
			t.Errorf("%s item %d a %d, want %d", k.mob, k.item, got[k], chance)
		}
		if chance != 250 && acampamentoTrollPacks[droprule.Canonical(k.mob)][k.item] <= 1 {
			t.Errorf("%s item %d entra na 0069 sem pacote no código", k.mob, k.item)
		}
	}
	if !strings.Contains(string(b), "DO UPDATE SET chance = EXCLUDED.chance") {
		t.Error("a 0069 não sobrescreve as chances que a 0064 já gravou")
	}
}

// O acampamento usa a Chave do Rei Orc, então a entrada dos Elfos sorteia uma
// chave só: um sorteio ganho não pode render duas.
func TestAcampamentoTrollChaveNoTicketDosElfos(t *testing.T) {
	d, w, e := mobKilledWorld(t)
	d.eventRNG = sorteioFixo(0)
	e.Level = 330
	e.Carry[0] = world.Item{Index: itemEmblemaDoGuarda}
	if !d.useQuest256Ticket(w, &world.Session{Conn: e.ID, Mode: world.UserPlay}, e, 0) {
		t.Fatal("o ticket dos Elfos não foi tratado")
	}
	chaves := 0
	for _, it := range e.Carry {
		switch it.Index {
		case itemChaveCasteloOrc:
			chaves++
		case 3223:
			t.Error("a entrada ainda dá a antiga Chave dos Trolls (3223)")
		}
	}
	if chaves != 1 {
		t.Errorf("a entrada paga nos Elfos deu %d Chaves do Rei Orc, want 1", chaves)
	}
}

// acampamentoTrollDesign é a quest como desenhada (14/09/2026): os números do
// Castelo Orc por tier, e o rosto do Troll de onde cada template saiu, para a
// réplica ter a cara do original.
//
// Recalibrado em 16/09/2026 (pedido do Marco): o Mago com metade do HP e do dano,
// o Caos com o HP que era do Mago e o dano de antes, a tropa com metade do dano;
// o Enigma fica para depois. Todos montados e com a arma +11, só no visual.
var acampamentoTrollDesign = map[string]struct {
	name             string
	lvl, hp, ac, dmg int32
	res              int8
	face             int16
	mount            int16
}{
	"ATroll_Enigma":  {"Troll Enigma", 350, 1500000, 3000, 1010, 25, 213, 2372}, // Cavalo Fantasma B
	"ATroll_Caos":    {"Troll Caos", 330, 150000, 2400, 1620, 20, 213, 2366},    // Cavalo s/Sela N
	"ATroll_Mago":    {"Troll Mago", 320, 75000, 2200, 760, 15, 213, 2365},      // Dente de Sabre
	"ATroll_Insano":  {"Troll Insano", 300, 36000, 1800, 610, 10, 212, 2363},    // Dragão Menor
	"ATroll_Cacador": {"Caçador Troll", 300, 18000, 1800, 610, 10, 213, 2365},   // Dente de Sabre
}

// Os templates entregues são o design, e nada vem junto: sem ouro, sem XP no
// arquivo, sem drop de template e sem resistência negativa.
func TestAcampamentoTrollTemplatesBatemComODesign(t *testing.T) {
	root := releaseDir(t)
	if len(acampamentoTrollDesign) != len(acampamentoTrollTemplates) {
		t.Fatalf("%d templates no design, %d na regra do handler", len(acampamentoTrollDesign), len(acampamentoTrollTemplates))
	}
	for file, want := range acampamentoTrollDesign {
		if !acampamentoTrollTemplates[droprule.Canonical(file)] {
			t.Errorf("%s não está em acampamentoTrollTemplates", file)
		}
		b, res, err := npctemplate.Load(root, file)
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		if res.Version != savefmt.MobVersionCurrent {
			t.Errorf("%s: layout %v, want o de 816 bytes", file, res.Version)
		}
		m, err := savefmt.DecodeMob(b)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var got []rune
		for _, c := range m.Name {
			if c == 0 {
				break
			}
			got = append(got, rune(c))
		}
		if string(got) != want.name {
			t.Errorf("%s: nome %q, want %q", file, string(got), want.name)
		}
		c := m.CurrentScore
		if c.Level != want.lvl || c.MaxHp != want.hp || c.Hp != want.hp || c.AC != want.ac || c.Damage != want.dmg {
			t.Errorf("%s: nv %d hp %d/%d def %d dano %d, want nv %d hp %d def %d dano %d",
				file, c.Level, c.Hp, c.MaxHp, c.AC, c.Damage, want.lvl, want.hp, want.ac, want.dmg)
		}
		if m.BaseScore.Level != c.Level || m.BaseScore.MaxHp != c.MaxHp || m.BaseScore.AC != c.AC || m.BaseScore.Damage != c.Damage {
			t.Errorf("%s: BaseScore difere do CurrentScore", file)
		}
		if m.Resist != [4]int8{want.res, want.res, want.res, want.res} {
			t.Errorf("%s: resistência %v, want %d em tudo", file, m.Resist, want.res)
		}
		if m.Exp != 0 || m.Coin != 0 || m.Merchant != 0 || c.Merchant != 0 || m.Clan == 4 {
			t.Errorf("%s: exp %d coin %d merchant %d/%d clan %d", file, m.Exp, m.Coin, m.Merchant, c.Merchant, m.Clan)
		}
		if m.Equip[0].Index != want.face {
			t.Errorf("%s: rosto %d, want %d (o do Troll original)", file, m.Equip[0].Index, want.face)
		}
		// +11 é EF_SANC 234..237: um 11 cru o cliente lê como +1 (protocol/visual.go).
		if arma := m.Equip[6]; arma.Index == 0 || arma.Effects[0].Effect != efSanc || arma.Effects[0].Value < 234 || arma.Effects[0].Value > 237 {
			t.Errorf("%s: arma %d %+v, want +11 (EF_SANC 234..237)", file, arma.Index, arma.Effects[0])
		}
		// A montaria precisa de vida no primeiro efeito, ou aparece morta.
		if mt := m.Equip[14]; mt.Index != want.mount || mt.Effects[0].Value == 0 {
			t.Errorf("%s: montaria %d %+v, want %d com vida", file, mt.Index, mt.Effects[0], want.mount)
		}
		for slot, it := range m.Carry {
			if it.Index != 0 {
				t.Errorf("%s: drop de template %d no slot %d; o saque é da Mesa de Drops", file, it.Index, slot)
			}
		}
	}
}

// O Xamã Troll é o único NPC de quest no grau 41: outro template Merchant 100 no
// mesmo grau abriria o acampamento também.
func TestAcampamentoTrollXamaNoConteudo(t *testing.T) {
	root := releaseDir(t)
	dir := filepath.Join(root, "TMsrv", "run", "npc")
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var comGrau []string
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		norm, _, err := savefmt.NormalizeMob(b)
		if err != nil {
			continue
		}
		m, err := savefmt.DecodeMob(norm)
		if err != nil || m.CurrentScore.Merchant != 100 {
			continue
		}
		for _, ef := range m.Equip[0].Effects {
			if ef.Effect == 100 && ef.Value == gradeAcampamentoTroll {
				comGrau = append(comGrau, f.Name())
			}
		}
	}
	if len(comGrau) != 1 || droprule.Canonical(comGrau[0]) != droprule.Canonical(acampamentoTrollNPCTemplate) {
		t.Errorf("templates de quest no grau %d: %v, want só %s", gradeAcampamentoTroll, comGrau, acampamentoTrollNPCTemplate)
	}
}

// Cada monstro da quest lidera um bloco da quest (é o que deixa o GM levantá-lo
// com "criar"), nenhum desses blocos renasce sozinho, e todos nascem dentro da
// caixa da corrida. Nenhum bloco do mundo nasce na caixa, a não ser o Troll Enigma
// que a migração 0064 desliga.
func TestAcampamentoTrollBlocosDoNPCGener(t *testing.T) {
	root := releaseDir(t)
	gens, err := npcgener.Load(filepath.Join(root, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) <= world.AcampamentoTrollGenLast {
		t.Fatalf("NPCGener tem %d blocos, want pelo menos %d", len(gens), world.AcampamentoTrollGenLast+1)
	}
	sp := acampamentoTrollSpec
	if got := gens[sp.genBoss].Leader; droprule.Canonical(got) != droprule.Canonical(acampamentoTrollBoss) {
		t.Errorf("o bloco do boss (%d) lidera %q, want %s", sp.genBoss, got, acampamentoTrollBoss)
	}
	if got := gens[sp.genSeguidor].Leader; droprule.Canonical(got) != droprule.Canonical("ATroll_Mago") {
		t.Errorf("o bloco dos seguidores (%d) lidera %q, want ATroll_Mago", sp.genSeguidor, got)
	}
	// Um Enigma só, no centro do acampamento; e quatro Troll Caos por chave.
	if b := gens[sp.genBoss]; b.MaxNumMob != 1 || b.SegX[0] != sp.entrada[0] || b.SegY[0] != sp.entrada[1] {
		t.Errorf("bloco do boss: até %d em (%d,%d), want 1 no centro %v", b.MaxNumMob, b.SegX[0], b.SegY[0], sp.entrada)
	}
	caos := 0
	lidera := map[string]bool{}
	for idx := world.AcampamentoTrollGenFirst; idx <= world.AcampamentoTrollGenLast; idx++ {
		g := gens[idx]
		if droprule.Canonical(g.Leader) == droprule.Canonical("ATroll_Caos") {
			caos += g.MaxNumMob
		}
		if !acampamentoTrollTemplates[droprule.Canonical(g.Leader)] {
			t.Errorf("bloco %d lidera %q, que não é da quest", idx, g.Leader)
		}
		if g.MinuteGenerate > 0 {
			t.Errorf("bloco %d renasce a cada %d min; os da quest não renascem sozinhos", idx, g.MinuteGenerate)
		}
		if !sp.caixa.contains(g.SegX[0], g.SegY[0]) {
			t.Errorf("bloco %d nasce em (%d,%d), fora da caixa %v", idx, g.SegX[0], g.SegY[0], sp.caixa)
		}
		lidera[droprule.Canonical(g.Leader)] = true
	}
	if caos != 4 {
		t.Errorf("os blocos da quest levantam %d Troll Caos, want 4", caos)
	}
	for file := range acampamentoTrollDesign {
		if !lidera[droprule.Canonical(file)] {
			t.Errorf("%s não lidera nenhum bloco da quest: o /gm criar não o acha", file)
		}
	}
	const enigmaDoMundo = 3804
	if gens[enigmaDoMundo].Leader != "Troll_Enigma" {
		t.Errorf("bloco %d = %q, want Troll_Enigma — revise a migração 0064", enigmaDoMundo, gens[enigmaDoMundo].Leader)
	}
	for idx := range gens {
		if world.IsAcampamentoTrollGenerator(idx) {
			continue
		}
		if acampamentoTrollTemplates[droprule.Canonical(gens[idx].Leader)] {
			t.Errorf("bloco %d, fora da faixa da quest, usa %s: nasceria no mundo aberto", idx, gens[idx].Leader)
		}
		if idx != enigmaDoMundo && sp.caixa.contains(gens[idx].SegX[0], gens[idx].SegY[0]) {
			t.Errorf("bloco %d (%s) nasce dentro da caixa da corrida", idx, gens[idx].Leader)
		}
	}
}

// Cada linha da migração nomeia um monstro da quest e um item que o catálogo tem;
// cada monstro tem saque e solta as oito Armas D; os ovos de Cavalo Fantasma só
// caem do boss; e a migração desliga o Troll Enigma do mundo.
func TestAcampamentoTrollMigracaoDeDrops(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := migrations.FS.ReadFile("0064_acampamento_troll.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(b)
	rows := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`).FindAllStringSubmatch(sql, -1)
	if len(rows) == 0 {
		t.Fatal("nenhuma linha na migração")
	}
	armas := map[string]map[int16]bool{}
	for _, r := range rows {
		item, _ := strconv.Atoi(r[2])
		chance, _ := strconv.Atoi(r[3])
		rule := droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(chance)}
		if !rule.Valid() || chance == 0 {
			t.Errorf("%v: a Mesa de Drops recusaria esta linha, ou ela não solta nada", r[0])
		}
		if _, ok := items.Get(item); !ok {
			t.Errorf("item %d não existe no ItemList", item)
		}
		if !acampamentoTrollTemplates[droprule.Canonical(r[1])] {
			t.Errorf("%s não é monstro da quest", r[1])
		}
		if (item == 2307 || item == 2312) && droprule.Canonical(r[1]) != droprule.Canonical(acampamentoTrollBoss) {
			t.Errorf("o ovo %d cai de %s; os ovos são só do boss", item, r[1])
		}
		if armas[r[1]] == nil {
			armas[r[1]] = map[int16]bool{}
		}
		if armasTrollFisicas[int16(item)] || armasTrollMagicas[int16(item)] {
			armas[r[1]][int16(item)] = true
		}
	}
	for file := range acampamentoTrollDesign {
		if n := len(armas[file]); n != len(armasTrollFisicas)+len(armasTrollMagicas) {
			t.Errorf("%s solta %d das 8 Armas D", file, n)
		}
	}
	if !strings.Contains(sql, "INSERT INTO npc_generator_off (generator_index, turned_off_by) VALUES (3804,") {
		t.Error("a migração não desliga o Troll Enigma do mundo (bloco 3804)")
	}
	// O Xamã fala o nome do catálogo ("Traga a %s").
	if chave, ok := items.Get(int(acampamentoTrollSpec.chave)); !ok || chave.Name != "Chave_do_Rei_Orc" {
		t.Errorf("item %d no catálogo = %q, want Chave_do_Rei_Orc", acampamentoTrollSpec.chave, chave.Name)
	}
}

// A 0071 põe os Restos de Ori e de Lac em todo monstro da quest, uma unidade por
// drop, e mais no Caos e no Enigma.
func TestAcampamentoTrollMigracaoDosRestos(t *testing.T) {
	b, err := migrations.FS.ReadFile("0071_acampamento_troll_restos.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]map[int]int{}
	for _, r := range regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`).FindAllStringSubmatch(string(b), -1) {
		item, _ := strconv.Atoi(r[2])
		chance, _ := strconv.Atoi(r[3])
		if rule := (droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(chance)}); !rule.Valid() || chance == 0 {
			t.Errorf("%v: a Mesa de Drops recusaria esta linha, ou ela não solta nada", r[0])
		}
		if !acampamentoTrollTemplates[droprule.Canonical(r[1])] {
			t.Errorf("%s não é monstro da quest", r[1])
		}
		if got[r[1]] == nil {
			got[r[1]] = map[int]int{}
		}
		got[r[1]][item] = chance
	}
	want := map[string][2]int{
		"ATroll_Insano":  {500, 200},
		"ATroll_Cacador": {500, 200},
		"ATroll_Mago":    {500, 200},
		"ATroll_Caos":    {1500, 750},
		"ATroll_Enigma":  {5000, 3000},
	}
	for mob, w := range want {
		if got[mob][419] != w[0] || got[mob][420] != w[1] || len(got[mob]) != 2 {
			t.Errorf("%s: %v, want Resto de Ori %d e de Lac %d", mob, got[mob], w[0], w[1])
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d monstros na 0071, want %d", len(got), len(want))
	}
	for mob := range acampamentoTrollPacks {
		for _, resto := range []int16{419, 420} {
			if acampamentoTrollPacks[mob][resto] > 1 {
				t.Errorf("%s solta Resto %d em pacote; o pedido é uma unidade", mob, resto)
			}
		}
	}
}
