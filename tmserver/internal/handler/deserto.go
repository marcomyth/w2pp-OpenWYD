package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os dois Agmo do Deserto (Tauron_Agmo e Verme_Agmo) são monstros de evento: os
// blocos deles (3451, 3452) nascem desligados desde a migração 0108, e a equipe
// os cria à mão num evento. Cada morte solta UM âmago N, um ou outro, sorteado
// com os pesos que a equipe deu (23/09/2026): Sem Sela 40, Fantasma 30, Cavalo
// Leve 20, Cavalo Equipado 10.
//
// Isto mora no código e não na Mesa de Drops porque a Mesa rola cada item por
// conta própria: quatro regras somando 100% dariam zero âmago numa morte e três
// na outra, e o pedido é exatamente um.
var agmoAmagos = []struct {
	item int16
	peso int
}{
	{2396, 40}, // Âmago de Cav s/Sela N
	{2397, 30}, // Âmago de Cav Fantasm N
	{2398, 20}, // Âmago de Cavalo Leve N
	{2399, 10}, // Âmago de Cavalo Equip N
}

// agmoPesoTotal é a base do sorteio. Base 100 gasta uma chamada de rand(), com o
// viés do legado desprezível (32768 % 100 = 68).
const agmoPesoTotal = 100

// isAgmo diz se o monstro é um dos dois Agmo, pelo nome do ARQUIVO do template,
// como a Mesa: o "/gm criar Tauron_Agmo" do evento é reconhecido.
func isAgmo(mob *world.Entity) bool {
	switch droprule.Canonical(mob.TemplateName) {
	case droprule.Canonical("Tauron_Agmo"), droprule.Canonical("Verme_Agmo"):
		return true
	}
	return false
}

// agmoSorteia devolve o âmago que um sorteio r em [0, agmoPesoTotal) escolhe.
func agmoSorteia(r int) int16 {
	for _, a := range agmoAmagos {
		if r < a.peso {
			return a.item
		}
		r -= a.peso
	}
	return agmoAmagos[len(agmoAmagos)-1].item
}

// agmoAmago entrega o âmago da morte de um Agmo na bolsa de quem mata, pela
// mesma porta do resto do saque. Se a Mesa de Drops tem regra para o âmago
// sorteado neste monstro (ou um "*" a 0% que o tire do mundo), vale a Mesa.
func (d *Dispatcher) agmoAmago(w *world.World, reward, mob *world.Entity) {
	if !isAgmo(mob) {
		return
	}
	item := agmoSorteia(w.Rand().Intn(agmoPesoTotal))
	if d.dropRules.Governs(mob.TemplateName, item) {
		return
	}
	it := world.Item{Index: item}
	if isSplittable(it.Index) {
		setItemAmount(&it, 1)
	}
	d.putMobDrop(w, reward, it)
}
