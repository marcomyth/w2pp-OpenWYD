package refine

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// O teto dos adds de armadura (Marco, 27/09/2026): adds combinados — Defesa,
// Magia, Dano, Crítico — param em Defesa 30, Crítico 7% e Magia 10%, e Defesa
// acima de 30 só existe sozinha.
//
// É um teto no item que SAI, aplicado no fim de todo caminho que gera add de
// armadura (Drop e ClasseBonus). Não gasta sorteio: a sequência do rand() fica a
// mesma, só o valor gravado é cortado. E vale seja qual for a faixa de drop que o
// painel gravar.
//
// Por que é preciso no drop, que é o legado: o elmo sorteia Magia em passos de 2
// até 6 passos, e saía Magia 12% junto de Defesa (0,14% dos elmos a 75+ níveis
// de distância); e o bônus especial que o drop põe na vaga do refino pode ser
// Defesa 2-6, somada aos 25 do add — Defesa 31 com Crítico.
const (
	tetoDefesaCombinada = 30
	tetoCriticoByte     = 70 // o tooltip divide o byte por dez: 7%
	tetoMagia           = 10
)

// ehArmadura diz se nPos é uma das cinco peças de armadura.
func ehArmadura(nPos int) bool {
	switch nPos {
	case posElmo, posArmadura, posCalca, posLuva, posBota:
		return true
	}
	return false
}

// LimitaAddsDeArmadura corta os adds de it até o teto. Reporta se mudou algo.
// Fora das cinco peças de armadura não faz nada.
func LimitaAddsDeArmadura(it *world.Item, nPos int) bool {
	if it == nil || !ehArmadura(nPos) {
		return false
	}
	soma := func(ef uint8) int {
		n := 0
		for _, e := range it.Effects {
			if e.Effect == ef {
				n += int(e.Value)
			}
		}
		return n
	}
	outros := 0
	for _, ef := range []uint8{efMagic, efDamageBonus, efCritical2} {
		if soma(ef) > 0 {
			outros++
		}
	}
	combinado := outros > 1 || (outros == 1 && soma(efAC) > 0)
	if !combinado {
		return false // Defesa sozinha, ou um add só: nada a cortar
	}
	mudou := cortaAte(it, efAC, tetoDefesaCombinada)
	mudou = cortaAte(it, efCritical2, tetoCriticoByte) || mudou
	mudou = cortaAte(it, efMagic, tetoMagia) || mudou
	return mudou
}

// cortaAte baixa a soma do efeito ef nas três vagas até teto. Corta primeiro a
// vaga 0 — onde o drop põe o bônus especial, que é o que se soma por cima dos dois
// adds — e depois as vagas 1 e 2. Uma vaga que chega a zero vira EF_UNIQUE, a vaga
// vazia do jogo, para o tooltip não mostrar "Defesa 0".
func cortaAte(it *world.Item, ef uint8, teto int) bool {
	total := 0
	for _, e := range it.Effects {
		if e.Effect == ef {
			total += int(e.Value)
		}
	}
	excesso := total - teto
	if excesso <= 0 {
		return false
	}
	for i := range it.Effects {
		if excesso == 0 {
			break
		}
		if it.Effects[i].Effect != ef {
			continue
		}
		v := int(it.Effects[i].Value)
		corte := min(v, excesso)
		v -= corte
		excesso -= corte
		if v == 0 {
			it.Effects[i] = world.Effect{Effect: efUnique}
		} else {
			it.Effects[i].Value = uint8(v)
		}
	}
	return true
}

// DentroDoLimite diz se it já cabe no teto dos adds de armadura, sem mexer nele.
func DentroDoLimite(it world.Item, nPos int) bool {
	return !LimitaAddsDeArmadura(&it, nPos)
}
