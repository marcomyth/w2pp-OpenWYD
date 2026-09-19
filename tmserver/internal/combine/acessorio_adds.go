package combine

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// Adds de acessório na +10 (decidido pelo Marco em 17/09/2026).
//
// O legado passa os três pares de efeito do SEGUNDO item para o primeiro
// (_MSG_CombineItemAilyn.cpp:113-118), então o add do item que fica na célula 0
// sumia. Com os acessórios ganhando add no drop (o Castelo Orc já grava um em
// cada Amuleto de Prata), a +10 passa a juntar os adds dos dois:
//
//  1. Junção: o mesmo add nos dois soma, até o teto do tipo (1,5 vez o maior
//     valor do drop).
//  2. Sorteio: Magia e Dano não convivem; se os dois sobrarem, fica um.
//  3. Se ainda sobrarem mais adds que os dois espaços, sorteia-se qual sai.
//  4. Mescla: dois adds de tipos diferentes que nenhum dos itens já trazia
//     juntos ficam, cada um, com 40% a 80% do valor, sorteado.
//
// Um item tem três pares de efeito e um deles guarda o refino e a joia, por isso
// dois espaços de add.

// Efeitos (ItemEffect.h) que a regra conhece.
const (
	efAddDano    uint8 = 2
	efAddHP      uint8 = 4
	efAddMP      uint8 = 5
	efAddCritico uint8 = 42 // décimo de percento: 10 = 1%
	efAddSanc    uint8 = 43
	efAddVazio   uint8 = 59 // o marcador de "espaço vazio" do sorteio de drop
	efAddMagia   uint8 = 60

	efAddSancAltLo uint8 = 116
	efAddSancAltHi uint8 = 125
)

// tetoDaJuncao é o máximo de um add somado na junção: 1,5 vez o maior valor que
// o drop do Castelo Orc dá (castelo_orc.go). Tipo fora da tabela soma até o
// limite do byte.
var tetoDaJuncao = map[uint8]int{
	efAddMagia:   15,
	efAddDano:    30,
	efAddCritico: 30,
	efAddHP:      105,
	efAddMP:      30, // o dos anéis (20)
}

// Faixa da mescla, em percentual do valor, inclusive.
const (
	mesclaMinPct = 40
	mesclaMaxPct = 80
)

// MaxAdds é quantos adds cabem num acessório refinado.
const MaxAdds = 2

func ehAdd(ef world.Effect) bool {
	switch {
	case ef.Effect == 0, ef.Effect == efAddVazio, ef.Effect == efAddSanc:
		return false
	case ef.Effect >= efAddSancAltLo && ef.Effect <= efAddSancAltHi:
		return false
	}
	return true
}

// addsDe devolve os adds de um item, na ordem dos espaços.
func addsDe(it world.Item) []world.Effect {
	var out []world.Effect
	for _, ef := range it.Effects {
		if ehAdd(ef) {
			out = append(out, ef)
		}
	}
	return out
}

func temTipo(adds []world.Effect, tipo uint8) bool {
	for _, a := range adds {
		if a.Effect == tipo {
			return true
		}
	}
	return false
}

// MesclarAdds aplica a regra aos adds de a e b e devolve os que o item da +10
// leva, em ordem. roll(n) é o rand()%n do mundo; só é chamado quando há sorteio.
func MesclarAdds(a, b world.Item, roll func(int) int) []world.Effect {
	addsA, addsB := addsDe(a), addsDe(b)

	// 1. Junção, na ordem em que os tipos aparecem (primeiro os do item a).
	var tipos []uint8
	soma := map[uint8]int{}
	for _, ef := range append(append([]world.Effect{}, addsA...), addsB...) {
		if _, ok := soma[ef.Effect]; !ok {
			tipos = append(tipos, ef.Effect)
		}
		soma[ef.Effect] += int(ef.Value)
	}
	for _, t := range tipos {
		if temTipo(addsA, t) && temTipo(addsB, t) {
			teto, ok := tetoDaJuncao[t]
			if !ok {
				teto = 255
			}
			soma[t] = min(soma[t], teto)
		}
		soma[t] = min(soma[t], 255)
	}

	// 2. Magia contra Dano.
	if _, mg := soma[efAddMagia]; mg {
		if _, ad := soma[efAddDano]; ad {
			perde := efAddDano
			if roll(2) == 1 {
				perde = efAddMagia
			}
			tipos = semTipo(tipos, perde)
		}
	}

	// 3. Mais adds que espaços.
	for len(tipos) > MaxAdds {
		i := roll(len(tipos))
		tipos = append(tipos[:i:i], tipos[i+1:]...)
	}

	out := make([]world.Effect, 0, len(tipos))
	for _, t := range tipos {
		out = append(out, world.Effect{Effect: t, Value: uint8(soma[t])})
	}

	// 4. Mescla: dois tipos que nenhum item trazia juntos.
	if len(out) == MaxAdds && !trazJuntos(addsA, out) && !trazJuntos(addsB, out) {
		for i := range out {
			out[i].Value = reduzir(out[i], mesclaMinPct+roll(mesclaMaxPct-mesclaMinPct+1))
		}
	}
	return out
}

func semTipo(tipos []uint8, t uint8) []uint8 {
	out := tipos[:0:0]
	for _, x := range tipos {
		if x != t {
			out = append(out, x)
		}
	}
	return out
}

func trazJuntos(adds, out []world.Effect) bool {
	for _, ef := range out {
		if !temTipo(adds, ef.Effect) {
			return false
		}
	}
	return true
}

// reduzir aplica o percentual e nunca zera o add. O crítico anda de 1% em 1%
// (10 em 10 no byte), como no drop: um 0,4% some na divisão por quatro do
// personagem.
func reduzir(ef world.Effect, pct int) uint8 {
	v := int(ef.Value) * pct / 100
	if ef.Effect == efAddCritico {
		v = v / 10 * 10
		return uint8(max(v, 10))
	}
	return uint8(max(v, 1))
}
