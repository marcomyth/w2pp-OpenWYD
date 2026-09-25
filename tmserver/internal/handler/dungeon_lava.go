package handler

import (
	"slices"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os mini chefes do salão de lava do 2º andar da Dungeon, pedido do Marco em
// 25/09/2026 junto com o campo de XP da 0142 (cinco vezes mais Golem de Pedra e
// Anf Ninja no mesmo salão).
//
// São dois, um de cada, nos blocos 6147 (Boss_Golem) e 6148 (Boss_Anf_Ninja): o
// corpo do monstro comum, maior (CON), com 60.000 de vida e o divisor ÷50 no
// slot 13 — 3 milhões de vida real —, dano 1.800 e a XP no teto de 10.000 da
// Dungeon (0091). Carry vazio: o saque é todo daqui.
//
// Voltam 2 horas depois da morte, e ficam 2 horas de fora depois do boot, como os
// outros chefes. E somem se ninguém lutar com eles: 30 minutos sem luta e o chefe
// sai do mapa, voltando 2 horas depois de ter nascido — então, sem ninguém por
// perto, os dois aparecem de 2 em 2 horas, como foi pedido.
const (
	bossGolemTemplate    = "Boss_Golem"
	bossAnfNinjaTemplate = "Boss_Anf_Ninja"

	// lavaChefeHoras é a espera entre a morte e a volta (esperaDoRenascimento),
	// a espera depois do boot (ApplyChefesDaLavaBoot) e o ciclo de quem some.
	lavaChefeHoras = 2
	// lavaChefeSemLuta é quanto um chefe fica de pé sem luta antes de sumir.
	lavaChefeSemLuta = 30 * msPorMinuto
	// lavaChefeVoltaMinima segura a volta de quem sumiu depois de uma luta longa:
	// o resto do ciclo de 2 h pode ter acabado, e ele não volta na mesma hora.
	lavaChefeVoltaMinima = 10 * msPorMinuto

	msPorMinuto = 60_000

	// lavaChefeBase é a base do sorteio. 32768 % 12 = 8: os valores de 0 a 7
	// saem 2.731 vezes e os outros 2.730, um desvio de 0,04%.
	lavaChefeBase = 12
)

// lavaChefePremios são os prêmios, UM por morte: a Barra de Prata de 10Mi, 10
// âmagos de Sem Sela (N ou B), 30 de Dente de Sabre, ou 40 de Urso, Lobo ou Dragão
// Menor. Os quatro pedidos saem a 25% cada; o de 40 divide os seus 25% entre os
// três bichos. Os pesos foram escolha minha — o pedido não os deu.
var lavaChefePremios = []bossManticoraPremio{
	{"Barra de Prata (10Mi)", 3, itemBarraPrata10Mi, 0, 1},
	{"Pacote de Sem Sela", 3, 2396, 2401, 10},
	{"Pacote de Dente de Sabre", 3, 2395, 0, 30},
	{"Pacote de Urso", 1, 2394, 0, 40},
	{"Pacote de Lobo", 1, 2392, 0, 40},
	{"Pacote de Dragão Menor", 1, 2393, 0, 40},
}

// lavaChefeVigia é o que o relógio de sumir guarda de um chefe de pé: qual
// monstro (o ponteiro muda a cada nascimento, então um chefe novo no mesmo id
// não herda o relógio do anterior), quando nasceu, a última vez que lutou e o
// ciclo dele em ms (cicloDoChefe).
type lavaChefeVigia struct {
	mob    *world.Entity
	nasceu uint32
	luta   uint32
	ciclo  uint32
}

// cicloDoChefe devolve, em ms, o ciclo de um mini chefe que some sem luta — os
// dois da lava e o Boss Conjurador do spot de caveiras (dungeon_caveiras.go) —,
// ou 0 para qualquer outro monstro. Vale só no bloco dele: um "/gm criar" fica.
func cicloDoChefe(w *world.World, e *world.Entity) uint32 {
	if e.GenIndex < 0 {
		return 0
	}
	idx := int(e.GenIndex)
	switch {
	case isChefeDaLava(e) && geradorDeChefeDaLava(w, idx):
		return lavaChefeHoras * msPorHora
	case isBossConjurador(e) && geradorDoBossConjurador(w, idx):
		return conjuradorHoras * msPorHora
	}
	return 0
}

// isChefeDaLava diz se o monstro é um dos dois mini chefes, pelo nome do arquivo
// do template, como a Mesa: um "/gm criar Boss_Golem" também paga.
func isChefeDaLava(mob *world.Entity) bool {
	n := droprule.Canonical(mob.TemplateName)
	return n == droprule.Canonical(bossGolemTemplate) || n == droprule.Canonical(bossAnfNinjaTemplate)
}

// geradorDeChefeDaLava diz se o bloco idx é o de um dos dois mini chefes.
func geradorDeChefeDaLava(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	if g == nil {
		return false
	}
	n := droprule.Canonical(g.LeaderName)
	return n == droprule.Canonical(bossGolemTemplate) || n == droprule.Canonical(bossAnfNinjaTemplate)
}

// chefeDaLavaSaque entrega o prêmio da morte na bolsa de quem mata
// (entregaPremioDeChefe).
func (d *Dispatcher) chefeDaLavaSaque(w *world.World, reward, mob *world.Entity) {
	if !isChefeDaLava(mob) {
		return
	}
	d.entregaPremioDeChefe(w, reward, mob, lavaChefePremios, lavaChefeBase)
}

// ApplyChefesDaLavaBoot segura os dois mini chefes por lavaChefeHoras depois que
// o servidor sobe, pelo mesmo motivo do Gelo (ApplyGeloChefesBoot): o boot povoa
// todo bloco de uma vez, e sem isto um reinício valeria um chefe.
func (d *Dispatcher) ApplyChefesDaLavaBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDeChefeDaLava(w, idx) && w.DeferGenerator(idx, lavaChefeHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("mini chefes da lava nascem horas depois do boot", "horas", lavaChefeHoras, "blocos", blocos)
}

// emLuta diz se o monstro está brigando: tem alvo, tem quem o atacou na lista,
// ou está ferido. A vida conta porque quem bate de longe e sai correndo deixa o
// chefe sem alvo, e isso ainda é gente lutando com ele.
func emLuta(e *world.Entity) bool {
	return e.Target != 0 || hasEnemyList(e) || e.HP < e.MaxHP
}

// tickChefesDaLava tira do mapa o mini chefe que ficou lavaChefeSemLuta sem luta,
// e o põe de volta na fila para nascer um ciclo (cicloDoChefe) depois de ter
// nascido.
// Roda na passada do relógio de minuto (12 s), que é precisão de sobra para meia
// hora.
func (d *Dispatcher) tickChefesDaLava(w *world.World) {
	if d.tickCount%minTimerTicks != 0 {
		return
	}
	agora := w.Now()
	var somem []int
	w.ForEachMob(func(_ int, e *world.Entity) {
		ciclo := cicloDoChefe(w, e)
		if e.HP <= 0 || ciclo == 0 {
			return
		}
		idx := int(e.GenIndex)
		if d.lavaVigia == nil {
			d.lavaVigia = map[int]lavaChefeVigia{}
		}
		v, ok := d.lavaVigia[idx]
		if !ok || v.mob != e {
			v = lavaChefeVigia{mob: e, nasceu: agora, luta: agora, ciclo: ciclo}
		}
		if emLuta(e) {
			v.luta = agora
		}
		d.lavaVigia[idx] = v
		if agora-v.luta >= lavaChefeSemLuta && !slices.Contains(somem, idx) {
			somem = append(somem, idx)
		}
	})
	// Fora do laço: DeferGenerator tira o monstro do mundo.
	for _, idx := range somem {
		v := d.lavaVigia[idx]
		espera := uint32(lavaChefeVoltaMinima)
		if resto := v.ciclo - min(agora-v.nasceu, v.ciclo); resto > espera {
			espera = resto
		}
		delete(d.lavaVigia, idx)
		if w.DeferGenerator(idx, espera) > 0 {
			d.log.Info("mini chefe sumiu sem luta", "bloco", idx, "volta_em_min", espera/msPorMinuto)
		}
	}
}
