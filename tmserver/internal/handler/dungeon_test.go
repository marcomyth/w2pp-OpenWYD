package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const dungeonMigracao = "0140_esqueleto_e_boss_dragao_lich.up.sql"

// O bloco do Boss Dragão Lich volta 4 h depois da morte; um chefe sozinho
// qualquer segue nas horas do painel.
func TestBossDragaoLichRenasceEm4Horas(t *testing.T) {
	boss := geradorSozinho(10_000, 10)
	boss.LeaderName = bossDragaoLichTemplate
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: geradorSozinho(2_990_849, 12), 31: boss})
	d := dispatcherQuieto()
	quatro := uint32(4 * msPorHora)
	if got := d.esperaDoRenascimento(w, 31); got != quatro {
		t.Errorf("Boss Dragão Lich volta em %d ms, want %d", got, quatro)
	}
	if got := d.esperaDoRenascimento(w, 30); got == quatro {
		t.Error("um chefe sozinho qualquer também ganhou as 4 h")
	}
}

// Depois do boot o Boss Dragão Lich só aparece 4 h depois, pela fila da morte, e
// o resto do mundo fica de pé.
func TestBossDragaoLichNasceHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	bloco := func(nome string) *world.Generator {
		g := geradorSozinho(10_000, 10)
		g.LeaderName = nome
		g.LeaderTmpl = moldeDeMonstro(nome, 10_000, 0)
		return g
	}
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return agora }},
		slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: bloco(bossDragaoLichTemplate), 31: bloco("Dragao_Lich")})
	for _, idx := range []int{30, 31} {
		if len(w.GenerateMob(idx)) != 1 {
			t.Fatalf("o boot não levantou o bloco %d", idx)
		}
	}
	vivo := func(idx int) bool {
		for id := world.MaxUser; id < world.MaxMob; id++ {
			if e := w.Entity(id); e != nil && int(e.GenIndex) == idx {
				return true
			}
		}
		return false
	}

	dispatcherQuieto().ApplyBossDragaoLichBoot(w)

	if vivo(30) || !vivo(31) {
		t.Fatalf("depois do boot: chefe %v, Dragão comum %v; want só o comum de pé", vivo(30), vivo(31))
	}
	quatro := uint32(bossDragaoLichHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + quatro - 1); len(got) != 0 {
		t.Fatalf("o chefe voltou antes das 4 h: %v", got)
	}
	agora += quatro
	if got := w.SpawnDueRespawns(agora); len(got) != 1 || !vivo(30) {
		t.Fatalf("4 h depois do boot voltaram %d, want o chefe de pé", len(got))
	}
}

// Os quatro prêmios do código saem a 25% cada, e a pedra não está entre eles: é
// a regra da Mesa.
func TestBossDragaoLichSorteio(t *testing.T) {
	vezes := map[string]int{}
	for v := range 32768 {
		vezes[sorteiaPremioDeChefe(bossDragaoLichPremios, v%bossDragaoLichBase).nome]++
	}
	soma := 0
	for _, p := range bossDragaoLichPremios {
		soma += p.peso
		if p.itemN == itemPedraDeDragaoLich || p.itemN == itemPedraDeManticora {
			t.Errorf("%s no sorteio do código: a pedra do chefe é a regra da Mesa", p.nome)
		}
		if taxa := float64(vezes[p.nome]) / 32768; taxa < 0.249 || taxa > 0.251 {
			t.Errorf("%s sai a %.3f%%, want 25%%", p.nome, taxa*100)
		}
	}
	if soma != bossDragaoLichBase || len(bossDragaoLichPremios) != 4 {
		t.Errorf("%d prêmios somando %d, want 4 somando %d", len(bossDragaoLichPremios), soma, bossDragaoLichBase)
	}
}

// Cada morte entrega exatamente um prêmio, com a quantidade do pacote.
func TestBossDragaoLichSoltaUmPremio(t *testing.T) {
	quantos := map[int16]int{}
	for _, p := range bossDragaoLichPremios {
		quantos[p.itemN] = p.quantidade
		if p.itemB != 0 {
			quantos[p.itemB] = p.quantidade
		}
	}
	for range 20 {
		d, w, killer := mobKilledWorld(t)
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(399, 0, 0), bossDragaoLichTemplate))
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
			t.Fatalf("%d prêmios do Boss Dragão Lich na bolsa, want 1", achou)
		}
	}
}

