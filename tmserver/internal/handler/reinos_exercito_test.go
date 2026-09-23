package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/reinos"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// reinosDesign é o exército como desenhado em 17/09/2026 (docs/reinos.md): os
// números de cada template, a montaria e o brilho. O de Akelonia é o mesmo nome
// com "_", e o reino decide dano e montaria: Hekalotia é o tanque, com a tropa
// batendo mais, e Akelonia é o dano.
type reinoPapelDesign struct {
	hp, ac            int32
	dmgAzul, dmgVerm  int32
	montAzul, montVer int16
	nivelMontaria     uint8
	sanc              uint8
}

var reinosPapeis = map[papelNoReino]reinoPapelDesign{
	papelTropa:   {60000, 2600, 2600, 1900, 2375, 2370, 0, 234},
	papelElite:   {180000, 3000, 3200, 2400, 2375, 2370, 0, 234},
	papelEscolta: {500000, 3400, 4800, 4000, 2376, 2378, 35, 234},
}

func carregarMob(t *testing.T, root, file string) savefmt.Mob {
	t.Helper()
	b, res, err := npctemplate.Load(root, file)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	if res.Version != savefmt.MobVersionCurrent {
		t.Fatalf("%s: layout %v, want o de 816 bytes", file, res.Version)
	}
	m, err := savefmt.DecodeMob(b)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return m
}

func nomeDoMob(m savefmt.Mob) string {
	return strings.TrimRight(string(m.Name[:strings.IndexByte(string(m.Name[:])+"\x00", 0)]), "\x00")
}

