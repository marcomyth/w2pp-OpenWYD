package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/ciclopes"
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O spot dos Ciclopes Cruéis e dos Lanceiros Zakum (x 2195-2285, y 1300-1380),
// pedido do Marco em 26/09/2026 com print em (2239,1333):
//
//   - os dois apanham (internal/ciclopes: o byte de NPC deles era lido errado) e,
//     no spot, nascem 3x, pelas cópias Ciclope_Cruel_Spot (sem o divisor de dano
//     do slot 13) e Lanceiro_Zakum_Spot;
//   - o saque novo vem pela Mesa (migração 0170): todo Ciclope Cruel do mapa solta
//     Moeda de Prata 1Mi, os Restos e os Âmagos de Dente de Sabre, sem Sela e
//     Fantasma; as cópias do spot soltam também as Armas C, com o add daqui
//     (ciclopesFinish);
//   - um chefe, o Ciclope Tirano (bloco 6164): o corpo do Ciclope Cruel com os
//     números do Troll Enigma, de 4 em 4 horas. O saque dele é todo deste arquivo
//     (ciclopeTiranoSaque).
const (
	// ciclopeTiranoHoras é a espera entre a morte e a volta, e depois do boot.
	ciclopeTiranoHoras = 4

	// As chances do Ciclope Tirano, em % por morte, cada uma sorteada à parte:
	// as barras são a mais fácil, depois a Arma D, e os dois ovos iguais.
	tiranoBarrasPct = 80
	tiranoArmaDPct  = 60
	tiranoOvoPct    = 10

	// tiranoBarras é quantas Barras de Prata (10Mi) saem juntas.
	tiranoBarras = 3

	itemOvoCavaloLeveN = 2308
	itemOvoCavaloLeveB = 2313
)

// ciclopesDoSpot são as cópias de template que só nascem no spot. O Ciclope Cruel
// e o Lanceiro Zakum do resto do mapa não recebem as Armas C.
var ciclopesDoSpot = map[string]bool{
	droprule.Canonical(ciclopes.CiclopeCruelSpot):  true,
	droprule.Canonical(ciclopes.LanceiroZakumSpot): true,
}

// Os adds pedidos, nos degraus do bônus de drop (dano de 9 em 9, magia de 4 em 4):
// dano 45 a 72 e magia 20 a 32. Quanto maior mais raro, como nas outras escadas;
// os pesos são escolha daqui, o pedido deu só os valores.
var (
	addCiclopeFisica = []addArma{
		{40, efDamage, 45},
		{30, efDamage, 54},
		{20, efDamage, 63},
		{10, efDamage, 72},
	}
	addCiclopeMagica = []addArma{
		{40, efMagic, 20},
		{30, efMagic, 24},
		{20, efMagic, 28},
		{10, efMagic, 32},
	}
)

// ciclopesFinish carimba o add de uma Arma C que um monstro do spot soltou pela
// Mesa, depois do bônus de drop comum.
func (d *Dispatcher) ciclopesFinish(w *world.World, mob *world.Entity, it *world.Item) {
	if !ciclopesDoSpot[droprule.Canonical(mob.TemplateName)] {
		return
	}
	carimbaAddArmaC(w, it, addCiclopeFisica, addCiclopeMagica)
}

// isCiclopeTirano diz se o monstro é o Ciclope Tirano, pelo nome do arquivo do
// template, como a Mesa: um "/gm criar Ciclope_Tirano" também paga.
func isCiclopeTirano(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(ciclopes.Chefe)
}

// geradorDoCiclopeTirano diz se o bloco idx é o do Ciclope Tirano.
func geradorDoCiclopeTirano(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(ciclopes.Chefe)
}

// ciclopeTiranoSaque entrega o saque do Ciclope Tirano na bolsa de quem mata. São
// quatro sorteios independentes, sempre nesta ordem: as três Barras de Prata
// (10Mi), UMA Arma D das oito do Troll Enigma com o add do spot, o Ovo de Cavalo
// Leve N e o B. Uma regra da Mesa para um desses itens neste monstro o governa,
// e aí aquela parte não sai daqui.
func (d *Dispatcher) ciclopeTiranoSaque(w *world.World, reward, mob *world.Entity) {
	if !isCiclopeTirano(mob) {
		return
	}
	if w.Rand().Intn(100) < tiranoBarrasPct && !d.dropRules.Governs(mob.TemplateName, itemBarraPrata10Mi) {
		it := world.Item{Index: itemBarraPrata10Mi}
		if isSplittable(it.Index) {
			setItemAmount(&it, tiranoBarras)
			d.putMobDrop(w, reward, it)
		} else {
			for range tiranoBarras {
				d.putMobDrop(w, reward, it)
			}
		}
	}
	if w.Rand().Intn(100) < tiranoArmaDPct {
		d.entregaArmaDDoTirano(w, reward, mob)
	}
	for _, ovo := range []int16{itemOvoCavaloLeveN, itemOvoCavaloLeveB} {
		if w.Rand().Intn(100) < tiranoOvoPct && !d.dropRules.Governs(mob.TemplateName, ovo) {
			d.putMobDrop(w, reward, world.Item{Index: ovo})
		}
	}
}

// armasDDoTirano são as oito Armas D do Troll Enigma (acampamento_troll.go), em
// ordem fixa para o sorteio ser reproduzível.
var armasDDoTirano = []int16{869, 809, 910, 824, 935, 899, 854, 902}

// entregaArmaDDoTirano entrega UMA Arma D, sorteada por igual entre as oito, com o
// add de dano (físicas) ou de magia (lanças e cajados) da escada do spot.
func (d *Dispatcher) entregaArmaDDoTirano(w *world.World, reward, mob *world.Entity) {
	arma := armasDDoTirano[w.Rand().Intn(len(armasDDoTirano))]
	if d.dropRules.Governs(mob.TemplateName, arma) {
		return
	}
	tabela := addCiclopeFisica
	if armasTrollMagicas[arma] {
		tabela = addCiclopeMagica
	}
	linha := sortearAddArma(w, tabela)
	it := world.Item{Index: arma}
	it.Effects[0] = world.Effect{Effect: efSanc, Value: 0}
	it.Effects[1] = world.Effect{Effect: linha.efeito, Value: uint8(linha.valor)}
	d.putMobDrop(w, reward, it)
}

// ApplyCiclopeTiranoBoot segura o Ciclope Tirano por ciclopeTiranoHoras depois que
// o servidor sobe, como o Boss Hidra Dourada: o boot povoa todo bloco de uma vez,
// e sem isto um reinício valeria um chefe.
func (d *Dispatcher) ApplyCiclopeTiranoBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDoCiclopeTirano(w, idx) && w.DeferGenerator(idx, ciclopeTiranoHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("Ciclope Tirano nasce horas depois do boot", "horas", ciclopeTiranoHoras, "blocos", blocos)
}
