package handler

import (
	"bytes"
	"io"
	"log/slog"
	"math"
	"slices"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/ciclopes"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const ciclopesMigracao = "0169_ciclopes_spot_saque.up.sql"

// O bloco do Ciclope Tirano volta 4 h depois da morte.
func TestCiclopeTiranoRenasceEm4Horas(t *testing.T) {
	boss := geradorSozinho(10_000, 10)
	boss.LeaderName = ciclopes.Chefe
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: boss})
	if got := dispatcherQuieto().esperaDoRenascimento(w, 30); got != 4*msPorHora {
		t.Errorf("Ciclope Tirano volta em %d ms, want 4 h", got)
	}
}

// Depois do boot o chefe só aparece 4 h depois, e o monstro comum fica de pé.
func TestCiclopeTiranoNasceHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	w := mundoDaLava(t, &agora, ciclopes.Chefe)
	dispatcherQuieto().ApplyCiclopeTiranoBoot(w)
	if doBloco(w, 30) != nil || doBloco(w, 31) == nil {
		t.Fatal("depois do boot: want só o monstro comum de pé")
	}
	quatro := uint32(ciclopeTiranoHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + quatro - 1); len(got) != 0 {
		t.Fatalf("o chefe voltou antes das 4 h: %v", got)
	}
	if got := w.SpawnDueRespawns(agora + quatro); len(got) != 1 || doBloco(w, 30) == nil {
		t.Fatalf("4 h depois do boot voltaram %d, want o chefe de pé", len(got))
	}
}

// Uma Arma C dos monstros do spot sai com o add pedido — dano 45 a 72 nas físicas,
// magia 20 a 32 nas lanças e cajados —, o refino do bônus fica e o segundo add
// sai; todos os degraus aparecem, e o mais alto é o mais raro.
func TestCiclopesCarimbamArmasC(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	for _, mob := range []string{ciclopes.CiclopeCruelSpot, ciclopes.LanceiroZakumSpot} {
		m := spawnNamed(t, w, expMobTemplate(150, 0, 0), mob)
		for _, c := range []struct {
			armas  []int16
			efeito uint8
			want   []int
		}{
			{armasCFisicas, efDamage, []int{45, 54, 63, 72}},
			{armasCMagicas, efMagic, []int{20, 24, 28, 32}},
		} {
			visto := map[int]int{}
			for i := range 800 {
				arma := world.Item{Index: c.armas[i%len(c.armas)], Effects: [3]world.Effect{{Effect: efSanc, Value: 1}, {Effect: 26, Value: 3}, {Effect: efDamage, Value: 9}}}
				d.ciclopesFinish(w, m, &arma)
				if arma.Effects[0] != (world.Effect{Effect: efSanc, Value: 1}) {
					t.Fatalf("%s %d: refino %+v, want o do bônus", mob, arma.Index, arma.Effects[0])
				}
				if arma.Effects[1].Effect != c.efeito || !slices.Contains(c.want, int(arma.Effects[1].Value)) {
					t.Fatalf("%s %d: add %+v, want efeito %d em %v", mob, arma.Index, arma.Effects[1], c.efeito, c.want)
				}
				if arma.Effects[2] != (world.Effect{}) {
					t.Fatalf("%s %d: segundo add %+v, want vazio", mob, arma.Index, arma.Effects[2])
				}
				visto[int(arma.Effects[1].Value)]++
			}
			for _, v := range c.want {
				if visto[v] == 0 {
					t.Errorf("%s: o add %d nunca saiu em 800 armas", mob, v)
				}
			}
			if visto[c.want[0]] <= visto[c.want[len(c.want)-1]] {
				t.Errorf("%s: o maior add (%d) saiu tanto quanto o menor (%d)", mob, visto[c.want[len(c.want)-1]], visto[c.want[0]])
			}
		}
	}
}

