package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Reforma dos acessórios (2026-09-16): a +10 e o Odin passam a atender uma lista
// de acessórios, e a +10 ganha a evolução de degrau (Cristal → Místico,
// Necromântica → Siren → Ankh). As receitas moram em combine/acessorio.go.

// Chaves da Mesa das Máquinas. A +10 de acessório usa a chance da +10 das armas
// (Ailyn/Chance): é a mesma máquina e o mesmo anúncio.
const (
	chaveEvolucaoAcessorio = "Evolucao"
	chaveEvolucaoArcano    = "Evolucao_Arcano"

	// Sem linha salva, a evolução roda na chance da +10 e o Arcano na da
	// composição de armas do Odin, as duas máquinas de onde elas saíram.
	padraoEvolucaoAcessorio = 41
	padraoEvolucaoArcano    = 35
)

// combineAcessorioAilyn atende a +10 quando a célula 0 é um acessório da
// reforma. Responde false para o handler seguir com a +10 das armas.
func (d *Dispatcher) combineAcessorioAilyn(w *world.World, s *world.Session, e *world.Entity, it [protocol.MaxCombine]world.Item, sl [protocol.MaxCombine]int, active []int) bool {
	switch {
	case combine.AcessorioAteMais15(it[0].Index):
		if !combine.AcessorioMais10Recipe(it[:]) {
			d.recusarAcessorio(w, s, it)
			return true
		}
		d.acessorioMais10(w, s, e, it, sl, active)
		return true
	case isEvolucaoAcessorio(it[0].Index):
		if !combine.EvolucaoRecipe(it[:]) {
			d.recusarAcessorio(w, s, it)
			return true
		}
		d.evoluirAcessorio(w, s, e, it, sl, active)
		return true
	}
	return false
}

func isEvolucaoAcessorio(index int16) bool {
	_, ok := combine.EvolucaoAcessorio(index)
	return ok
}

// recusarAcessorio loga as sete células: sem isso "a máquina não aceitou meu
// brinco" não tem como ser separado de joia errada ou item fora do +9.
func (d *Dispatcher) recusarAcessorio(w *world.World, s *world.Session, it [protocol.MaxCombine]world.Item) {
	d.log.Info("ailyn recusou a receita de acessório",
		"conn", s.Conn,
		"itens", []int16{it[0].Index, it[1].Index, it[2].Index, it[3].Index, it[4].Index, it[5].Index, it[6].Index},
		"refino0", refine.Level(it[0]), "refino1", refine.Level(it[1]))
	d.refuseCombine(w, s, msgWrongCombination)
}

// acessorioMais10 segue a +10 das armas passo a passo — o mesmo custo, a mesma
// chance, o mesmo sorteio e o mesmo anúncio —, só com a receita própria.
func (d *Dispatcher) acessorioMais10(w *world.World, s *world.Session, e *world.Entity, it [protocol.MaxCombine]world.Item, sl [protocol.MaxCombine]int, active []int) {
	rate := d.mais10Chance(it[0])
	if !d.consumePositions(w, s, e, sl, active, func(i int) bool { return i < 2 }, umaUnidade) {
		d.refuseCombine(w, s, msgPilhaSemEspaco)
		return
	}
	e.Coin -= ailynCost
	d.sendEtc(w, s, e)
	rate, alq := chanceComAlquimia(e, rate)
	roll, success := combine.Roll(w.Rand(), rate)
	if !success {
		d.announceMais10(w, e.Name, it[0].Index, roll, rate, alq, false)
		sendCombineComplete(w, s, combineFailed)
		return
	}
	w.Rand().Intn(1) // o mesmo rand()%1 da +10 das armas, para o fluxo do RNG não divergir
	result := it[0]
	// O legado copia os efeitos do segundo item e o add do primeiro some
	// (_MSG_CombineItemAilyn.cpp:113-118). Aqui os adds dos dois passam pela
	// junção, sorteio e mescla decididos em 17/09 (combine/acessorio_adds.go),
	// e o primeiro espaço fica para o refino e a joia.
	result.Effects = [3]world.Effect{{Effect: efSanc}}
	for i, add := range combine.MesclarAdds(it[0], it[1], w.Rand().Intn) {
		result.Effects[i+1] = add
	}
	refine.Set(&result, 10, int(it[3].Index)-2441)
	e.Carry[sl[0]] = result
	e.Carry[sl[1]] = world.Item{}
	sendCarrySlot(w, s, e, sl[1])
	d.announceMais10(w, e.Name, result.Index, roll, rate, alq, true)
	sendCombineComplete(w, s, combineSuccess)
	sendCarrySlot(w, s, e, sl[0])
}