// O template: o corpo do Dragão Lich, maior; o nível do Cav. Lugefer; a vida
// real e o dano do Boss Mantícora; a XP no teto da Dungeon (0091); nada no Carry.
// E o Dragão Lich comum com +40% de vida e dano (26.000 e 1.100 antes).
func TestBossDragaoLichTemplate(t *testing.T) {
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
	boss, drag, mant, cav := ler(bossDragaoLichTemplate), ler("Dragao_Lich"), ler(bossManticoraTemplate), ler("Cav._Lugefer")
	vidaReal := func(m savefmt.Mob) int64 {
		v := int64(m.Equip[13].Effects[0].Value)
		if v == 0 {
			v = 2
		}
		switch m.Equip[13].Index {
		case 786:
			return int64(m.CurrentScore.MaxHp) * v
		case 1936:
			return int64(m.CurrentScore.MaxHp) * v * 10
		case 1937:
			return int64(m.CurrentScore.MaxHp) * v * 1000
		}
		return int64(m.CurrentScore.MaxHp)
	}
	if boss.Equip[0].Index != drag.Equip[0].Index {
		t.Errorf("corpo %d, o Dragão Lich é %d", boss.Equip[0].Index, drag.Equip[0].Index)
	}
	if boss.CurrentScore.Con <= drag.CurrentScore.Con {
		t.Errorf("CON %d: want maior que a do Dragão Lich (%d), que é o tamanho", boss.CurrentScore.Con, drag.CurrentScore.Con)
	}
	if boss.CurrentScore.Level != cav.CurrentScore.Level {
		t.Errorf("nível %d, o Cav. Lugefer é %d", boss.CurrentScore.Level, cav.CurrentScore.Level)
	}
	if vidaReal(boss) != vidaReal(mant) || boss.CurrentScore.Damage != mant.CurrentScore.Damage {
		t.Errorf("vida real %d e dano %d, want os do Boss Mantícora (%d e %d)",
			vidaReal(boss), boss.CurrentScore.Damage, vidaReal(mant), mant.CurrentScore.Damage)
	}
	if boss.Exp > 10_000 {
		t.Errorf("XP %d, acima do teto de 10.000 da Dungeon (0091)", boss.Exp)
	}
	for i, it := range boss.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio", i, it.Index)
		}
	}
	for _, m := range []savefmt.Mob{boss, drag} {
		if m.BaseScore.MaxHp != m.CurrentScore.MaxHp || m.BaseScore.Damage != m.CurrentScore.Damage {
			t.Errorf("BaseScore e CurrentScore diferentes: vida %d/%d, dano %d/%d",
				m.BaseScore.MaxHp, m.CurrentScore.MaxHp, m.BaseScore.Damage, m.CurrentScore.Damage)
		}
	}
	if drag.CurrentScore.MaxHp != 36_400 || drag.CurrentScore.Hp != 36_400 || drag.CurrentScore.Damage != 1_540 {
		t.Errorf("Dragão Lich com vida %d/%d e dano %d, want 36.400 e 1.540 (+40%%)",
			drag.CurrentScore.Hp, drag.CurrentScore.MaxHp, drag.CurrentScore.Damage)
	}
}

// A 0140 só cita templates e itens que existem, e a pedra do chefe fica na
// régua das outras Pedras Arch.
func TestDungeonMigracaoApontaParaOQueExiste(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, dungeonMigracao)
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
	if got := linhas[bossDragaoLichTemplate][itemPedraDeDragaoLich]; got != 81 {
		t.Errorf("Pedra de Dragão Lich no chefe a %d, want 81 (0,99%%, a régua da 0136)", got)
	}
	for _, mob := range []string{"Guer_Caveira", "Cav.Caveira", "SkeltonWarrior"} {
		if got, ok := linhas[mob][1753]; !ok || got != 0 {
			t.Errorf("Pedra do Esqueleto em %s: %d (presente %v), want 0", mob, got, ok)
		}
	}
	if got, ok := linhas["Dragao_Lich"][itemPedraDeDragaoLich]; !ok || got != 0 {
		t.Errorf("Pedra de Dragão Lich no Dragão comum: %d (presente %v), want 0", got, ok)
	}
}

