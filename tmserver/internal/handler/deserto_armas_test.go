package handler

import (
	"bytes"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/ciclopes"
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const desertoArmasMigracao = "0180_deserto_armas_d_e_barra_dos_chefes.up.sql"

// efeitoDoCatalogo lê o valor de um EF_* na linha do ItemList, ou -1 se não há.
func efeitoDoCatalogo(e content.ItemEntry, nome string) int {
	for i := 0; i+1 < len(e.Fields); i++ {
		if strings.TrimSpace(e.Fields[i]) == nome {
			v, err := strconv.Atoi(strings.TrimSpace(e.Fields[i+1]))
			if err == nil {
				return v
			}
		}
	}
	return -1
}

// As dezenove armas da lista são Armas D no catálogo, e a escada de cada uma
// bate com o que ela é: EF_MAGIC leva magia, o resto dano.
func TestArmasDDoDesertoBatemComOCatalogo(t *testing.T) {
	items, err := content.LoadItemList(filepath.Join(releaseDir(t), "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	checa := func(ids map[int16]bool, magica bool) {
		for id := range ids {
			e, ok := items.Get(int(id))
			if !ok {
				t.Errorf("arma %d não existe no ItemList", id)
				continue
			}
			if lv := efeitoDoCatalogo(e, "EF_ITEMLEVEL"); lv != 4 {
				t.Errorf("%d %s tem EF_ITEMLEVEL %d, want 4 (Arma D)", id, e.Name, lv)
			}
			if tem := efeitoDoCatalogo(e, "EF_MAGIC") > 0; tem != magica {
				t.Errorf("%d %s: EF_MAGIC %v, mas está na lista de magia=%v", id, e.Name, tem, magica)
			}
		}
	}
	checa(armasDDoDesertoFisicas, false)
	checa(armasDDoDesertoMagicas, true)
	if n := len(armasDDoDesertoFisicas) + len(armasDDoDesertoMagicas); n != 19 {
		t.Errorf("%d Armas D na lista, want 19", n)
	}
}

// Toda Arma D que os quatro soltam pelo template está na lista, e é exatamente o
// que a 0180 põe na Mesa: arma do template fora da Mesa continuaria sem add.
func TestArmasDDoDesertoCobremOTemplate(t *testing.T) {
	root := releaseDir(t)
	linhas := linhasComZero(t, desertoArmasMigracao)
	for _, mob := range []string{"Taron_Assassino", "Adamant_Tauron", "Manticora", "Verme_"} {
		b, _, err := npctemplate.Load(root, mob)
		if err != nil {
			t.Fatalf("%s: %v", mob, err)
		}
		noTemplate := map[int16]int{}
		for _, it := range protocol.MobCarry(b) {
			id := int16(it.Index)
			if armasDDoDesertoFisicas[id] || armasDDoDesertoMagicas[id] {
				noTemplate[id]++
			}
		}
		if len(noTemplate) == 0 {
			t.Fatalf("%s não tem Arma D no template: o teste não prova nada", mob)
		}
		for id, vagas := range noTemplate {
			c, ok := linhas[mob][id]
			if !ok {
				t.Errorf("%s solta a Arma D %d pelo template e ela não está na 0180", mob, id)
				continue
			}
			// O dobro do que as vagas pagavam: 0,05% cada (g_pDropRate 2000).
			want := 2 * (1 - math.Pow(1-1.0/2000, float64(vagas)))
			if got := taxaPaga(c); math.Abs(got-want) > want*0.05 {
				t.Errorf("%s arma %d em %d vaga(s): chance %d paga %.4f%%, want %.4f%%", mob, id, vagas, c, got*100, want*100)
			}
		}
		for id := range linhas[mob] {
			if noTemplate[id] == 0 {
				t.Errorf("%s: a 0180 dá a arma %d, que o template não solta", mob, id)
			}
		}
	}
}

// Os quatro chefes soltam a Barra de Prata (100Mi) em toda morte.
func TestChefesSoltamBarra100Mi(t *testing.T) {
	linhas := linhasComZero(t, desertoArmasMigracao)
	for _, mob := range []string{"Boss_Manticora", "Boss_Hidra_Dourada", "Boss_Dragao_Lich", "Frenzy_Hidra"} {
		c, ok := linhas[mob][itemBarraPrata100Mi]
		if !ok || taxaPaga(c) != 1 {
			t.Errorf("%s: Barra 100Mi com chance %d (tem=%v), want 100%%", mob, c, ok)
		}
		if _, _, err := npctemplate.Load(releaseDir(t), mob); err != nil {
			t.Errorf("%s não é template: %v", mob, err)
		}
	}
}

// A Arma D que um dos cinco solta pela Mesa sai com o add da tropa, mantendo o
// refino do bônus de drop; o Escudo de Runas e os monstros de fora não mudam.
func TestDesertoCarimbaArmasD(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	for _, mob := range []string{"Taron_Assassino", "Adamant_Tauron", "Aeon_Tauron", "Manticora", "Verme_"} {
		m := spawnNamed(t, w, expMobTemplate(370, 0, 0), mob)
		for id := range armasDDoDesertoFisicas {
			arma := world.Item{Index: id, Effects: [3]world.Effect{{Effect: efSanc, Value: 1}, {Effect: 26, Value: 3}, {Effect: efDamage, Value: 9}}}
			d.desertoFinish(w, m, &arma)
			if arma.Effects[0] != (world.Effect{Effect: efSanc, Value: 1}) || arma.Effects[1].Effect != efDamage ||
				!slices.Contains([]int{27, 36, 45, 54, 63, 72}, int(arma.Effects[1].Value)) || arma.Effects[2] != (world.Effect{}) {
				t.Fatalf("%s arma %d: %+v, want +1 e dano 27-72", mob, id, arma.Effects)
			}
		}
		for id := range armasDDoDesertoMagicas {
			arma := world.Item{Index: id}
			d.desertoFinish(w, m, &arma)
			if arma.Effects[1].Effect != efMagic || !slices.Contains([]int{12, 16, 20, 24, 28, 32}, int(arma.Effects[1].Value)) {
				t.Fatalf("%s arma %d: %+v, want magia 12-32", mob, id, arma.Effects)
			}
		}
		escudo := world.Item{Index: 1710, Effects: [3]world.Effect{{Effect: efSanc, Value: 2}}}
		antes := escudo
		d.desertoFinish(w, m, &escudo)
		if escudo != antes {
			t.Errorf("%s: o Escudo de Runas ganhou add: %+v", mob, escudo)
		}
	}
	antes := world.Item{Index: 809, Effects: [3]world.Effect{{Effect: efSanc, Value: 1}, {Effect: 26, Value: 3}}}
	arma := antes
	d.desertoFinish(w, spawnNamed(t, w, expMobTemplate(370, 0, 0), "Tauron"), &arma)
	if arma != antes {
		t.Errorf("Martelo Dragão do Tauron comum ganhou add: %+v", arma)
	}
}

// O Taron Tirano volta 4 h depois da morte e nasce 4 h depois do boot.
func TestTaronTiranoRenasceEm4Horas(t *testing.T) {
	boss := geradorSozinho(10_000, 10)
	boss.LeaderName = taronTiranoMolde
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: boss})
	if got := dispatcherQuieto().esperaDoRenascimento(w, 30); got != 4*msPorHora {
		t.Errorf("Taron Tirano volta em %d ms, want 4 h", got)
	}
}

func TestTaronTiranoNasceHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	w := mundoDaLava(t, &agora, taronTiranoMolde)
	dispatcherQuieto().ApplyTaronTiranoBoot(w)
	if doBloco(w, 30) != nil || doBloco(w, 31) == nil {
		t.Fatal("depois do boot: want só o monstro comum de pé")
	}
	quatro := uint32(4 * msPorHora)
	if got := w.SpawnDueRespawns(agora + quatro - 1); len(got) != 0 {
		t.Fatalf("o chefe voltou antes das 4 h: %v", got)
	}
	if got := w.SpawnDueRespawns(agora + quatro); len(got) != 1 || doBloco(w, 30) == nil {
		t.Fatalf("4 h depois do boot voltaram %d, want o chefe de pé", len(got))
	}
}

// O saque do Taron Tirano: as três barras a 80%, no máximo UMA Arma D do Deserto
// a 60% com o add do spot, e cada ovo a 10%.
func TestTaronTiranoSaque(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	const mortes = 1000
	var barras, armas, ovoN, ovoB int
	vistas := map[int16]bool{}
	for range mortes {
		killer.Carry = [len(killer.Carry)]world.Item{}
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(350, 0, 0), taronTiranoMolde))
		nArmas, nBarras := 0, 0
		for _, it := range killer.Carry {
			switch {
			case it.Index == itemBarraPrata10Mi:
				nBarras += itemAmount(it)
			case armasDDoDesertoFisicas[it.Index] || armasDDoDesertoMagicas[it.Index]:
				nArmas++
				vistas[it.Index] = true
				efeito, want := uint8(efDamage), []int{45, 54, 63, 72}
				if armasDDoDesertoMagicas[it.Index] {
					efeito, want = efMagic, []int{20, 24, 28, 32}
				}
				if it.Effects[0] != (world.Effect{Effect: efSanc, Value: 0}) || it.Effects[1].Effect != efeito ||
					!slices.Contains(want, int(it.Effects[1].Value)) || it.Effects[2] != (world.Effect{}) {
					t.Fatalf("arma %d com %+v, want +0 e efeito %d em %v", it.Index, it.Effects, efeito, want)
				}
			case it.Index == itemOvoCavaloLeveN:
				ovoN++
			case it.Index == itemOvoCavaloLeveB:
				ovoB++
			}
		}
		if nBarras != 0 && nBarras != 3 {
			t.Fatalf("%d barras numa morte, want 0 ou 3", nBarras)
		}
		if nArmas > 1 {
			t.Fatalf("%d Armas D numa morte, want no máximo 1", nArmas)
		}
		if nBarras > 0 {
			barras++
		}
		armas += nArmas
	}
	perto := func(nome string, n, pct int) {
		if got := float64(n) * 100 / mortes; math.Abs(got-float64(pct)) > 5 {
			t.Errorf("%s em %.1f%% das mortes, want perto de %d%%", nome, got, pct)
		}
	}
	perto("barras", barras, 80)
	perto("Arma D", armas, 60)
	perto("Ovo Leve N", ovoN, 10)
	perto("Ovo Leve B", ovoB, 10)
	for _, id := range []int16{809, 869} { // Martelo Dragão e Gram, os do pedido
		if !vistas[id] {
			t.Errorf("a Arma D %d nunca saiu em %d mortes", id, mortes)
		}
	}
}

