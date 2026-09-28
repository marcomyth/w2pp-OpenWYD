package handler

import (
	"bytes"
	"encoding/binary"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const lavaMigracao = "0148_lava_so_na_sala.up.sql"

// mundoDaLava tem o bloco 30 com um mini chefe e o 31 com um Golem comum, e o
// relógio na mão do teste.
func mundoDaLava(t *testing.T, agora *uint32, chefe string) *world.World {
	t.Helper()
	bloco := func(nome string, x int16) *world.Generator {
		g := geradorSozinho(10_000, x)
		g.LeaderName = nome
		g.LeaderTmpl = moldeDeMonstro(nome, 10_000, 0)
		// Vida no MaxHp (@108) e no Hp (@116): o relógio não olha monstro morto.
		binary.LittleEndian.PutUint32(g.LeaderTmpl[108:], 1_000)
		binary.LittleEndian.PutUint32(g.LeaderTmpl[116:], 1_000)
		return g
	}
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return *agora }},
		slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: bloco(chefe, 10), 31: bloco("Golem_de_Pedra", 20)})
	for _, idx := range []int{30, 31} {
		if len(w.GenerateMob(idx)) != 1 {
			t.Fatalf("o boot não levantou o bloco %d", idx)
		}
	}
	return w
}

// doBloco devolve o monstro de pé do bloco idx, ou nil.
func doBloco(w *world.World, idx int) *world.Entity {
	for id := world.MaxUser; id < world.MaxMob; id++ {
		if e := w.Entity(id); e != nil && int(e.GenIndex) == idx {
			return e
		}
	}
	return nil
}

// passadaDaLava roda o relógio de sumir numa passada do relógio de minuto.
func passadaDaLava(d *Dispatcher, w *world.World) {
	d.tickCount += minTimerTicks - d.tickCount%minTimerTicks
	d.tickChefesDaLava(w)
}

// Os dois blocos voltam 2 h depois da morte; um chefe sozinho qualquer segue nas
// horas do painel.
func TestChefesDaLavaRenascemEm2Horas(t *testing.T) {
	duas := uint32(2 * msPorHora)
	for _, nome := range []string{bossGolemTemplate, bossAnfNinjaTemplate} {
		boss := geradorSozinho(10_000, 10)
		boss.LeaderName = nome
		w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
		w.RegisterGenerators([]*world.Generator{30: geradorSozinho(2_990_849, 12), 31: boss})
		d := dispatcherQuieto()
		if got := d.esperaDoRenascimento(w, 31); got != duas {
			t.Errorf("%s volta em %d ms, want %d", nome, got, duas)
		}
		if got := d.esperaDoRenascimento(w, 30); got == duas {
			t.Errorf("um chefe sozinho qualquer também ganhou as 2 h (%s)", nome)
		}
	}
}

// Depois do boot os mini chefes só aparecem 2 h depois, e o Golem comum fica de pé.
func TestChefesDaLavaNascemHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	w := mundoDaLava(t, &agora, bossAnfNinjaTemplate)
	dispatcherQuieto().ApplyChefesDaLavaBoot(w)
	if doBloco(w, 30) != nil || doBloco(w, 31) == nil {
		t.Fatal("depois do boot: want só o Golem comum de pé")
	}
	duas := uint32(lavaChefeHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + duas - 1); len(got) != 0 {
		t.Fatalf("o chefe voltou antes das 2 h: %v", got)
	}
	agora += duas
	if got := w.SpawnDueRespawns(agora); len(got) != 1 || doBloco(w, 30) == nil {
		t.Fatalf("2 h depois do boot voltaram %d, want o chefe de pé", len(got))
	}
}