// Um bloco das salas do 1º andar (0143) com grupo cheio levanta todos os bichos
// numa chamada só — que é o que o boot faz uma vez por bloco. Com o grupo de 0
// seguidores de antes, só um nasceria.
func TestDungeonSalasBlocoNasceCheio(t *testing.T) {
	w := world.New(world.Config{GridDim: 4096}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	g := geradorSozinho(10_000, 200)
	g.LeaderName = "Urso_Zumbi"
	g.LeaderTmpl = moldeDeMonstro("Urso_Zumbi", 10_000, 0)
	g.FollowerTmpl = g.LeaderTmpl
	g.MaxNumMob, g.MinGroup, g.MaxGroup = 5, 4, 4
	g.SegRange[0] = 2
	w.RegisterGenerators([]*world.Generator{30: g})
	if got := len(w.GenerateMob(30)); got != 5 {
		t.Fatalf("o bloco levantou %d, want 5 numa chamada", got)
	}
	if got := len(w.GenerateMob(30)); got != 0 {
		t.Errorf("o bloco cheio levantou mais %d, want 0", got)
	}
}

// A 0143 soma os três âmagos e o Resto de Ori nos três monstros das salas, e só
// cita templates e itens que existem.
func TestDungeonSalasMigracao(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, "0143_dungeon_salas_amagos_e_restos.up.sql")
	want := map[int16]int32{2392: 50, 2393: 50, 2394: 50, 419: 100}
	for _, mob := range []string{"Caveira", "Urso_Zumbi", "Arq_Caveira"} {
		if _, _, err := npctemplate.Load(root, mob); err != nil {
			t.Errorf("%s: %v", mob, err)
		}
		for item, chance := range want {
			if _, ok := items.Get(int(item)); !ok {
				t.Errorf("item %d não existe no ItemList", item)
			}
			if got := linhas[mob][item]; got != chance {
				t.Errorf("%s item %d a %d, want %d", mob, item, got, chance)
			}
		}
		if len(linhas[mob]) != len(want) {
			t.Errorf("%s com %d regras, want %d: a 0143 só soma, não tira", mob, len(linhas[mob]), len(want))
		}
	}
}

const hidraMigracao = "0149_boss_hidra_dourada.up.sql"

// O bloco do Boss Hidra Dourada volta 4 h depois da morte, e o da escolta segue
// nos 15 s do resto do mundo.
func TestBossHidraDouradaRenasceEm4Horas(t *testing.T) {
	boss := geradorSozinho(10_000, 10)
	boss.LeaderName = bossHidraDouradaTemplate
	escolta := geradorSozinho(10_000, 10)
	escolta.LeaderName = "Guer_Caveira_Escolta"
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: escolta, 31: boss})
	d := dispatcherQuieto()
	quatro := uint32(bossHidraDouradaHoras * msPorHora)
	if quatro != 4*msPorHora {
		t.Fatalf("bossHidraDouradaHoras = %d, want 4", bossHidraDouradaHoras)
	}
	if got := d.esperaDoRenascimento(w, 31); got != quatro {
		t.Errorf("Boss Hidra Dourada volta em %d ms, want %d", got, quatro)
	}
	if got := d.esperaDoRenascimento(w, 30); got != world.DefaultRespawnDelay {
		t.Errorf("a escolta volta em %d ms, want %d", got, world.DefaultRespawnDelay)
	}
}

// Depois do boot o Boss Hidra Dourada só aparece 4 h depois, e a escolta fica de
// pé.
func TestBossHidraDouradaNasceHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	bloco := func(nome string) *world.Generator {
		g := geradorSozinho(10_000, 10)
		g.LeaderName = nome
		g.LeaderTmpl = moldeDeMonstro(nome, 10_000, 0)
		return g
	}
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return agora }},
		slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: bloco(bossHidraDouradaTemplate), 31: bloco("Guer_Caveira_Escolta")})
	for _, idx := range []int{30, 31} {
		if len(w.GenerateMob(idx)) != 1 {
			t.Fatalf("o boot não levantou o bloco %d", idx)
		}
	}
	vivo := func(idx int) bool {
		for id := world.MaxUser; id < world.MaxMob; id++ {
			if e := w.Entity(id); e != nil && int(e.GenIndex) == idx {
				return true
			}
		}
		return false
	}

	dispatcherQuieto().ApplyBossHidraDouradaBoot(w)

	if vivo(30) || !vivo(31) {
		t.Fatalf("depois do boot: chefe %v, escolta %v; want só a escolta de pé", vivo(30), vivo(31))
	}
	quatro := uint32(bossHidraDouradaHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + quatro - 1); len(got) != 0 {
		t.Fatalf("o chefe voltou antes das 4 h: %v", got)
	}
	agora += quatro
	if got := w.SpawnDueRespawns(agora); len(got) != 1 || !vivo(30) {
		t.Fatalf("4 h depois do boot voltaram %d, want o chefe de pé", len(got))
	}
}

