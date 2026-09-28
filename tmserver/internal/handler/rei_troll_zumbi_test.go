package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/route"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const reiTrollZumbiMigracao = "0147_troll_zumbi_e_rei.up.sql"

// O bloco do Rei Troll Zumbi volta 1 h depois da morte; um chefe sozinho
// qualquer segue nas horas do painel.
func TestReiTrollZumbiRenasceEmUmaHora(t *testing.T) {
	rei := geradorSozinho(10_000, 10)
	rei.LeaderName = reiTrollZumbiTemplate
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: geradorSozinho(2_990_849, 12), 31: rei})
	d := dispatcherQuieto()
	uma := uint32(msPorHora)
	if got := d.esperaDoRenascimento(w, 31); got != uma {
		t.Errorf("Rei Troll Zumbi volta em %d ms, want %d", got, uma)
	}
	if got := d.esperaDoRenascimento(w, 30); got == uma {
		t.Error("um chefe sozinho qualquer também ganhou 1 h")
	}
}

// Depois do boot o Rei Troll Zumbi só aparece 1 h depois, pela fila da morte, e
// o resto do mundo fica de pé.
func TestReiTrollZumbiNasceUmaHoraDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	bloco := func(nome string) *world.Generator {
		g := geradorSozinho(10_000, 10)
		g.LeaderName = nome
		g.LeaderTmpl = moldeDeMonstro(nome, 10_000, 0)
		return g
	}
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return agora }},
		slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: bloco(reiTrollZumbiTemplate), 31: bloco("Troll_Zumbi")})
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

	dispatcherQuieto().ApplyReiTrollZumbiBoot(w)

	if vivo(30) || !vivo(31) {
		t.Fatalf("depois do boot: Rei %v, Troll comum %v; want só o comum de pé", vivo(30), vivo(31))
	}
	uma := uint32(reiTrollZumbiHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + uma - 1); len(got) != 0 {
		t.Fatalf("o Rei voltou antes de 1 h: %v", got)
	}
	agora += uma
	if got := w.SpawnDueRespawns(agora); len(got) != 1 || !vivo(30) {
		t.Fatalf("1 h depois do boot voltaram %d, want o Rei de pé", len(got))
	}
}

// pacoteNaBolsa soma, por item, as unidades do pacote do Rei que estão na bolsa.
func pacoteNaBolsa(killer *world.Entity) map[int16]int {
	no := map[int16]bool{}
	for _, p := range reiTrollZumbiPacote {
		no[p.item] = true
	}
	got := map[int16]int{}
	for _, it := range killer.Carry {
		if no[it.Index] {
			got[it.Index] += itemAmount(it)
		}
	}
	return got
}

// Cada morte entrega o pacote inteiro, sem sorteio: a Moeda de 5 milhões, 5
// Poeiras de Oriharucon, 3 de Lactolerium e 20 Âmagos de Dente de Sabre.
func TestReiTrollZumbiSoltaOPacote(t *testing.T) {
	want := map[int16]int{itemMoeda5KK: 1, 412: 5, 413: 3, 2395: 20}
	for range 10 {
		d, w, killer := mobKilledWorld(t)
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(115, 0, 0), reiTrollZumbiTemplate))
		got := pacoteNaBolsa(killer)
		if len(got) != len(want) {
			t.Fatalf("pacote na bolsa = %v, want %v", got, want)
		}
		for item, n := range want {
			if got[item] != n {
				t.Errorf("item %d: %d unidades, want %d", item, got[item], n)
			}
		}
	}
	// Um Troll Zumbi comum não solta o pacote.
	d, w, killer := mobKilledWorld(t)
	d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(102, 0, 0), "Troll_Zumbi"))
	if got := pacoteNaBolsa(killer); len(got) != 0 {
		t.Errorf("o Troll Zumbi comum soltou %v do pacote do Rei", got)
	}
}

// Uma regra da Mesa para um item do pacote vale mais que o código: o "*" a 0%
// tira o item do mundo, e o resto do pacote sai igual.
func TestReiTrollZumbiRespeitaAMesa(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	d.dropRules = droprule.NewTable([]droprule.Rule{{Mob: droprule.AllMobs, Item: 2395, Chance: 0}})
	d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(115, 0, 0), reiTrollZumbiTemplate))
	got := pacoteNaBolsa(killer)
	if got[2395] != 0 {
		t.Errorf("%d Âmagos de Dente de Sabre com o item tirado do mundo, want 0", got[2395])
	}
	if got[itemMoeda5KK] != 1 || got[412] != 5 || got[413] != 3 {
		t.Errorf("resto do pacote = %v, want a moeda e as poeiras", got)
	}
}

