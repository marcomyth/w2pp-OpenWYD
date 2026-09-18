package handler

import (
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprate"
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
)

// A limpeza dos LE (0080, decisão de 17/09/2026): as peças (Le) e as pedras que
// fazem LE só caem na Vila Amald e no Kefra, e os monstros dessas duas áreas
// ficaram com HP ×5, dano ×2 e defesa +30%, chefes inclusive.

func itemLE(i int) bool { return (i >= 575 && i <= 578) || (i >= 2171 && i <= 2250) }

// naVilaAmald: blocos do NPCGener 4050-4068 (os Amald) e 4078, 4079 e 4177 (o
// Verid). noKefra: a caixa do Kefra Esquerda, Direita e Meio; o Kefra City
// (3300,1750) é outro lugar.
func naVilaAmald(bloco int) bool {
	return (bloco >= 4050 && bloco <= 4068) || bloco == 4078 || bloco == 4079 || bloco == 4177
}

func noKefra(x, y int16) bool { return x >= 2180 && x <= 2560 && y >= 3840 && y <= 4100 }

// statsLE é o desenho: HP, dano e defesa de cada template depois da mudança. O
// Krill do Kefra Meio (nível 8, nasce também em Armia) ficou de fora.
//
// Três exceções à conta de HP ×5, dano ×2 e defesa +30%:
//
//   - o Ranger Amald tinha dano 7.000, o dobro dos outros três Amald, e fica com
//     os 4.300 deles: 14.000 mata um jogador de defesa comum num golpe;
//   - o Kefra é chefe de guilda: 56 milhões de HP com o divisor ÷20 do slot 13,
//     1,12 bilhão efetivo, e dano 20.000, que mata em UM golpe qualquer classe,
//     até um TK montado;
//   - o Lich Crunt VOLTOU aos números do template (800.000, 5.000, 6.000): o
//     divisor já multiplica a vida dele por 20, e o ×5 em cima disso fazia uma
//     party de seis levar mais de 3 h no bicho.
//
// Os tempos saem da simulação de raide (simulacao_chefes_test.go, tag simulacao),
// com o divisor ligado: o Kefra cai em 2 h com 20 jogadores e NÃO cai com 16 ou
// menos, porque o dano de mono derruba mais rápido do que os mortos voltam; o
// Lich Crunt cai em 51 min com seis.
var statsLE = map[string]struct{ hp, dmg, ac int32 }{
	"Templario_Amald": {50000, 4320, 4810},
	"Mago_Amald":      {50000, 4200, 3900},
	"Shama_Amald":     {50000, 4400, 4810},
	"Ranger_Amald":    {75000, 4300, 4810},
	"Verid":           {160000, 7200, 6500},
	"Verid_":          {160000, 7200, 6500},
	"Batorero":        {160000, 6720, 5590},
	"Batorero__":      {160000, 4720, 5590},
	"FunerSickler":    {160000, 2600, 5200},
	"Funer_Scyther":   {160000, 3800, 4420},
	"Funer_Seamer":    {160000, 3800, 4420},
	"Horizon_Cropper": {135000, 3300, 4225},
	"Lich_Batama":     {125000, 10000, 7800},
	"Lich_Crunt":      {800000, 5000, 6000},
	"Simio":           {160000, 2660, 5590},
	"Simio_Bleg":      {100000, 6360, 5070},
	"Talos_Imortal":   {160000, 10000, 9100},
	"Xeno_Cropper":    {135000, 3300, 4225},
	"FunerSeamer":     {160000, 2600, 5460},
	"Funer_Momenter":  {160000, 4200, 5070},
	"Funer_Sickler":   {150000, 4580, 6435},
	"GrubSwarm":       {105000, 3220, 3705},
	"HorizonCropper":  {135000, 3300, 4225},
	"Simio_Inf":       {150000, 4380, 6305},
	"WriggleSwarm":    {105000, 3300, 3705},
	"Aranha_Dourada":  {125000, 3260, 3640},
	"Aranha_Rubra":    {130000, 3200, 4030},
	"Kefra":           {56000000, 20000, 0},
	"LiggleSwarm":     {105000, 3300, 3705},
	"Mago_Negro":      {160000, 10000, 6500},
	"Rainha_Rubra":    {160000, 7000, 4810},
	"Serva_Rubra":     {128000, 4600, 3250},
}