// O arquivo do Taron Tirano: o corpo do Taron Assassino (equipamento 0-12), sem
// o divisor do slot 13, com os números do Ciclope Tirano e vida e dano em dobro,
// e sem saque no template.
func TestTaronTiranoTemplate(t *testing.T) {
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
	b, corpo, tirano := ler(taronTiranoMolde), ler("Taron_Assassino"), ler(ciclopes.Chefe)
	if nome := string(bytes.TrimRight(b.Name[:], "\x00")); nome != taronTiranoMolde {
		t.Errorf("nome %q, want o do arquivo", nome)
	}
	if b.Merchant != 0 || b.CurrentScore.Merchant != 0 {
		t.Errorf("byte de NPC %d/%d, want 0: com ele o chefe nasceria intocável", b.Merchant, b.CurrentScore.Merchant)
	}
	for i := range 13 {
		if b.Equip[i] != corpo.Equip[i] {
			t.Errorf("Equip[%d] = %+v, o Taron Assassino usa %+v", i, b.Equip[i], corpo.Equip[i])
		}
	}
	if b.Equip[13].Index != 0 {
		t.Errorf("Equip[13] = %d, want sem divisor", b.Equip[13].Index)
	}
	for _, s := range []savefmt.Score{b.BaseScore, b.CurrentScore} {
		e := tirano.CurrentScore
		if s.MaxHp != 3_600_000 || s.Hp != 3_600_000 || s.Damage != 3_636 {
			t.Errorf("vida %d/%d e dano %d, want 3.600.000 e 3.636", s.MaxHp, s.Hp, s.Damage)
		}
		if s.MaxHp != 2*e.MaxHp || s.Damage != 2*e.Damage {
			t.Errorf("vida %d e dano %d, want o dobro do Ciclope Tirano (%d, %d)", s.MaxHp, s.Damage, e.MaxHp, e.Damage)
		}
		if s.Level != e.Level || s.AC != e.AC || s.Con != e.Con {
			t.Errorf("score %+v, want nível, defesa e CON do Ciclope Tirano %+v", s, e)
		}
	}
	for i, it := range b.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio: o saque é do código", i, it.Index)
		}
	}
}

// Pela morte: com a Mesa dando a Arma D a 100%, os cinco entregam a arma com o
// add da tropa. Prova a ligação do gancho no sorteio da Mesa, e não só o carimbo.
func TestDesertoArmaDPelaMorte(t *testing.T) {
	for _, c := range []struct {
		mob  string
		arma int16
	}{
		{"Taron_Assassino", 809}, // Martelo Dragão
		{"Taron_Assassino", 854}, // Gungnir
		{"Adamant_Tauron", 936},
		{"Manticora", 870},
		{"Verme_", 885},
		{"Aeon_Tauron", 900},    // Fúria Divina (0186)
		{"Adamant_Tauron", 935}, // Martelo Psíquico (0186)
		{"Manticora", 869},      // Gram (0186)
	} {
		d, w, killer := mobKilledWorld(t)
		d.dropRules = droprule.NewTable([]droprule.Rule{{Mob: c.mob, Item: c.arma, Chance: droprule.MaxChance}})
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(370, 0, 0), c.mob))
		it, ok := carryHas(killer, c.arma)
		if !ok {
			t.Fatalf("%s não derrubou a arma %d a 100%%", c.mob, c.arma)
		}
		efeito, want := uint8(efDamage), []int{27, 36, 45, 54, 63, 72}
		if armasDDoDesertoMagicas[c.arma] {
			efeito, want = efMagic, []int{12, 16, 20, 24, 28, 32}
		}
		if it.Effects[0].Effect != efSanc || it.Effects[1].Effect != efeito || !slices.Contains(want, int(it.Effects[1].Value)) {
			t.Errorf("%s arma %d saiu com %+v, want refino e efeito %d em %v", c.mob, c.arma, it.Effects, efeito, want)
		}
	}
}