func TestReinosTemplatesBatemComODesign(t *testing.T) {
	root := releaseDir(t)
	vistos := map[papelNoReino]int{}
	for file, papel := range papelPorTemplate {
		if papel == papelRei {
			continue
		}
		vistos[papel]++
		d := reinosPapeis[papel]
		// O mapa guarda o nome canônico; o arquivo tem a caixa do NPCGener.
		res, err := npctemplate.Resolve(root, file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		m := carregarMob(t, root, res.Name)
		dmg, mont := d.dmgAzul, d.montAzul
		switch m.Clan {
		case reinos.ClanAkelonia:
			dmg, mont = d.dmgVerm, d.montVer
		case reinos.ClanHekalotia:
		default:
			t.Fatalf("%s: clã %d, want 7 ou 8", res.Name, m.Clan)
		}
		if strings.HasSuffix(res.Name, "_") != (m.Clan == reinos.ClanAkelonia) {
			t.Errorf("%s: clã %d não bate com o nome", res.Name, m.Clan)
		}
		for _, s := range []savefmt.Score{m.BaseScore, m.CurrentScore} {
			if s.MaxHp != d.hp || s.Hp != d.hp || s.AC != d.ac || s.Damage != dmg {
				t.Errorf("%s: hp %d/%d def %d dano %d, want hp %d def %d dano %d",
					res.Name, s.Hp, s.MaxHp, s.AC, s.Damage, d.hp, d.ac, dmg)
			}
		}
		if mt := m.Equip[14]; mt.Index != mont || mt.Effects[0].Value == 0 || mt.Effects[1].Effect != d.nivelMontaria {
			t.Errorf("%s: montaria %+v, want %d com vida e nível %d", res.Name, mt, mont, d.nivelMontaria)
		}
		if arma := m.Equip[6]; arma.Index == 0 || arma.Effects[0] != (savefmt.Effect{Effect: efSanc, Value: d.sanc}) {
			t.Errorf("%s: arma %+v, want +11 (EF_SANC %d)", res.Name, arma, d.sanc)
		}
		for i, r := range m.Resist {
			if r < 0 {
				t.Errorf("%s: resistência %d = %d; negativa vira 100 no servidor", res.Name, i, r)
			}
		}
		for slot, it := range m.Carry {
			if it.Index != 0 {
				t.Errorf("%s: drop de template %d no slot %d; o saque é da Mesa de Drops", res.Name, it.Index, slot)
			}
		}
		if papel == papelEscolta && (m.Exp != 0 || m.Coin != 0 || m.Merchant != 0 || m.CurrentScore.Merchant != 0) {
			t.Errorf("%s: exp %d coin %d merchant %d/%d", res.Name, m.Exp, m.Coin, m.Merchant, m.CurrentScore.Merchant)
		}
	}
	if vistos[papelTropa] != 10 || vistos[papelElite] != 6 || vistos[papelEscolta] != 2 {
		t.Errorf("templates por papel = %v, want tropa 10, elite 6, escolta 2", vistos)
	}
}

// Os Reis: vida de 6 milhões, o Vermelho no dano e o Azul no tanque, Unicórnio,
// a arma celestial +12, a capa celestial do reino e a Coroa Celestial. Continuam
// NPC do Arch (Merchant 111) e com o Símbolo de Coragem no saque.
func TestReinosReisBatemComODesign(t *testing.T) {
	root := releaseDir(t)
	for _, c := range []struct {
		file       string
		clan       uint8
		dmg        int32
		arma, capa int16
	}{
		{templateReiHarabard, reinos.ClanHekalotia, 3500, 3596, 3197},
		{templateReiGlantuar, reinos.ClanAkelonia, 12000, 3591, 3198},
	} {
		m := carregarMob(t, root, c.file)
		s := m.CurrentScore
		if m.Clan != c.clan || s.Level != 399 || s.MaxHp != 6000000 || s.Hp != 6000000 || s.AC != 4000 || s.Damage != c.dmg {
			t.Errorf("%s: clã %d nv %d hp %d/%d def %d dano %d", c.file, m.Clan, s.Level, s.Hp, s.MaxHp, s.AC, s.Damage)
		}
		if m.BaseScore.MaxHp != s.MaxHp || m.BaseScore.Damage != s.Damage || m.BaseScore.AC != s.AC {
			t.Errorf("%s: BaseScore difere do CurrentScore", c.file)
		}
		mais12 := savefmt.Effect{Effect: efSanc, Value: 238}
		if m.Equip[6].Index != c.arma || m.Equip[6].Effects[0] != mais12 {
			t.Errorf("%s: arma %+v, want %d +12", c.file, m.Equip[6], c.arma)
		}
		if m.Equip[1].Index != 3303 || m.Equip[15].Index != c.capa {
			t.Errorf("%s: coroa %d capa %d, want 3303 e %d", c.file, m.Equip[1].Index, m.Equip[15].Index, c.capa)
		}
		if mt := m.Equip[14]; mt.Index != 2381 || mt.Effects[0].Value == 0 || mt.Effects[1].Effect != 120 {
			t.Errorf("%s: montaria %+v, want Unicórnio nível 120 com vida", c.file, mt)
		}
		if s.Merchant != kingQuestMerchant || m.Carry[56].Index != 1732 {
			t.Errorf("%s: merchant %d, saque %d; o Rei continua NPC do Arch e com o Símbolo", c.file, s.Merchant, m.Carry[56].Index)
		}
	}
}

// As Lendas: um template por classe, todas "Lenda_Passada_", NPC de fala (Merchant
// 100, grau 42), set e arma +15, capa dos Aventureiros e Unicórnio 120.
func TestReinosLendasBatemComODesign(t *testing.T) {
	root := releaseDir(t)
	for _, c := range []struct {
		file      string
		class     uint8
		set, arma int16
	}{
		{"Lenda_TK", 0, 1230, 3765},
		{"Lenda_FM", 1, 1365, 3665},
		{"Lenda_BM", 2, 1515, 3733},
		{"Lenda_HT", 3, 1665, 3625},
	} {
		m := carregarMob(t, root, c.file)
		if nomeDoMob(m) != "Lenda_Passada_" || m.Class != c.class {
			t.Errorf("%s: nome %q classe %d", c.file, nomeDoMob(m), m.Class)
		}
		if m.CurrentScore.Merchant != 100 || m.Equip[0].Effects[0] != (savefmt.Effect{Effect: 100, Value: gradeLendaPassada}) {
			t.Errorf("%s: merchant %d, rosto %+v; sem o grau 42 o clique não fala", c.file, m.CurrentScore.Merchant, m.Equip[0])
		}
		mais15 := savefmt.Effect{Effect: efSanc, Value: 250}
		for i := int16(0); i < 4; i++ {
			if it := m.Equip[2+i]; it.Index != c.set+i || it.Effects[0] != mais15 {
				t.Errorf("%s: peça %d = %+v, want %d +15", c.file, 2+i, it, c.set+i)
			}
		}
		if m.Equip[6].Index != c.arma || m.Equip[6].Effects[0] != mais15 || m.Equip[15].Index != 3199 {
			t.Errorf("%s: arma %+v capa %d", c.file, m.Equip[6], m.Equip[15].Index)
		}
		if mt := m.Equip[14]; mt.Index != 2381 || mt.Effects[1].Effect != 120 || mt.Effects[0].Value == 0 {
			t.Errorf("%s: montaria %+v", c.file, mt)
		}
	}
}

// Os blocos: os Reis sem período de minuto (voltam em 4 h pela fila), seis
// cavaleiros da Escolta em volta de cada Rei, dentro da sala do trono, e as quatro
// Lendas em volta do Dragão.
func TestReinosBlocosDoNPCGener(t *testing.T) {
	root := releaseDir(t)
	gens, err := npcgener.Load(filepath.Join(root, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{kingHarabardGen, kingGlantuarGen} {
		if gens[i].MinuteGenerate > 0 {
			t.Errorf("bloco %d (%s) com período %d: o relógio de minuto o traria de volta antes das 4 h", i, gens[i].Leader, gens[i].MinuteGenerate)
		}
	}
	for i := world.EscoltaDoTronoGenFirst; i <= world.EscoltaDoTronoGenLast; i++ {
		g := gens[i]
		sala, lider := kingdom1Room, "Escolta_Real"
		if i >= world.EscoltaDoTronoGenFirst+6 {
			sala, lider = kingdom2Room, "Escolta_Real_"
		}
		if g.Leader != lider || g.MinuteGenerate > 0 || g.MaxNumMob != 1 || !sala.contains(g.SegX[0], g.SegY[0]) {
			t.Errorf("bloco %d = %s período %d max %d em (%d,%d), want %s na sala do trono",
				i, g.Leader, g.MinuteGenerate, g.MaxNumMob, g.SegX[0], g.SegY[0], lider)
		}
		if world.IsEventOwnedGenerator(i) {
			t.Errorf("bloco %d é de evento e não nasceria no boot", i)
		}
	}
	for k, cls := range []string{"TK", "FM", "BM", "HT"} {
		g := gens[world.EscoltaDoTronoGenLast+1+k]
		if g.Leader != "Lenda_"+cls || abs16(g.SegX[0]-2068) > 4 || abs16(g.SegY[0]-2068) > 4 {
			t.Errorf("Lenda %s: bloco %s em (%d,%d)", cls, g.Leader, g.SegX[0], g.SegY[0])
		}
	}
	// +6, não +5, desde 19/09/2026: a Loja de Pontos entrou depois das Lendas.
	// +7 desde 23/09/2026: o Boss Mantícora do Deserto (migração 0109), no 6145.
	// As Lendas continuam onde estavam — são lidas por índice absoluto logo
	// acima —, e é justamente por isso que bloco novo vai sempre no FIM.
	if len(gens) != world.EscoltaDoTronoGenLast+7 {
		t.Errorf("%d blocos, want %d: bloco novo entra no fim, depois das Lendas", len(gens), world.EscoltaDoTronoGenLast+7)
	}
}

// A 0074: cada linha é de monstro do Reino (ou "*" a 0%), de item do catálogo, e
// o que o desenho manda em pacote tem pacote no código. As Almas só nos Reis.
func TestReinosMigracaoDoSaque(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := migrations.FS.ReadFile("0074_reinos_saque.up.sql")
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
		if rule := (droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(chance)}); !rule.Valid() {
			t.Errorf("%v: a Mesa de Drops recusaria esta linha", r[0])
		}
		if _, ok := items.Get(item); !ok {
			t.Errorf("item %d não existe no ItemList", item)
		}
		if r[1] != droprule.AllMobs && papelPorTemplate[droprule.Canonical(r[1])] == papelNenhum {
			t.Errorf("%s não é monstro do Reino", r[1])
		}
		got[chave{r[1], int16(item)}] = int32(chance)
	}
	for _, alma := range []int16{1740, 1741, 1742} {
		if c, ok := got[chave{"*", alma}]; !ok || c != 0 {
			t.Errorf("item %d não sai de todo monstro", alma)
		}
	}
	for k, c := range got {
		if (k.item == 1740 && k.mob != templateReiHarabard && k.mob != "*") ||
			(k.item == 1741 && k.mob != templateReiGlantuar && k.mob != "*") {
			t.Errorf("Alma %d em %s", k.item, k.mob)
		}
		// Âmago de um reino no outro: a cor segue o reino.
		verm := strings.HasSuffix(k.mob, "_") || k.mob == templateReiGlantuar
		if (k.item == 2405 || k.item == 2406) && verm || (k.item == 2400 || k.item == 2408) && !verm {
			t.Errorf("%s solta o âmago %d, da cor do outro reino", k.mob, k.item)
		}
		_ = c
	}
	want := map[chave]int32{
		{templateReiHarabard, 1740}: 500, {templateReiGlantuar, 1741}: 500,
		{templateReiHarabard, 4027}: 10000, {"Escolta_Real", 3224}: 200, {"Escolta_Real_", 3224}: 200,
		{"Escolta_Real_", 2408}: 1200, {"Cav._Real", 2405}: 800, {"Bruxa_", 4018}: 600, {"Virago", 4026}: 300,
	}
	for k, c := range want {
		if got[k] != c {
			t.Errorf("%s item %d a %d, want %d", k.mob, k.item, got[k], c)
		}
	}
	// 3 regras "*", 6 por Rei, 6 por Escolta e 5 por template de tropa e elite.
	if len(got) != 3+2*6+2*6+16*5 {
		t.Errorf("%d linhas na 0074, want %d", len(got), 3+2*6+2*6+16*5)
	}
	for papel, pacotes := range reinosPacotes {
		for item := range pacotes {
			achou := false
			for k := range got {
				if k.item == item && papelPorTemplate[droprule.Canonical(k.mob)] == papel {
					achou = true
				}
			}
			if !achou {
				t.Errorf("pacote do item %d no papel %d sem linha na 0074", item, papel)
			}
		}
	}
	if !strings.Contains(string(b), "DO UPDATE SET chance = EXCLUDED.chance") {
		t.Error("a 0074 não sobrescreve o que o painel tiver gravado")
	}
	if !strings.Contains(strings.Join(strings.Fields(string(b)), " "),
		"DELETE FROM drop_rule WHERE item IN (1740, 1741, 1742) AND mob NOT IN ('*', 'Rei_Harabard', 'Rei_Glantuar');") {
		t.Error("a 0074 deixa uma regra nomeada de Alma do painel valendo por cima do \"*\"")
	}
}

// monstroReino levanta um monstro de template name em (x, y), no bloco gen.
func monstroReino(t *testing.T, w *world.World, name string, clan uint8, hp uint32, x, y int16, gen int16) *world.Entity {
	t.Helper()
	b := targetMobWithClan(name, clan, 0, hp)
	id := w.SpawnMobAt(world.MobSpawn{Template: b, TemplateName: name, X: x, Y: y, GenIndex: gen})
	if id < 0 {
		t.Fatal("SpawnMobAt falhou")
	}
	e := w.Entity(id)
	e.MaxHP, e.HP = int32(hp), int32(hp)
	return e
}

func reinosFixture(t *testing.T) (*Dispatcher, *world.World) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 4096}, log, nil, d.Handle)
	return d, w
}

