package handler

import (
	"slices"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Taron Tirano (pedido do Marco em 28/09/2026): o chefe dos Taron Assassino do
// Deserto Baixo, no bloco 6165, em (1470,1724). É o corpo do Taron Assassino com
// os números do Ciclope Tirano e vida e dano em dobro — 3,6 milhões de vida e
// 3.636 de dano, sem divisor —, e volta de 4 em 4 horas, como ele.
//
// O saque segue o estilo do Ciclope Tirano, com as mesmas chances: as três Barras
// de Prata (10Mi), UMA Arma D com o add do spot e os dois Ovos de Cavalo Leve.
// A Arma D sai das dezesseis do Deserto (deserto_armas.go), e não das oito do
// Troll Enigma: é o chefe deles, e o Martelo Dragão e a Gram entram no sorteio.
const (
	taronTiranoMolde = "Taron_Tirano"
	// taronTiranoHoras é a espera entre a morte e a volta, e depois do boot.
	taronTiranoHoras = 4
)

// armasDDoTaronTirano são as dezesseis Armas D do Deserto em ordem fixa, para o
// sorteio ser reproduzível.
var armasDDoTaronTirano = func() []int16 {
	var out []int16
	for id := range armasDDoDesertoFisicas {
		out = append(out, id)
	}
	for id := range armasDDoDesertoMagicas {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}()

// isTaronTirano diz se o monstro é o Taron Tirano, pelo nome do arquivo do
// template, como a Mesa: um "/gm criar Taron_Tirano" também paga.
func isTaronTirano(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(taronTiranoMolde)
}

// geradorDoTaronTirano diz se o bloco idx é o do Taron Tirano.
func geradorDoTaronTirano(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(taronTiranoMolde)
}

// taronTiranoSaque entrega o saque do Taron Tirano na bolsa de quem mata: quatro
// sorteios independentes, sempre nesta ordem — as três Barras de Prata (10Mi),
// UMA Arma D do Deserto com o add do spot, o Ovo de Cavalo Leve N e o B. Uma
// regra da Mesa para um desses itens neste monstro o governa, e aí aquela parte
// não sai daqui.
func (d *Dispatcher) taronTiranoSaque(w *world.World, reward, mob *world.Entity) {
	if !isTaronTirano(mob) {
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
		arma := armasDDoTaronTirano[w.Rand().Intn(len(armasDDoTaronTirano))]
		if !d.dropRules.Governs(mob.TemplateName, arma) {
			it := world.Item{Index: arma}
			carimbaAddArmaD(w, &it, addCiclopeFisica, addCiclopeMagica)
			d.putMobDrop(w, reward, it)
		}
	}
	for _, ovo := range []int16{itemOvoCavaloLeveN, itemOvoCavaloLeveB} {
		if w.Rand().Intn(100) < tiranoOvoPct && !d.dropRules.Governs(mob.TemplateName, ovo) {
			d.putMobDrop(w, reward, world.Item{Index: ovo})
		}
	}
}

// ApplyTaronTiranoBoot segura o Taron Tirano por taronTiranoHoras depois que o
// servidor sobe, como o Ciclope Tirano: o boot povoa todo bloco de uma vez, e sem
// isto um reinício valeria um chefe.
func (d *Dispatcher) ApplyTaronTiranoBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDoTaronTirano(w, idx) && w.DeferGenerator(idx, taronTiranoHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("Taron Tirano nasce horas depois do boot", "horas", taronTiranoHoras, "blocos", blocos)
}
