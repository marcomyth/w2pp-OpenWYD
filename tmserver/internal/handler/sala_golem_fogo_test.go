package handler

import (
	"bytes"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const salaGolemFogoMigracao = "0152_sala_golem_de_fogo.up.sql"

// Os bichos da sala voltam em 10 s e o chefe em 2 h; o Golem de Fogo e a Gárgula
// comuns seguem os 15 s da fila.
func TestSalaGolemFogoRenascimento(t *testing.T) {
	gens := make([]*world.Generator, 36)
	nomes := []string{golemFogoDaSalaTemplate, gargulaDaSalaTemplate, bossGolemFogoTemplate, "Golem_de_Fogo", "Gargula"}
	for i, nome := range nomes {
		g := geradorSozinho(10_000, int16(10+2*i))
		g.LeaderName = nome
		gens[30+i] = g
	}
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators(gens)
	d := dispatcherQuieto()
	want := []uint32{lavaSalaRenasce, lavaSalaRenasce, lavaChefeHoras * msPorHora, world.DefaultRespawnDelay, world.DefaultRespawnDelay}
	for i, nome := range nomes {
		if got := d.esperaDoRenascimento(w, 30+i); got != want[i] {
			t.Errorf("%s volta em %d ms, want %d", nome, got, want[i])
		}
	}
}

// O Boss Golem de Fogo some sem luta como os da lava: 30 min e sai do mapa.
func TestBossGolemFogoSomeSemLuta(t *testing.T) {
	var agora uint32 = 1_000
	w := mundoDaLava(t, &agora, bossGolemFogoTemplate)
	d := dispatcherQuieto()
	passadaDaLava(d, w)
	agora += lavaChefeSemLuta
	passadaDaLava(d, w)
	if doBloco(w, 30) != nil {
		t.Fatal("30 min sem luta e o Boss Golem de Fogo continua de pé")
	}
}

// Cada morte solta UM prêmio: 20 âmagos de Sem Sela (N ou B) ou 30 de Dente de
// Sabre, metade cada; e nada do prêmio dos chefes da lava.
func TestBossGolemFogoSoltaUmPremio(t *testing.T) {
	want := map[int16]int{2396: 20, 2401: 20, 2395: 30}
	vistos := map[int16]bool{}
	for i := range 20 {
		d, w, killer := mobKilledWorld(t)
		for range i { // cada mundo novo começa na mesma semente
			w.Rand().Intn(2)
		}
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(250, 0, 0), bossGolemFogoTemplate))
		achou := 0
		for _, it := range killer.Carry {
			if it.Index == itemBarraPrata10Mi || it.Index == 2392 || it.Index == 2393 || it.Index == 2394 {
				t.Errorf("item %d do prêmio da lava no Boss Golem de Fogo", it.Index)
			}
			q, ok := want[it.Index]
			if !ok {
				continue
			}
			achou++
			vistos[it.Index] = true
			if got := itemAmount(it); got != q {
				t.Errorf("item %d com %d unidades, want %d", it.Index, got, q)
			}
		}
		if achou != 1 {
			t.Fatalf("%d prêmios na bolsa, want 1", achou)
		}
	}
	if !vistos[2395] || (!vistos[2396] && !vistos[2401]) {
		t.Errorf("em 20 mortes saíram %v, want os dois pacotes", vistos)
	}
	vezes := map[string]int{}
	for v := range 32768 {
		vezes[sorteiaPremioDeChefe(golemFogoPremios, v%len(golemFogoPremios)).nome]++
	}
	for _, p := range golemFogoPremios {
		if vezes[p.nome] != 16384 {
			t.Errorf("%s sai %d vezes em 32768, want metade", p.nome, vezes[p.nome])
		}
	}
}

// As cópias da sala são o monstro comum sem o Carry; o chefe é o Golem de Fogo
// maior, com a vida real e o dano dos chefes da lava.
func TestSalaGolemFogoTemplates(t *testing.T) {
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
	for copia, original := range map[string]string{golemFogoDaSalaTemplate: "Golem_de_Fogo", gargulaDaSalaTemplate: "Gargula"} {
		c, o := ler(copia), ler(original)
		for i, it := range c.Carry {
			if it.Index != 0 {
				t.Errorf("%s: Carry[%d] = %d, want vazio", copia, i, it.Index)
			}
		}
		c.Carry, o.Carry = [64]savefmt.Item{}, [64]savefmt.Item{}
		if c != o {
			t.Errorf("%s difere de %s fora do Carry", copia, original)
		}
		if !bytes.Equal(c.Name[:], o.Name[:]) {
			t.Errorf("%s: nome de dentro %q, want %q", copia, c.Name, o.Name)
		}
	}
	boss, golem, mini := ler(bossGolemFogoTemplate), ler("Golem_de_Fogo"), ler(bossGolemTemplate)
	if boss.Equip[0].Index != golem.Equip[0].Index {
		t.Errorf("corpo %d, o Golem de Fogo é %d", boss.Equip[0].Index, golem.Equip[0].Index)
	}
	if boss.CurrentScore.Con <= golem.CurrentScore.Con {
		t.Errorf("CON %d, want maior que a do Golem de Fogo (%d)", boss.CurrentScore.Con, golem.CurrentScore.Con)
	}
	if boss.CurrentScore.MaxHp != mini.CurrentScore.MaxHp || boss.Equip[13] != mini.Equip[13] || boss.CurrentScore.Damage != mini.CurrentScore.Damage {
		t.Errorf("vida %d, divisor %+v, dano %d; want os do Boss Golem (%d, %+v, %d)",
			boss.CurrentScore.MaxHp, boss.Equip[13], boss.CurrentScore.Damage,
			mini.CurrentScore.MaxHp, mini.Equip[13], mini.CurrentScore.Damage)
	}
	if boss.Exp > 10_000 {
		t.Errorf("XP %d, acima do teto de 10.000 da Dungeon (0091)", boss.Exp)
	}
	for i, it := range boss.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio", i, it.Index)
		}
	}
}

// A 0152 dá às duas cópias o saque pedido, e só ele; o Sem Sela é o mais difícil.
func TestSalaGolemFogoMigracao(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, salaGolemFogoMigracao)
	if len(linhas) != 2 {
		t.Fatalf("a 0152 cita %d monstros, want as duas cópias", len(linhas))
	}
	pedidos := []int16{591, 592, 593, 594, 595, 419, 420, 4019, 4026}
	for _, mob := range []string{golemFogoDaSalaTemplate, gargulaDaSalaTemplate} {
		porItem := linhas[mob]
		if len(porItem) != len(pedidos)+2 {
			t.Errorf("%s com %d itens, want %d", mob, len(porItem), len(pedidos)+2)
		}
		menor := int32(1 << 30)
		for _, item := range pedidos {
			c := porItem[item]
			if c <= 0 {
				t.Errorf("%s: item %d a %d, want > 0", mob, item, c)
			}
			if _, ok := items.Get(int(item)); !ok {
				t.Errorf("item %d não existe no ItemList", item)
			}
			if item < 591 || item > 595 { // os brincos dividem entre si uma chance só
				menor = min(menor, c)
			}
		}
		for _, item := range []int16{2396, 2401} {
			if c := porItem[item]; c <= 0 || c >= menor {
				t.Errorf("%s: Sem Sela %d a %d, want > 0 e abaixo do resto (%d)", mob, item, c, menor)
			}
		}
	}
}