// O Rei Azul absorve 60% de todo golpe; o Vermelho e o resto do exército, nada.
func TestReinosAbsorcaoDoReiAzul(t *testing.T) {
	d, w := reinosFixture(t)
	azul := monstroReino(t, w, templateReiHarabard, 7, 6000000, 1750, 1574, kingHarabardGen)
	verm := monstroReino(t, w, templateReiGlantuar, 8, 6000000, 1750, 1880, kingGlantuarGen)
	guarda := monstroReino(t, w, "Bruxa", 7, 60000, 1706, 1686, 2703)
	casos := []struct {
		nome string
		e    *world.Entity
		dano int
		want int
	}{
		{"Rei Azul", azul, 1000, 400},
		{"Rei Azul, golpe de 1", azul, 1, 1},
		{"Rei Vermelho", verm, 1000, 1000},
		{"Bruxa azul", guarda, 1000, 1000},
	}
	for _, c := range casos {
		if got := d.absorbBlow(w, c.e, c.dano, true); got != c.want {
			t.Errorf("%s: golpe %d = %d, want %d", c.nome, c.dano, got, c.want)
		}
	}
}

// Os Reis e a Escolta voltam em 4 horas; a tropa segue a regra de sempre.
func TestReinosTronoVoltaEmQuatroHoras(t *testing.T) {
	d, w := reinosFixture(t)
	quatro := uint32(4 * msPorHora)
	for _, idx := range []int{kingHarabardGen, kingGlantuarGen, world.EscoltaDoTronoGenFirst, world.EscoltaDoTronoGenLast} {
		if got := d.esperaDoRenascimento(w, idx); got != quatro {
			t.Errorf("bloco %d volta em %d ms, want %d", idx, got, quatro)
		}
	}
	for _, idx := range []int{2703, world.EscoltaDoTronoGenLast + 1} {
		if d.esperaDoRenascimento(w, idx) == quatro {
			t.Errorf("bloco %d também ganhou as 4 h", idx)
		}
	}
}

