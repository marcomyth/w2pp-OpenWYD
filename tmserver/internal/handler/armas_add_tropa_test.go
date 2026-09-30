package handler

import (
	"math"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/ciclopes"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const desertoAddRaroMigracao = "0188_deserto_armas_add_raro.up.sql"

// mortesPorHora é o ritmo de quem farma o Verme, tirado dos logs de "drop table
// hit" de produção (30/09/2026); é a régua das "5 horas" do pedido.
const mortesPorHora = 300

// fatiaDaEscada conta, entre os 32.768 valores do rand() de 15 bits, quantos caem
// em cada valor da escada pelo sorteio de sortearAddArma (Intn = rand() % total).
func fatiaDaEscada(tabela []addArma) map[int]float64 {
	total := pesoTotal(tabela)
	out := map[int]float64{}
	for r := range 32768 {
		out[linhaDoPeso(tabela, r%total).valor] += 1.0 / 32768
	}
	return out
}

// A escada da tropa: seis degraus do bônus de drop, o maior sempre o mais raro,
// e o que passa do teto do legado raro de verdade. Os números são os da doc de
// armas_add_tropa.go, já com o viés do rand().
func TestEscadaDaTropaPagaRaro(t *testing.T) {
	for _, c := range []struct {
		nome    string
		tabela  []addArma
		degraus []int
	}{
		{"física", addTropaFisica, []int{27, 36, 45, 54, 63, 72}},
		{"mágica", addTropaMagica, []int{12, 16, 20, 24, 28, 32}},
	} {
		if len(c.tabela) != len(c.degraus) {
			t.Fatalf("%s: %d degraus, want %d", c.nome, len(c.tabela), len(c.degraus))
		}
		for i, l := range c.tabela {
			if l.valor != c.degraus[i] {
				t.Errorf("%s degrau %d = %d, want %d", c.nome, i, l.valor, c.degraus[i])
			}
			if i > 0 && l.peso >= c.tabela[i-1].peso {
				t.Errorf("%s: o %d (peso %d) não é mais raro que o %d (peso %d)", c.nome, l.valor, l.peso, c.tabela[i-1].valor, c.tabela[i-1].peso)
			}
		}
		fatia := fatiaDaEscada(c.tabela)
		alto := func(desde int) float64 {
			s := 0.0
			for _, v := range c.degraus[desde:] {
				s += fatia[v]
			}
			return s
		}
		for _, w := range []struct {
			desde int
			want  float64
		}{
			{3, 0.0430}, // 54+ (24+ de magia)
			{4, 0.0156}, // 63+
			{5, 0.0039}, // 72
		} {
			if got := alto(w.desde); math.Abs(got-w.want) > 0.0002 {
				t.Errorf("%s: %d ou mais em %.3f%% das armas, want %.2f%%", c.nome, c.degraus[w.desde], got*100, w.want*100)
			}
		}
	}
}

// A 0188: os cinco monstros do Deserto soltam arma a ~1,5% por morte, só as da
// lista das Armas D (as que ganham add), e todas as que a 0180 e a 0186 davam a
// eles foram reescritas. Com a escada da tropa, quem mata 300 por hora leva ao
// menos 5 h para ver um 54+ em qualquer um deles.
func TestDesertoArmaComAddAltoLevaCincoHoras(t *testing.T) {
	novas := linhasComZero(t, desertoAddRaroMigracao)
	antigas := linhasComZero(t, desertoArmasMigracao)
	for mob, l := range linhasComZero(t, "0186_deserto_e_vira_d.up.sql") {
		if antigas[mob] == nil {
			antigas[mob] = map[int16]int32{}
		}
		for id, c := range l {
			antigas[mob][id] = c
		}
	}
	p54 := 0.0
	for v, p := range fatiaDaEscada(addTropaFisica) {
		if v >= 54 {
			p54 += p
		}
	}
	for _, mob := range []string{"Manticora", "Adamant_Tauron", "Aeon_Tauron", "Taron_Assassino", "Verme_"} {
		if len(novas[mob]) == 0 {
			t.Errorf("%s fora da 0188", mob)
			continue
		}
		porMorte := 0.0
		for id, c := range novas[mob] {
			if !armasDDoDesertoFisicas[id] && !armasDDoDesertoMagicas[id] {
				t.Errorf("%s: a 0188 dá o item %d, que não é Arma D com add", mob, id)
			}
			porMorte += taxaPaga(c)
		}
		for id, c := range antigas[mob] {
			if (armasDDoDesertoFisicas[id] || armasDDoDesertoMagicas[id]) && c > 0 {
				if _, ok := novas[mob][id]; !ok {
					t.Errorf("%s: a arma %d segue com a chance antiga (%d)", mob, id, c)
				}
			}
		}
		if porMorte < 0.014 || porMorte > 0.016 {
			t.Errorf("%s: arma em %.3f%% das mortes, want ~1,5%%", mob, porMorte*100)
		}
		if horas := 1 / (mortesPorHora * porMorte * p54); horas < 5 {
			t.Errorf("%s: um 54+ a cada %.1f h, want ao menos 5 h", mob, horas)
		}
	}
}

// Os três ganchos da tropa sorteiam a escada da tropa, e não a antiga de cada
// spot: em 5.000 armas o 27 aparece, e 54 ou mais fica perto dos 4,3%. (A escada
// antiga cabe dentro da nova; só o teste de valores não pegaria a troca.)
func TestGanchosDaTropaUsamAEscadaNova(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	for _, c := range []struct {
		mob    string
		arma   int16
		gancho func(*Dispatcher, *world.World, *world.Entity, *world.Item)
	}{
		{"Verme_", 885, (*Dispatcher).desertoFinish},
		{ciclopes.CiclopeCruelSpot, 868, (*Dispatcher).ciclopesFinish},
		{"Caveira_Lanc_Fonte", 868, (*Dispatcher).caveirasFinish},
	} {
		m := spawnNamed(t, w, expMobTemplate(150, 0, 0), c.mob)
		const n = 5000
		baixo, alto := 0, 0
		for range n {
			it := world.Item{Index: c.arma}
			c.gancho(d, w, m, &it)
			switch v := int(it.Effects[1].Value); {
			case v == 27:
				baixo++
			case v >= 54:
				alto++
			}
		}
		if baixo == 0 {
			t.Errorf("%s: nenhum 27 em %d armas", c.mob, n)
		}
		if got := float64(alto) / n; got > 0.07 {
			t.Errorf("%s: 54 ou mais em %.1f%% das armas, want perto de 4,3%%", c.mob, got*100)
		}
	}
}