// Sem luta, o chefe some depois de 30 min e volta 2 h depois de ter nascido; o
// monstro comum do lado não é tocado.
func TestChefeDaLavaSomeSemLuta(t *testing.T) {
	var agora uint32 = 1_000
	w := mundoDaLava(t, &agora, bossGolemTemplate)
	d := dispatcherQuieto()
	passadaDaLava(d, w) // o relógio vê o chefe nascer agora
	nasceu := agora

	agora = nasceu + lavaChefeSemLuta - 1
	passadaDaLava(d, w)
	if doBloco(w, 30) == nil {
		t.Fatal("o chefe sumiu antes dos 30 min")
	}
	agora = nasceu + lavaChefeSemLuta
	passadaDaLava(d, w)
	if doBloco(w, 30) != nil {
		t.Fatal("30 min sem luta e o chefe continua de pé")
	}
	if doBloco(w, 31) == nil {
		t.Fatal("o Golem comum sumiu junto")
	}
	volta := nasceu + lavaChefeHoras*msPorHora
	if got := w.SpawnDueRespawns(volta - 1); len(got) != 0 {
		t.Fatalf("voltou antes das 2 h desde que nasceu: %v", got)
	}
	if got := w.SpawnDueRespawns(volta); len(got) != 1 || doBloco(w, 30) == nil {
		t.Fatalf("2 h depois de nascer voltaram %d, want o chefe de pé", len(got))
	}
}

// Quem luta segura o chefe: ferido ou com alvo, o relógio recomeça; 30 min depois
// da última luta ele some, e não volta antes do mínimo mesmo com o ciclo vencido.
func TestChefeDaLavaFicaEnquantoHaLuta(t *testing.T) {
	var agora uint32 = 1_000
	w := mundoDaLava(t, &agora, bossAnfNinjaTemplate)
	d := dispatcherQuieto()
	passadaDaLava(d, w)
	boss := doBloco(w, 30)

	// Luta de 2 h e meia: ferido, e o relógio vê a cada 20 min.
	boss.HP = boss.MaxHP - 1
	for range 8 {
		agora += 20 * msPorMinuto
		passadaDaLava(d, w)
		if doBloco(w, 30) != boss {
			t.Fatal("o chefe sumiu no meio da luta")
		}
	}
	// Quem lutava foi embora; com alvo também conta, e depois nem isso.
	boss.HP = boss.MaxHP
	boss.Target = 1
	agora += lavaChefeSemLuta
	passadaDaLava(d, w)
	if doBloco(w, 30) != boss {
		t.Fatal("o chefe com alvo sumiu")
	}
	boss.Target = 0
	fim := agora + lavaChefeSemLuta
	agora = fim
	passadaDaLava(d, w)
	if doBloco(w, 30) != nil {
		t.Fatal("30 min sem luta e o chefe continua de pé")
	}
	if got := w.SpawnDueRespawns(fim + lavaChefeVoltaMinima - 1); len(got) != 0 {
		t.Fatalf("o ciclo tinha vencido e ele voltou antes do mínimo: %v", got)
	}
	if got := w.SpawnDueRespawns(fim + lavaChefeVoltaMinima); len(got) != 1 {
		t.Fatalf("voltaram %d depois do mínimo, want o chefe", len(got))
	}
}

// Um chefe novo no bloco não herda o relógio do anterior, mesmo que caia no mesmo
// id.
func TestChefeDaLavaNovoNaoHerdaRelogio(t *testing.T) {
	var agora uint32 = 1_000
	w := mundoDaLava(t, &agora, bossGolemTemplate)
	d := dispatcherQuieto()
	passadaDaLava(d, w)
	velho := doBloco(w, 30)
	id := velho.ID

	// Morre e renasce 2 h depois, na fila da morte.
	w.DespawnMob(id, 1)
	agora += lavaChefeHoras * msPorHora
	w.SpawnDueRespawns(agora)
	novo := doBloco(w, 30)
	if novo == nil || novo == velho {
		t.Fatal("o chefe não renasceu como um monstro novo")
	}
	passadaDaLava(d, w)
	if doBloco(w, 30) != novo {
		t.Fatal("o chefe recém-nascido sumiu com o relógio do anterior")
	}
}

// Os quatro prêmios pedidos saem a 25% cada; o de 40 âmagos divide os seus 25%
// entre Urso, Lobo e Dragão Menor.
func TestChefeDaLavaSorteio(t *testing.T) {
	vezes := map[string]int{}
	for v := range 32768 {
		vezes[sorteiaPremioDeChefe(lavaChefePremios, v%lavaChefeBase).nome]++
	}
	soma := 0
	for _, p := range lavaChefePremios {
		soma += p.peso
	}
	if soma != lavaChefeBase {
		t.Fatalf("pesos somam %d, want %d", soma, lavaChefeBase)
	}
	taxa := func(nomes ...string) float64 {
		n := 0
		for _, nome := range nomes {
			n += vezes[nome]
		}
		return float64(n) / 32768
	}
	for _, grupo := range [][]string{
		{"Barra de Prata (10Mi)"}, {"Pacote de Sem Sela"}, {"Pacote de Dente de Sabre"},
		{"Pacote de Urso", "Pacote de Lobo", "Pacote de Dragão Menor"},
	} {
		if got := taxa(grupo...); got < 0.249 || got > 0.251 {
			t.Errorf("%v sai a %.3f%%, want 25%%", grupo, got*100)
		}
	}
}