// O aviso de Rei sob ataque sai uma vez por luta, abaixo de 90%, e a luta acaba
// com a vida cheia de novo ou com a morte.
func TestReinosAvisoDeReiSobAtaque(t *testing.T) {
	d, w := reinosFixture(t)
	rei := monstroReino(t, w, templateReiHarabard, 7, 6000000, 1750, 1574, kingHarabardGen)
	d.vigiarReis(w)
	if d.reiAvisado[0] {
		t.Fatal("aviso com o Rei de vida cheia")
	}
	rei.HP = 5400001 // 90% e um pouco
	d.vigiarReis(w)
	if d.reiAvisado[0] {
		t.Fatal("aviso com o Rei acima de 90%")
	}
	rei.HP = 5399999
	d.vigiarReis(w)
	if !d.reiAvisado[0] || d.reiAvisado[1] {
		t.Fatalf("avisado = %v, want só Hekalotia", d.reiAvisado)
	}
	rei.HP = rei.MaxHP
	d.vigiarReis(w)
	if d.reiAvisado[0] {
		t.Error("o Rei voltou à vida cheia e a próxima luta não avisaria")
	}
	d.reiAvisado[0] = true
	d.avisarQuedaDoRei(w, nil, rei)
	if d.reiAvisado[0] {
		t.Error("a queda do Rei não encerrou a luta")
	}
}