// npcDeServico diz se o template é NPC (porteiro, mercador): os dois bytes de
// Merchant, porque o legado roteia por um e o port pelo outro
// (merchant-tem-dois-bytes).
func npcDeServico(t *testing.T, root, file string) bool {
	t.Helper()
	b, _, err := npctemplate.Load(root, file)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	m, err := savefmt.DecodeMob(b)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return m.Merchant != 0 || m.CurrentScore.Merchant != 0
}

// Todo monstro que nasce na vila ou no Kefra está no desenho, e cada template do
// desenho tem os números nas duas cópias do score.
func TestLimpezaLETemplatesComOsNumerosNovos(t *testing.T) {
	root := releaseDir(t)
	gens, err := npcgener.Load(filepath.Join(root, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	nasce := map[string]bool{}
	for i, g := range gens {
		if naVilaAmald(i) || noKefra(g.SegX[0], g.SegY[0]) {
			nasce[g.Leader], nasce[g.Follower] = true, true
		}
	}
	for n := range nasce {
		if _, ok := statsLE[n]; ok || n == "Krill" || n == "" || npcDeServico(t, root, n) {
			continue
		}
		t.Errorf("%s nasce na vila ou no Kefra e não está no desenho", n)
	}
	for file, want := range statsLE {
		if !nasce[file] {
			t.Errorf("%s está no desenho e não nasce na vila nem no Kefra", file)
		}
		b, _, err := npctemplate.Load(root, file)
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		m, err := savefmt.DecodeMob(b)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, s := range []savefmt.Score{m.BaseScore, m.CurrentScore} {
			if s.MaxHp != want.hp || s.Hp != want.hp || s.Damage != want.dmg || s.AC != want.ac {
				t.Errorf("%s: hp %d/%d dano %d def %d, want hp %d dano %d def %d",
					file, s.Hp, s.MaxHp, s.Damage, s.AC, want.hp, want.dmg, want.ac)
			}
		}
	}
}

// A 0080 zera as 84 peças e pedras em todo monstro e devolve, em cada monstro da
// vila e do Kefra, a chance que o template dava (sem bônus de drop, arredondada
// ao centésimo de por cento). Nenhuma regra nomeada fora das duas áreas.
func TestLimpezaLEMigracao(t *testing.T) {
	root := releaseDir(t)
	b, err := migrations.FS.ReadFile("0080_limpeza_le.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`generate_series\(575, 578\)`).Match(b) || !regexp.MustCompile(`generate_series\(2171, 2250\)`).Match(b) {
		t.Error("a regra '*' não cobre 575-578 e 2171-2250")
	}
	got := map[string]map[int]int{}
	for _, r := range regexp.MustCompile(`\('([^'*]+)',\s*(\d+),\s*(\d+)\)`).FindAllStringSubmatch(string(b), -1) {
		item, _ := strconv.Atoi(r[2])
		chance, _ := strconv.Atoi(r[3])
		if !(droprule.Rule{Mob: r[1], Item: int16(item), Chance: int32(chance)}).Valid() {
			t.Errorf("%v: a Mesa de Drops recusaria esta linha", r[0])
		}
		if _, ok := statsLE[r[1]]; !ok {
			t.Errorf("%s ganha LE e não é da vila nem do Kefra", r[1])
		}
		if !itemLE(item) {
			t.Errorf("%s: item %d não é LE nem pedra", r[1], item)
		}
		if got[r[1]] == nil {
			got[r[1]] = map[int]int{}
		}
		got[r[1]][item] = chance
	}

	for file := range statsLE {
		tb, _, err := npctemplate.Load(root, file)
		if err != nil {
			t.Fatal(err)
		}
		m, err := savefmt.DecodeMob(tb)
		if err != nil {
			t.Fatal(err)
		}
		falha := map[int]float64{}
		for slot, it := range m.Carry {
			if !itemLE(int(it.Index)) {
				continue
			}
			p := 1.0
			if r := droprate.EffectiveDropRate(slot, 0, int(m.CurrentScore.Level)); r > 0 {
				p = 1 / float64(r)
			}
			if _, ok := falha[int(it.Index)]; !ok {
				falha[int(it.Index)] = 1
			}
			falha[int(it.Index)] *= 1 - p
		}
		if len(falha) != len(got[file]) {
			t.Errorf("%s: template com %d itens LE, migração com %d", file, len(falha), len(got[file]))
		}
		for item, f := range falha {
			want := int(math.Round((1 - f) * 10000))
			if c, ok := got[file][item]; !ok || c != want {
				t.Errorf("%s item %d: migração %d (tem=%v), template %d", file, item, c, ok, want)
			}
		}
	}
}