// Fora do spot nada muda: o Ciclope Cruel e o Lanceiro Zakum originais não
// carimbam Arma C.
func TestCiclopesNaoMexemForaDoSpot(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	antes := world.Item{Index: 868, Effects: [3]world.Effect{{Effect: efSanc, Value: 1}, {Effect: 26, Value: 3}, {Effect: efDamage, Value: 9}}}
	for _, mob := range []string{ciclopes.CiclopeCruel, "Lanceiro_Zakum", "Anciao_Ciclops"} {
		arma := antes
		d.ciclopesFinish(w, spawnNamed(t, w, expMobTemplate(150, 0, 0), mob), &arma)
		if arma != antes {
			t.Errorf("Lâmina Espiritual de %s, fora do spot, virou %+v", mob, arma)
		}
	}
}

// O saque do Ciclope Tirano: as três barras a 80%, no máximo UMA Arma D a 60%, com
// o add do spot, e cada ovo a 10%. Em 1.000 mortes as taxas ficam perto do pedido.
func TestCiclopeTiranoSaque(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	const mortes = 1000
	var barras, armas, ovoN, ovoB int
	for range mortes {
		killer.Carry = [len(killer.Carry)]world.Item{}
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(350, 0, 0), ciclopes.Chefe))
		nArmas, nBarras := 0, 0
		for _, it := range killer.Carry {
			switch {
			case it.Index == itemBarraPrata10Mi:
				nBarras += itemAmount(it)
			case slices.Contains(armasDDoTirano, it.Index):
				nArmas++
				efeito, want := uint8(efDamage), []int{45, 54, 63, 72}
				if armasTrollMagicas[it.Index] {
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
	// Os números do pedido, e não as constantes: mudar uma constante tem de
	// quebrar este teste.
	perto("barras", barras, 80)
	perto("Arma D", armas, 60)
	perto("Ovo Leve N", ovoN, 10)
	perto("Ovo Leve B", ovoB, 10)
}

// O Ciclope Tirano tem os números do Troll Enigma no corpo do Ciclope Cruel, e a
// bolsa vazia: o saque é do código.
func TestCiclopeTiranoTemplate(t *testing.T) {
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
	b, cruel, enigma := ler(ciclopes.Chefe), ler(ciclopes.CiclopeCruel), ler("ATroll_Enigma")
	if nome := string(bytes.TrimRight(b.Name[:], string([]byte{0}))); nome != ciclopes.Chefe {
		t.Errorf("nome %q, want o do arquivo", nome)
	}
	for i := range 13 {
		if b.Equip[i] != cruel.Equip[i] {
			t.Errorf("Equip[%d] = %+v, o Ciclope Cruel usa %+v", i, b.Equip[i], cruel.Equip[i])
		}
	}
	if b.Equip[13].Index != 0 {
		t.Errorf("Equip[13] = %d, want sem divisor", b.Equip[13].Index)
	}
	for _, s := range []savefmt.Score{b.BaseScore, b.CurrentScore} {
		e := enigma.CurrentScore
		if s.Level != e.Level || s.MaxHp != e.MaxHp || s.Hp != e.Hp || s.AC != e.AC || s.Damage != e.Damage || s.Con != e.Con {
			t.Errorf("score %+v, want os números do Enigma %+v", s, e)
		}
	}
	if b.Resist != enigma.Resist {
		t.Errorf("resistência %v, want a do Enigma %v", b.Resist, enigma.Resist)
	}
	for i, it := range b.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio: o saque é do código", i, it.Index)
		}
	}
}