// O template: o corpo do Troll Zumbi, maior; vida de 12 Trolls, dano, defesa e
// nível um pouco acima; XP no teto da Dungeon (0091); nada no Carry.
func TestReiTrollZumbiTemplate(t *testing.T) {
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
	rei, troll := ler(reiTrollZumbiTemplate), ler("Troll_Zumbi")
	c, tr := rei.CurrentScore, troll.CurrentScore
	if rei.Equip[0].Index != troll.Equip[0].Index || rei.Equip[6].Index != troll.Equip[6].Index {
		t.Errorf("corpo %d/%d, o Troll Zumbi é %d/%d", rei.Equip[0].Index, rei.Equip[6].Index, troll.Equip[0].Index, troll.Equip[6].Index)
	}
	if c.Con != 1200 || c.Con <= tr.Con {
		t.Errorf("CON %d, want 1200 (maior que a do Troll, %d: é o tamanho)", c.Con, tr.Con)
	}
	if c.MaxHp != 12*tr.MaxHp || c.Hp != c.MaxHp {
		t.Errorf("vida %d/%d, want 12 Trolls Zumbi (%d)", c.Hp, c.MaxHp, 12*tr.MaxHp)
	}
	if c.Damage != 250 || c.AC != 1000 || c.Level != 115 {
		t.Errorf("dano %d, defesa %d, nível %d; want 250, 1.000 e 115", c.Damage, c.AC, c.Level)
	}
	if rei.Equip[world.DividerEquipSlot].Index != 0 {
		t.Errorf("divisor de dano no slot 13 (%d): o Rei é para novato", rei.Equip[world.DividerEquipSlot].Index)
	}
	if rei.Exp > 10_000 {
		t.Errorf("XP %d, acima do teto de 10.000 da Dungeon (0091)", rei.Exp)
	}
	for i, it := range rei.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio", i, it.Index)
		}
	}
	if rei.Merchant != 0 || c.Merchant != 0 {
		t.Errorf("Merchant %d/%d: mob com Merchant não recebe dano", rei.Merchant, c.Merchant)
	}
	b := rei.BaseScore
	if b.MaxHp != c.MaxHp || b.Hp != c.Hp || b.Damage != c.Damage || b.AC != c.AC || b.Level != c.Level || b.Con != c.Con {
		t.Errorf("BaseScore %+v difere do CurrentScore %+v", b, c)
	}
	if nome := string(rei.Name[:len(reiTrollZumbiTemplate)]); nome != reiTrollZumbiTemplate || rei.Name[len(reiTrollZumbiTemplate)] != 0 {
		t.Errorf("nome %q, want %q terminado em NUL", rei.Name, reiTrollZumbiTemplate)
	}
}

// A 0147 só cita templates e itens que existem, com as chances pedidas.
func TestReiTrollZumbiMigracao(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, reiTrollZumbiMigracao)
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
	want := map[int16]int32{2395: 150, 4026: 20, 419: 50, 420: 30}
	if len(linhas) != 1 || len(linhas["Troll_Zumbi"]) != len(want) {
		t.Fatalf("regras = %v, want só as quatro do Troll_Zumbi", linhas)
	}
	for item, chance := range want {
		if got := linhas["Troll_Zumbi"][item]; got != chance {
			t.Errorf("Troll_Zumbi/%d = %d, want %d", item, got, chance)
		}
	}
}

// Todo bloco novo nasce em chão que se alcança a pé do ponto do Rei: cada tile da
// caixa que o gerador sorteia (x-r..x, y-r..y, generateMob) está na mesma área
// andável que (284,3753), pelo mapa assado que a IA usa para andar (route.Bake)
// e com o passo dela (altura dentro de ±MH do vizinho). Comparar com a altura do
// ponto não serve: o salão tem rampa, e -4 a -17 em três tiles é chão.
func TestReiTrollZumbiNasceNoChao(t *testing.T) {
	root := releaseDir(t)
	run := filepath.Join(root, "TMsrv", "run")
	h, err := content.LoadHeightMap(filepath.Join(run, "HeightMap.dat"))
	if err != nil {
		t.Skipf("HeightMap: %v", err)
	}
	a, err := content.LoadGrid(filepath.Join(run, "AttributeMap.dat"), content.AttributeMapDim)
	if err != nil {
		t.Skipf("AttributeMap: %v", err)
	}
	route.Bake(h, a)
	gens, err := content.LoadNPCGenerators(filepath.Join(run, "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	altura := func(x, y int) int { return int(int8(h.Data[y*h.Dim+x])) }
	// A área andável em volta do Rei, numa janela que cobre os três salões.
	const x0, y0, x1, y1 = 225, 3715, 345, 3835
	type tile struct{ x, y int }
	chao := map[tile]bool{{284, 3753}: true}
	fila := []tile{{284, 3753}}
	for len(fila) > 0 {
		p := fila[0]
		fila = fila[1:]
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				n := tile{p.x + dx, p.y + dy}
				if n.x < x0 || n.x > x1 || n.y < y0 || n.y > y1 || chao[n] {
					continue
				}
				if v, c := altura(n.x, n.y), altura(p.x, p.y); v < c+route.MH && v > c-route.MH {
					chao[n] = true
					fila = append(fila, n)
				}
			}
		}
	}
	for idx := 6150; idx <= 6160 && idx < len(gens); idx++ {
		g := gens[idx]
		for _, k := range []int{0, 4} {
			x, y, r := int(g.SegX[k]), int(g.SegY[k]), g.SegRange[k]
			for yy := y - r; yy <= y; yy++ {
				for xx := x - r; xx <= x; xx++ {
					if !chao[tile{xx, yy}] {
						t.Errorf("bloco %d (%s): (%d,%d) fora do chão do Rei (altura %d)",
							idx, g.Leader, xx, yy, altura(xx, yy))
					}
				}
			}
		}
	}
}