// evoluirAcessorio troca o +9 pelo degrau seguinte em +0 e gasta a cópia. Na
// falha, como na +10, o item e a cópia ficam; perdem-se a pedra e as joias.
func (d *Dispatcher) evoluirAcessorio(w *world.World, s *world.Session, e *world.Entity, it [protocol.MaxCombine]world.Item, sl [protocol.MaxCombine]int, active []int) {
	next, _ := combine.EvolucaoAcessorio(it[0].Index)
	rate := d.machineKeyRate("Ailyn", chaveEvolucaoAcessorio, padraoEvolucaoAcessorio)
	if !d.consumePositions(w, s, e, sl, active, func(i int) bool { return i < 2 }, umaUnidade) {
		d.refuseCombine(w, s, msgPilhaSemEspaco)
		return
	}
	e.Coin -= ailynCost
	d.sendEtc(w, s, e)
	rate, alq := chanceComAlquimia(e, rate)
	roll, success := combine.Roll(w.Rand(), rate)
	acao := "evoluir " + d.itemName(it[0].Index) + " em " + d.itemName(next)
	if !success {
		d.announceRoll(w, e.Name, acao, roll, rate, alq, false)
		sendCombineComplete(w, s, combineFailed)
		return
	}
	e.Carry[sl[0]] = world.Item{Index: next}
	e.Carry[sl[1]] = world.Item{}
	sendCarrySlot(w, s, e, sl[1])
	d.announceRoll(w, e.Name, acao, roll, rate, alq, true)
	sendCombineComplete(w, s, combineSuccess)
	sendCarrySlot(w, s, e, sl[0])
}

// odinArcano atende o Místico +15 → Arcano no Odin. Responde false quando a
// receita não é essa, para o Odin seguir com as dele.
//
// Na falha o Místico volta +15 e só as Pedras Secretas se perdem, como no +12
// do Odin: perder um +15 inteiro num sorteio seria a única máquina do jogo a
// cobrar isso.
func (d *Dispatcher) odinArcano(w *world.World, s *world.Session, e *world.Entity, items [protocol.MaxCombine]world.Item, slots [protocol.MaxCombine]int, active []int) bool {
	arcano, ok := combine.OdinArcanoRecipe(items[:])
	if !ok {
		return false
	}
	ativos := make([]int, 0, len(active))
	for _, i := range active {
		ativos = append(ativos, slots[i])
	}
	if !d.separarUnidadesParaMaquina(w, s, e, ativos, umaUnidade) {
		d.refuseCombine(w, s, msgPilhaSemEspaco)
		return true
	}
	for _, i := range active {
		e.Carry[slots[i]] = world.Item{}
		sendCarrySlot(w, s, e, slots[i])
	}
	roll := combine.RollOdin(w.Rand())
	chance, alq := chanceComAlquimia(e, d.machineKeyRate("Odin", chaveEvolucaoArcano, padraoEvolucaoArcano))
	acao := "evoluir " + d.itemName(items[0].Index) + " em " + d.itemName(arcano)
	if roll > chance {
		e.Carry[slots[0]] = items[0]
		sendCarrySlot(w, s, e, slots[0])
		d.announceRoll(w, e.Name, acao, roll, chance, alq, false)
		sendCombineComplete(w, s, combineFailed)
		return true
	}
	e.Carry[slots[0]] = world.Item{Index: arcano}
	sendCarrySlot(w, s, e, slots[0])
	d.announceRoll(w, e.Name, acao, roll, chance, alq, true)
	sendCombineComplete(w, s, combineSuccess)
	return true
}