// As cópias do spot são o original byte a byte, menos o divisor de dano do slot
// 13 do Ciclope Cruel, que no spot sai.
func TestCiclopesCopiasDoSpot(t *testing.T) {
	root := releaseDir(t)
	ler := func(nome string) []byte {
		b, _, err := npctemplate.Load(root, nome)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		return b
	}
	const divisor = 140 + 13*8 // Equip[13]
	cruel, spot := ler(ciclopes.CiclopeCruel), ler(ciclopes.CiclopeCruelSpot)
	if int(cruel[divisor])|int(cruel[divisor+1])<<8 != 786 {
		t.Fatal("o Ciclope Cruel original não tem mais o divisor 786: o teste não prova nada")
	}
	for i := range cruel {
		dentro := i >= divisor && i < divisor+8
		if dentro && spot[i] != 0 {
			t.Fatalf("byte %d da cópia = %d, want o divisor zerado", i, spot[i])
		}
		if !dentro && spot[i] != cruel[i] {
			t.Fatalf("byte %d da cópia = %d, o original tem %d", i, spot[i], cruel[i])
		}
	}
	if !bytes.Equal(ler("Lanceiro_Zakum"), ler(ciclopes.LanceiroZakumSpot)) {
		t.Error("a cópia do Lanceiro Zakum difere do original")
	}
}

// Os dois do spot e todo Ciclope Cruel apanham; o Lanceiro Zakum de fora do spot
// continua como era. É a regra do mundo com os templates de verdade.
func TestCiclopesApanham(t *testing.T) {
	root := releaseDir(t)
	_, w, _ := mobKilledWorld(t)
	for _, c := range []struct {
		template string
		npc      bool
	}{
		{ciclopes.CiclopeCruel, false},
		{ciclopes.CiclopeCruelSpot, false},
		{ciclopes.LanceiroZakumSpot, false},
		{"Lanceiro_Zakum", true},
	} {
		tmpl, _, err := npctemplate.Load(root, c.template)
		if err != nil {
			t.Fatalf("%s: %v", c.template, err)
		}
		if got := spawnNamed(t, w, tmpl, c.template).NonCombatNPC; got != c.npc {
			t.Errorf("%s: NonCombatNPC = %v, want %v", c.template, got, c.npc)
		}
	}
}

// A 0169 cita só os três templates, com os itens que existem e as chances já
// compensadas pelo rand() do MSVC: 1%, 0,5% e 0,05% pagos de verdade.
func TestCiclopesMigracao(t *testing.T) {
	linhas := linhasComZero(t, ciclopesMigracao)
	comum := map[int16]float64{4026: 1, 419: 1, 420: 0.5}
	ciclope := map[int16]float64{2395: 1, 2396: 0.05, 2401: 0.05, 2397: 0.05, 2402: 0.05}
	armas := slices.Concat(armasCFisicas, armasCMagicas)
	quer := map[string]map[int16]float64{}
	for _, mob := range []string{ciclopes.CiclopeCruel, ciclopes.CiclopeCruelSpot, ciclopes.LanceiroZakumSpot} {
		quer[mob] = map[int16]float64{}
		for it, p := range comum {
			quer[mob][it] = p
		}
	}
	for _, mob := range []string{ciclopes.CiclopeCruel, ciclopes.CiclopeCruelSpot} {
		for it, p := range ciclope {
			quer[mob][it] = p
		}
	}
	for _, mob := range []string{ciclopes.CiclopeCruelSpot, ciclopes.LanceiroZakumSpot} {
		for _, it := range armas {
			quer[mob][it] = 0.05
		}
	}
	if len(linhas) != len(quer) {
		t.Errorf("%d templates na 0169, want %d", len(linhas), len(quer))
	}
	for mob, itens := range quer {
		for it, pct := range itens {
			c, ok := linhas[mob][it]
			if !ok {
				t.Errorf("%s sem o item %d", mob, it)
				continue
			}
			if got := taxaPaga(c) * 100; math.Abs(got-pct) > pct*0.05 {
				t.Errorf("%s item %d: chance %d paga %.4f%%, want %.2f%%", mob, it, c, got, pct)
			}
		}
		if len(linhas[mob]) != len(itens) {
			t.Errorf("%s: %d itens na 0169, want %d", mob, len(linhas[mob]), len(itens))
		}
	}
}
