package refine

import (
	"math/rand"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/itemeffect"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func basesDoTeste() map[string]Base {
	return map[string]Base{
		"elmo":         {Pos: posElmo, ReqLvl: 60, Indice: 1000},
		"armadura":     {Pos: posArmadura, ReqLvl: 120, Indice: 1001},
		"calca":        {Pos: posCalca, ReqLvl: 200, Indice: 1002},
		"luva":         {Pos: posLuva, ReqLvl: 10, Indice: 1003},
		"bota":         {Pos: posBota, ReqLvl: 300, Indice: 1004},
		"espada":       {Pos: posArma, Unique: 41, ReqLvl: 150, Indice: 1005},
		"duas maos":    {Pos: posDuasMaos, Unique: 42, ReqLvl: 90, Indice: 1006},
		"lanca":        {Pos: posArma, Unique: uniqueLanca, ReqLvl: 180, Indice: 1007},
		"cajado":       {Pos: posDuasMaos, Unique: uniqueCajado, ReqLvl: 30, Indice: 1008},
		"refino fixo":  {Pos: posArmadura, ReqLvl: 100, Indice: 1009, Efeitos: []itemeffect.BaseEffect{{Eff: efSanc, Val: 3}}},
		"escudo":       {Pos: posEscudo, ReqLvl: 50, Indice: 1010},
		"acessorio":    {Pos: 256, ReqLvl: 50, Indice: 1011},
		"consumivel":   {Pos: 0, Indice: 1012},
		"poeira (mat)": {Pos: 0, Indice: 412},
	}
}

// A DICA NÃO PODE MENTIR. Para cada peça e cada distância de nível, milhares de
// drops de verdade, com o sorteio de verdade e com cada bônus de drop: tudo que
// sai nas vagas dos adicionais tem de estar dentro da faixa que Possiveis deu. E
// a faixa não pode ser folgada: o menor e o maior valor dela têm de aparecer.
func TestPossiveisCobreODrop(t *testing.T) {
	tab := TabelasPadrao()
	p := NovoPossiveis(tab)
	rng := rand.New(rand.NewSource(7))
	for nome, base := range basesDoTeste() {
		for _, nivel := range []int{1, base.ReqLvl, base.ReqLvl + 30, base.ReqLvl + 60, base.ReqLvl + 80, base.ReqLvl + 130, 209, 210, 256, 400} {
			faixas := p.Do(base, nivel)
			por := map[uint8]Faixa{}
			for _, f := range faixas {
				if f.Min == 0 || f.Min > f.Max {
					t.Fatalf("%s nível %d: faixa inválida %+v", nome, nivel, f)
				}
				por[f.Efeito] = f
			}
			visto := map[uint8][2]uint8{}
			for i := 0; i < 20000; i++ {
				it := world.Item{Index: int16(base.Indice)}
				tab.Drop(&it, base, nivel, []int{0, 8, 16, 32, 48}[i%5], false, rng.Intn)
				for _, vaga := range []int{1, 2} {
					e := it.Effects[vaga]
					if e.Effect == 0 || e.Effect == efUnique || e.Value == 0 {
						continue
					}
					f, ok := por[e.Effect]
					if !ok || e.Value < f.Min || e.Value > f.Max {
						t.Fatalf("%s nível %d: o drop deu efeito %d valor %d, fora de %+v", nome, nivel, e.Effect, e.Value, faixas)
					}
					v, tem := visto[e.Effect]
					if !tem {
						v = [2]uint8{e.Value, e.Value}
					}
					visto[e.Effect] = [2]uint8{min(v[0], e.Value), max(v[1], e.Value)}
				}
			}
			for ef, f := range por {
				// O extremo raro (o degrau mais alto sai 1 ou 2 vezes em cem) aparece
				// em 20 mil drops; se não apareceu, a faixa está folgada.
				if v, tem := visto[ef]; !tem || v[0] != f.Min || v[1] != f.Max {
					t.Errorf("%s nível %d: efeito %d, a faixa diz %d~%d e os drops deram %v", nome, nivel, ef, f.Min, f.Max, v)
				}
			}
		}
	}
}

// O que não ganha adicional não tem faixa: escudo, acessório, consumível.
func TestPossiveisSoParaQuemGanhaAdicional(t *testing.T) {
	p := NovoPossiveis(TabelasPadrao())
	b := basesDoTeste()
	for _, nome := range []string{"escudo", "acessorio", "consumivel", "poeira (mat)"} {
		if f := p.Do(b[nome], 200); len(f) != 0 {
			t.Errorf("%s: veio %+v, quero nada", nome, f)
		}
	}
	if f := p.Do(b["espada"], 200); len(f) == 0 {
		t.Error("a espada veio sem faixa")
	}
	desligado := TabelasPadrao()
	desligado.Ligado = false
	if f := NovoPossiveis(desligado).Do(b["espada"], 200); len(f) != 0 {
		t.Errorf("com o sorteio desligado veio %+v", f)
	}
}