// Cada morte entrega exatamente um prêmio, com a quantidade pedida. A chance de
// cada um é o TestChefeDaLavaSorteio: aqui todo mundo novo começa na mesma semente.
func TestChefeDaLavaSoltaUmPremio(t *testing.T) {
	want := map[int16]int{itemBarraPrata10Mi: 1, 2396: 10, 2401: 10, 2395: 30, 2394: 40, 2392: 40, 2393: 40}
	for i := range 20 {
		nome := bossGolemTemplate
		if i%2 == 1 {
			nome = bossAnfNinjaTemplate
		}
		d, w, killer := mobKilledWorld(t)
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(250, 0, 0), nome))
		achou := 0
		for _, it := range killer.Carry {
			q, ok := want[it.Index]
			if !ok {
				continue
			}
			achou++
			if got := itemAmount(it); got != q {
				t.Errorf("item %d com %d unidades, want %d", it.Index, got, q)
			}
		}
		if achou != 1 {
			t.Fatalf("%s: %d prêmios na bolsa, want 1", nome, achou)
		}
	}
}

// Os templates: o corpo do monstro comum, maior; 300 mil de vida real pelo
// divisor ÷5 (eram 3 milhões, "vida infinita"); mais forte que o comum; XP no teto da Dungeon; Carry vazio.
func TestChefesDaLavaTemplates(t *testing.T) {
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
	for boss, comum := range map[string]string{bossGolemTemplate: "Golem_de_Pedra", bossAnfNinjaTemplate: "Anf_Ninja"} {
		b, c := ler(boss), ler(comum)
		if nome := string(bytes.TrimRight(b.Name[:], string([]byte{0}))); nome != boss {
			t.Errorf("%s: nome %q, want o do arquivo", boss, nome)
		}
		if b.Equip[0].Index != c.Equip[0].Index {
			t.Errorf("%s: corpo %d, o %s é %d", boss, b.Equip[0].Index, comum, c.Equip[0].Index)
		}
		if b.CurrentScore.Con <= c.CurrentScore.Con {
			t.Errorf("%s: CON %d, want maior que a do %s (%d), que é o tamanho", boss, b.CurrentScore.Con, comum, c.CurrentScore.Con)
		}
		if b.Equip[13].Index != itemDivisorDobro || b.Equip[13].Effects[0].Value != 5 {
			t.Errorf("%s: slot 13 = %d/%d, want o divisor %d a 5", boss, b.Equip[13].Index, b.Equip[13].Effects[0].Value, itemDivisorDobro)
		}
		if vidaReal := int64(b.CurrentScore.MaxHp) * 5; vidaReal != 300_000 {
			t.Errorf("%s: vida real %d, want 300 mil", boss, vidaReal)
		}
		if b.CurrentScore.Damage <= c.CurrentScore.Damage || b.CurrentScore.AC <= c.CurrentScore.AC {
			t.Errorf("%s: dano %d e defesa %d, want acima do %s (%d e %d)", boss,
				b.CurrentScore.Damage, b.CurrentScore.AC, comum, c.CurrentScore.Damage, c.CurrentScore.AC)
		}
		if b.Exp > 10_000 {
			t.Errorf("%s: XP %d, acima do teto de 10.000 da Dungeon (0091)", boss, b.Exp)
		}
		if b.BaseScore.MaxHp != b.CurrentScore.MaxHp || b.BaseScore.Damage != b.CurrentScore.Damage {
			t.Errorf("%s: BaseScore e CurrentScore diferentes", boss)
		}
		for i, it := range b.Carry {
			if it.Index != 0 {
				t.Errorf("%s: Carry[%d] = %d, want vazio", boss, i, it.Index)
			}
		}
	}
}

