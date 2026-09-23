package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Gelo é a caixa Karden do Regions.txt (TestGeloCaixaBateComRegions confere):
// Nippleheim, a Vila Amald e a tropa em volta. A mesa dos monstros comuns é a
// migração 0110; os chefes pagam aqui, pedido da equipe em 23/09/2026.
const (
	geloMinX, geloMinY = 3391, 2649
	geloMaxX, geloMaxY = 4027, 3255
)

// nasceuNoGelo diz se o monstro nasceu dentro do Gelo. A Sombra Negra usa o
// mesmo template em outros lugares do mapa, e só a do Gelo paga o saque daqui.
func nasceuNoGelo(mob *world.Entity) bool {
	x, y := int(mob.SpawnX), int(mob.SpawnY)
	return x >= geloMinX && x <= geloMaxX && y >= geloMinY && y <= geloMaxY
}

// Os chefes do Gelo — as duas Sombras Negras e os três Verid — soltam UMA coisa
// por morte, sorteada: um pacote de 20 âmagos do Cavalo Equipado B, o ovo dele,
// o Fragmento de Alma, a Barra de Prata de 50Mi ou a RCoin de 100, em partes
// iguais, e a Alma a 1% — a da Fênix na Sombra, a do Unicórnio no Verid.
//
// O template de cada um continua soltando o que a 0110 deixou nele; isto vem a
// mais. E não passa pela Mesa de Drops: o "*" a 0% da 0074 tira as Almas do
// mundo para segurar o Arch nos Reis, e a Alma daqui é a exceção pedida.
// As Almas e o Fragmento são as constantes da Praça dos Reinos (reinos_praca.go).
const (
	itemRCoin100 int16 = 3393

	// geloChefeBase é a base do sorteio. 32768 % 500 = 268: os valores de 0 a 267
	// saem uma vez a mais que os outros. A Alma fica no FIM da faixa, onde cada
	// valor sai 65 vezes, e paga 0,99% — nunca mais que o 1% pedido.
	geloChefeBase = 500
)

type geloChefePremio struct {
	nome       string
	peso       int
	item       int16 // 0 = a Alma do chefe (geloChefeAlma)
	quantidade int
}

// geloChefePremios soma geloChefeBase, e a Alma vem por último.
var geloChefePremios = []geloChefePremio{
	{"Pacote de Cavalo Equipado B", 99, 2404, 20},
	{"Ovo de Cavalo Equipado B", 99, 2314, 1},
	{"Fragmento de Alma", 99, itemFragmentoDeAlma, 1},
	{"Barra de Prata (50Mi)", 99, itemBarraPrata50Mi, 1},
	{"RCoin (100)", 99, itemRCoin100, 1},
	{"Alma", 5, 0, 1},
}

// geloChefeAlma devolve a Alma que o chefe paga, ou 0 se ele não é chefe do Gelo.
func geloChefeAlma(mob *world.Entity) int16 {
	switch droprule.Canonical(mob.TemplateName) {
	case droprule.Canonical("Sombra_Negra"), droprule.Canonical("Sombra_Negra_"):
		return itemAlmaDaFenix
	case droprule.Canonical("Verid"), droprule.Canonical("Verid_"):
		return itemAlmaDoUnicornio
	}
	return 0
}

// geloChefeSorteia devolve o prêmio que um sorteio r em [0, geloChefeBase) escolhe.
func geloChefeSorteia(r int) geloChefePremio {
	for _, p := range geloChefePremios {
		if r < p.peso {
			return p
		}
		r -= p.peso
	}
	return geloChefePremios[len(geloChefePremios)-1]
}

// geloChefeSaque entrega o prêmio da morte de um chefe do Gelo na bolsa de quem
// mata, pela mesma porta do resto do saque. Um sorteio só por morte.
func (d *Dispatcher) geloChefeSaque(w *world.World, reward, mob *world.Entity) {
	alma := geloChefeAlma(mob)
	if alma == 0 || !nasceuNoGelo(mob) {
		return
	}
	p := geloChefeSorteia(w.Rand().Intn(geloChefeBase))
	item := p.item
	if item == 0 {
		item = alma
	}
	it := world.Item{Index: item}
	if isSplittable(it.Index) {
		setItemAmount(&it, p.quantidade)
	}
	d.putMobDrop(w, reward, it)
}

// geloChefeHoras é a espera entre a morte de um chefe do Gelo e a volta dele,
// pedido da equipe em 23/09 (esperaDoRenascimento). No boot eles nascem como
// todo bloco sem período.
const geloChefeHoras = 4

// geradorDeChefeDoGelo diz se o bloco idx é de um chefe do Gelo — Sombra Negra ou
// Verid — com a partida dentro da caixa. O mesmo template fora dela (a Sombra de
// outros mapas, o Verid do Coliseu) segue as horas de chefe do painel.
func geradorDeChefeDoGelo(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	if g == nil || geloChefeAlma(&world.Entity{TemplateName: g.LeaderName}) == 0 {
		return false
	}
	x, y := int(g.SegX[0]), int(g.SegY[0])
	return x >= geloMinX && x <= geloMaxX && y >= geloMinY && y <= geloMaxY
}
