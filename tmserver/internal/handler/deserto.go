package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os dois Agmo do Deserto (Tauron_Agmo e Verme_Agmo) são monstros de evento: os
// blocos deles (3451, 3452) nascem desligados desde a migração 0109, e a equipe
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

// O Boss Mantícora (template Boss_Manticora, bloco 6145 do NPCGener) é o chefe
// do Deserto_Manticora, pedido da equipe em 23/09/2026: o corpo da Mantícora com
// o nível e o divisor de dano do Cav. Lugefer (÷250) e o dobro da vida dele.
//
// Cada morte solta UMA coisa, sorteada: a Pedra de Mantícora a 10%, ou, nos 90%
// restantes, em partes iguais, a Barra de Prata de 50Mi ou um pacote de âmagos
// (20 Cavalo Equipado, 40 Cavalo Leve ou 60 Fantasma), cada pacote N ou B em
// metade das vezes. A pedra saiu da Mantícora comum na 0109: é deste chefe.
const (
	bossManticoraTemplate = "Boss_Manticora"
	// bossManticoraHoras é a espera entre a morte e a volta (esperaDoRenascimento).
	bossManticoraHoras = 5

	itemPedraDeManticora = 1756

	// bossManticoraBase é a base do sorteio. 32768 % 200 = 168: os valores de 0 a
	// 167 saem uma vez a mais que os outros. A pedra fica no FIM da faixa, onde
	// cada valor sai 163 vezes, e paga 9,95% — nunca mais que os 10% pedidos.
	bossManticoraBase = 200
)

type bossManticoraPremio struct {
	nome       string
	peso       int
	itemN      int16 // o item (ou o âmago N)
	itemB      int16 // o âmago B; 0 quando não há versão B
	quantidade int
}

// bossManticoraPremios soma bossManticoraBase, e a pedra vem por último.
var bossManticoraPremios = []bossManticoraPremio{
	{"Barra de Prata (50Mi)", 45, itemBarraPrata50Mi, 0, 1},
	{"Pacote de Cavalo Equipado", 45, 2399, 2404, 20},
	{"Pacote de Cavalo Leve", 45, 2398, 2403, 40},
	{"Pacote de Cavalo Fantasma", 45, 2397, 2402, 60},
	{"Pedra de Mantícora", 20, itemPedraDeManticora, 0, 1},
}

// isBossManticora diz se o monstro é o Boss Mantícora, pelo nome do arquivo do
// template, como a Mesa: um "/gm criar Boss_Manticora" também paga.
func isBossManticora(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(bossManticoraTemplate)
}

// geradorDoBossManticora diz se o bloco idx é o do Boss Mantícora.
func geradorDoBossManticora(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(bossManticoraTemplate)
}

// bossManticoraSorteia devolve o prêmio que um sorteio r em [0, bossManticoraBase)
// escolhe.
func bossManticoraSorteia(r int) bossManticoraPremio {
	for _, p := range bossManticoraPremios {
		if r < p.peso {
			return p
		}
		r -= p.peso
	}
	return bossManticoraPremios[len(bossManticoraPremios)-1]
}

// bossManticoraSaque entrega o prêmio da morte do Boss Mantícora na bolsa de quem
// mata. Dois sorteios, sempre na mesma ordem: o prêmio, e N ou B para o pacote.
// Se a Mesa de Drops tem regra para o item sorteado neste monstro (ou um "*" a
// 0% que o tire do mundo), vale a Mesa.
func (d *Dispatcher) bossManticoraSaque(w *world.World, reward, mob *world.Entity) {
	if !isBossManticora(mob) {
		return
	}
	p := bossManticoraSorteia(w.Rand().Intn(bossManticoraBase))
	item := p.itemN
	if p.itemB != 0 && w.Rand().Intn(2) == 1 {
		item = p.itemB
	}
	if d.dropRules.Governs(mob.TemplateName, item) {
		return
	}
	it := world.Item{Index: item}
	if isSplittable(it.Index) {
		setItemAmount(&it, p.quantidade)
	}
	d.putMobDrop(w, reward, it)
}