// A 0148 só cita templates e itens que existem. Nas cópias da sala o Sem Sela é o
// mais difícil e tudo o que o template soltava vai a 0%; o Golem de Pedra e o Anf
// Ninja do resto do andar voltam ao template, com o Sem Sela da 0091 (33 e 29).
func TestLavaMigracaoSaqueDeCampo(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, lavaMigracao)
	for mob, porItem := range linhas {
		if _, _, err := npctemplate.Load(root, mob); err != nil {
			t.Errorf("%s: %v", mob, err)
		}
		for item := range porItem {
			if _, ok := items.Get(int(item)); !ok {
				t.Errorf("%s: item %d não existe no ItemList", mob, item)
			}
		}
	}
	pedidos := []int16{2392, 2394, 2395, 2441, 4019, 4018, 4026}
	semSela := []int16{2396, 2401}
	for _, mob := range []string{"Anf_Ninja", "Golem_de_Pedra"} {
		if got := linhas[mob]; len(got) != 2 || got[2396] != 33 || got[2401] != 29 {
			t.Errorf("%s fora da sala: %v, want só o Sem Sela da 0091 (2396 a 33, 2401 a 29)", mob, got)
		}
	}
	for _, mob := range []string{anfDaSalaTemplate, golemDaSalaTemplate} {
		porItem := linhas[mob]
		menor := int32(1 << 30)
		for _, item := range pedidos {
			c, ok := porItem[item]
			if !ok || c <= 0 {
				t.Errorf("%s: item %d a %d (presente %v), want > 0", mob, item, c, ok)
			}
			menor = min(menor, c)
		}
		for _, item := range semSela {
			if c := porItem[item]; c <= 0 || c >= menor {
				t.Errorf("%s: Sem Sela %d a %d, want > 0 e abaixo de todo o resto (%d)", mob, item, c, menor)
			}
		}
		b, _, err := npctemplate.Load(root, mob)
		if err != nil {
			t.Fatal(err)
		}
		m, _, err := savefmt.DecodeMobAny(b)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range m.Carry {
			if it.Index <= 390 || isPedido(it.Index, pedidos, semSela) {
				continue
			}
			if c, ok := porItem[it.Index]; !ok || c != 0 {
				t.Errorf("%s: o item %d do template ficou a %d (presente %v), want 0", mob, it.Index, c, ok)
			}
		}
	}
}

func isPedido(item int16, listas ...[]int16) bool {
	for _, l := range listas {
		for _, x := range l {
			if x == item {
				return true
			}
		}
	}
	return false
}

// As cópias da sala são o monstro comum byte a byte: só o nome do arquivo muda,
// para a Mesa, e o jogador vê o mesmo Golem de Pedra e o mesmo Anf Ninja.
func TestSalaDaLavaCopiasDosTemplates(t *testing.T) {
	root := releaseDir(t)
	for copia, original := range map[string]string{golemDaSalaTemplate: "Golem_de_Pedra", anfDaSalaTemplate: "Anf_Ninja"} {
		a, _, err := npctemplate.Load(root, copia)
		if err != nil {
			t.Fatalf("%s: %v", copia, err)
		}
		b, _, err := npctemplate.Load(root, original)
		if err != nil {
			t.Fatalf("%s: %v", original, err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s difere de %s", copia, original)
		}
	}
}

// Os bichos da sala voltam em 10 s; o mesmo Golem fora dela segue os 15 s da fila.
func TestSalaDaLavaRenasceEm10Segundos(t *testing.T) {
	sala := geradorSozinho(10_000, 10)
	sala.LeaderName = golemDaSalaTemplate
	fora := geradorSozinho(10_000, 12)
	fora.LeaderName = "Golem_de_Pedra"
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: sala, 31: fora})
	d := dispatcherQuieto()
	if got := d.esperaDoRenascimento(w, 30); got != lavaSalaRenasce || got > 10_000 {
		t.Errorf("bicho da sala volta em %d ms, want %d", got, lavaSalaRenasce)
	}
	if got := d.esperaDoRenascimento(w, 31); got != world.DefaultRespawnDelay {
		t.Errorf("Golem fora da sala volta em %d ms, want os %d da fila", got, world.DefaultRespawnDelay)
	}
}