// O pacote do Reino: empilhável numa pilha só, e nada fora do papel. A Moeda de
// Prata saía em dez cópias soltas até 20/09/2026, quando entrou na lista de
// pilha (internal/pilha) — agora ela segue a mesma regra dos outros.
func TestReinosPacotes(t *testing.T) {
	_, w := reinosFixture(t)
	tropa := monstroReino(t, w, "Bruxa_", 8, 60000, 1706, 1766, 2674)
	rei := monstroReino(t, w, templateReiGlantuar, 8, 6000000, 1750, 1880, kingGlantuarGen)
	fora := monstroReino(t, w, "Bruxa_", 8, 60000, 2600, 1700, -1)
	casos := []struct {
		nome        string
		mob         *world.Entity
		item        int16
		copias, qtd int
	}{
		{"âmago da tropa", tropa, reinoAmagoAndaluzN, 1, 5},
		{"Classe C da tropa", tropa, reinoClasseC, 1, 5},
		{"Poeira da tropa, sem pacote", tropa, jeffiPoeiraOri, 1, 1},
		{"Poeira do Rei", rei, jeffiPoeiraOri, 1, 30},
		{"Moeda do Rei", rei, reinoMoeda5Mi, 1, 10},
		{"Alma do Rei", rei, 1741, 1, 1},
		{"a mesma Bruxa fora dos Reinos", fora, reinoAmagoAndaluzN, 1, 1},
	}
	for _, c := range casos {
		it := world.Item{Index: c.item}
		if isSplittable(c.item) {
			setItemAmount(&it, 1)
		}
		if got := reinosFinish(c.mob, &it); got != c.copias || itemAmount(it) != c.qtd {
			t.Errorf("%s: %d cópias de %d, want %d de %d", c.nome, got, itemAmount(it), c.copias, c.qtd)
		}
	}
}

// Pela Mesa de Drops: o Rei a 100% entrega as dez Moedas e a pilha de 30 Poeiras
// na bolsa de quem mata.
func TestReinosSaqueDoReiNaBolsa(t *testing.T) {
	d, w := reinosFixture(t)
	d.setDropRules(droprule.Config{Version: 1, Rules: []droprule.Rule{
		{Mob: templateReiGlantuar, Item: reinoMoeda5Mi, Chance: droprule.MaxChance},
		{Mob: templateReiGlantuar, Item: jeffiPoeiraOri, Chance: droprule.MaxChance},
	}})
	rei := monstroReino(t, w, templateReiGlantuar, 8, 6000000, 1750, 1880, kingGlantuarGen)
	matador := &world.Entity{ID: 0, Mode: world.MobUser, Name: "Heroi", Level: 1}
	d.dropTableRolls(w, matador, rei, 0)
	if got := contaNaBolsa(matador, reinoMoeda5Mi); got != 10 {
		t.Errorf("Moedas = %d, want 10", got)
	}
	if got := contaNaBolsa(matador, jeffiPoeiraOri); got != 30 {
		t.Errorf("Poeiras = %d, want 30", got)
	}
}

// A vigia roda sozinha no pulso de seis segundos dos Reinos.
func TestReinosVigiaNoPulsoDosReinos(t *testing.T) {
	d, w := reinosFixture(t)
	rei := monstroReino(t, w, templateReiGlantuar, 8, 6000000, 1750, 1880, kingGlantuarGen)
	rei.HP = 100
	d.tickCount = kingdomTickPeriod * 7
	d.tickKingdomRvR(w)
	if !d.reiAvisado[1] {
		t.Error("o pulso dos Reinos não vigiou o Rei")
	}
}