// Cada morte entrega exatamente um dos quatro prêmios do Boss Mantícora sem a
// pedra, com a quantidade do pacote; a pedra é a Mesa.
func TestBossHidraDouradaSoltaUmPremio(t *testing.T) {
	quantos := map[int16]int{}
	for _, p := range bossDragaoLichPremios {
		quantos[p.itemN] = p.quantidade
		if p.itemB != 0 {
			quantos[p.itemB] = p.quantidade
		}
	}
	if _, ok := quantos[itemPedraDoEsqueleto]; ok {
		t.Fatal("a Pedra do Esqueleto está no sorteio do código: ela é a regra da Mesa")
	}
	for range 20 {
		d, w, killer := mobKilledWorld(t)
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(399, 0, 0), bossHidraDouradaTemplate))
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
			t.Fatalf("%d prêmios do Boss Hidra Dourada na bolsa, want 1", achou)
		}
	}
}

// O chefe: o corpo da Hidra Dourada, maior, com o nível, a defesa, a vida real
// (divisor no slot 13) e o dano do Boss Dragão Lich — que são os do Boss
// Mantícora (TestBossDragaoLichTemplate) —, a XP no teto da Dungeon e nada no
// Carry. A escolta: o Guer Caveira com 6x a vida e o dano, e o resto igual.
func TestBossHidraDouradaTemplates(t *testing.T) {
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
	boss, hidra, molde := ler(bossHidraDouradaTemplate), ler("Hidra_Dourada_"), ler(bossDragaoLichTemplate)
	if boss.Equip[0] != hidra.Equip[0] {
		t.Errorf("corpo %v, a Hidra Dourada é %v", boss.Equip[0], hidra.Equip[0])
	}
	if boss.CurrentScore.Con <= hidra.CurrentScore.Con {
		t.Errorf("CON %d: want maior que a da Hidra (%d), que é o tamanho", boss.CurrentScore.Con, hidra.CurrentScore.Con)
	}
	b, m := boss.CurrentScore, molde.CurrentScore
	if b.Level != m.Level || b.AC != m.AC || b.Damage != m.Damage || b.MaxHp != m.MaxHp || b.Hp != m.Hp || boss.Equip[13] != molde.Equip[13] {
		t.Errorf("nível %d, AC %d, dano %d, vida %d/%d, slot 13 %v; want os do Boss Dragão Lich (%d, %d, %d, %d/%d, %v)",
			b.Level, b.AC, b.Damage, b.Hp, b.MaxHp, boss.Equip[13], m.Level, m.AC, m.Damage, m.Hp, m.MaxHp, molde.Equip[13])
	}
	if boss.Exp > 10_000 {
		t.Errorf("XP %d, acima do teto de 10.000 da Dungeon (0091)", boss.Exp)
	}
	for i, it := range boss.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio", i, it.Index)
		}
	}

	esc, guer := ler("Guer_Caveira_Escolta"), ler("Guer_Caveira")
	for _, par := range [][2]savefmt.Score{{esc.CurrentScore, guer.CurrentScore}, {esc.BaseScore, guer.BaseScore}} {
		e, g := par[0], par[1]
		if e.MaxHp != 6*g.MaxHp || e.Hp != 6*g.Hp || e.Damage != 6*g.Damage {
			t.Errorf("escolta com vida %d/%d e dano %d, want 6x o Guer Caveira (%d e %d)", e.Hp, e.MaxHp, e.Damage, 6*g.MaxHp, 6*g.Damage)
		}
		if e.Level != g.Level || e.AC != g.AC || e.Con != g.Con {
			t.Errorf("escolta com nível %d, AC %d e CON %d, want os do Guer Caveira (%d, %d, %d)", e.Level, e.AC, e.Con, g.Level, g.AC, g.Con)
		}
	}
	if esc.Name != guer.Name || esc.Equip != guer.Equip || esc.Carry != guer.Carry {
		t.Error("a escolta mudou de nome, corpo ou Carry: só a vida e o dano deviam mudar")
	}
}

// A 0149 põe a Pedra do Esqueleto no chefe a 0,99%, a zera na escolta, copia a
// Mesa do Guer Caveira para a escolta, e só cita templates e itens que existem.
func TestBossHidraDouradaMigracao(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, hidraMigracao)
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
	if got := linhas[bossHidraDouradaTemplate][itemPedraDoEsqueleto]; got != 81 {
		t.Errorf("Pedra do Esqueleto no chefe a %d, want 81 (0,99%%, a régua da 0136)", got)
	}
	if got, ok := linhas["Guer_Caveira_Escolta"][itemPedraDoEsqueleto]; !ok || got != 0 {
		t.Errorf("Pedra do Esqueleto na escolta: %d (presente %v), want 0", got, ok)
	}
	sql, err := migrations.FS.ReadFile(hidraMigracao)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`SELECT 'Guer_Caveira_Escolta', item, chance FROM drop_rule WHERE mob = 'Guer_Caveira'`).Match(sql) {
		t.Error("a 0149 não copia a Mesa do Guer_Caveira para a escolta")
	}
}
