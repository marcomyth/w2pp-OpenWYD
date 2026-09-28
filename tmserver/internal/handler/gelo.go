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
// As Almas e o Fragmento são as constantes da Praça dos Reinos (reinos_praca.go);
// a RCoin (itemRCoin100) mora em rcoin.go, com as outras quatro.
const (
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
// pedido da equipe em 23/09 (esperaDoRenascimento), e desde 24/09 também a
// espera depois de o servidor subir (ApplyGeloChefesBoot).
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

// ApplyGeloChefesBoot segura os chefes do Gelo por geloChefeHoras depois que o
// servidor sobe, pedido da equipe em 24/09/2026. O boot povoa todo bloco de uma
// vez (spawnNPCs), e sem isto a Sombra e o Verid estariam de pé no primeiro
// segundo de cada reinício — um reinício valeria um chefe.
//
// Eles entram na mesma fila de quem morre (world.DeferGenerator), então voltam
// como voltariam de uma morte, e ficam até alguém matá-los: bloco sem período de
// minuto não é reposto nem esvaziado pelo relógio. Roda depois do populate, do
// overlay de NPC e dos blocos desligados, para adiar só o que ficou de pé.
func (d *Dispatcher) ApplyGeloChefesBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDeChefeDoGelo(w, idx) && w.DeferGenerator(idx, geloChefeHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("chefes do Gelo nascem horas depois do boot", "horas", geloChefeHoras, "blocos", blocos)
}

// Os macacos fortes do Gelo — Soldado e Guerreiro Amon — soltam o topo das Armas
// D (0128), pedido da equipe em 24/09/2026, com add aleatório e, em 10% delas,
// um add alto: 45-54 de dano ou 20-24 de magia. Físicas levam dano; a Lança do
// Triunfo e a Fúria Divina (nUnique 44 e 47) levam magia, como o bônus de drop do
// legado as separa (refine/dropbonus.go).
var (
	geloAmon = map[string]bool{
		droprule.Canonical("Soldado_Amon"):   true,
		droprule.Canonical("Guerreiro_Amon"): true,
	}
	armasAmonFisicas = map[int16]bool{
		810: true, // Martelo Assassino
		825: true, // Arco Divino
		840: true, // Garra Draconiana
		870: true, // Espada Vorpal
		885: true, // Cruz Sagrada
		911: true, // Solaris
		936: true, // Mjolnir
	}
	armasAmonMagicas = map[int16]bool{
		855: true, // Lança do Triunfo
		900: true, // Fúria Divina
	}
)

// O add alto dos Amon, pedido da equipe em 24/09/2026: 10% das armas saem com ele,
// e as outras 90% ficam com o add aleatório do bônus de drop do legado, sem mexer.
// Ele para um degrau acima do teto do legado — o SetItemBonus nunca passa de 45
// de dano nem de 20 de magia numa arma, e 54 e 24 são o degrau que o pedido abre.
// Dentro do add alto, o maior sai em 40% (escolha daqui; a equipe não deu): 4%
// das armas no total.
//
//	física   45 · 54
//	mágica   20 · 24
const geloAmonAddAltoPct = 10

var (
	addAmonFisica = []addArma{
		{60, efDamage, 45},
		{40, efDamage, 54},
	}
	addAmonMagica = []addArma{
		{60, efMagic, 20},
		{40, efMagic, 24},
	}
)

// geloAmonFinish dá o add alto a uma parte das Armas D que um Amon do Gelo
// soltou, depois do bônus de drop comum; as outras passam como o bônus as deixou.
//
// Na que ganha, o slot 1 é o add alto. O slot 0 fica com o refino que o bônus
// sorteou (+0 a +2); se o bônus pôs ali outra coisa — o bônus especial, que pode
// ser dano —, a arma sai +0, refinável. O slot 2 guarda o segundo add aleatório do
// legado (velocidade, skill…), a não ser que seja o mesmo atributo do add alto:
// dano sobre dano passaria do teto pedido.
func (d *Dispatcher) geloAmonFinish(w *world.World, mob *world.Entity, it *world.Item) {
	if !geloAmon[droprule.Canonical(mob.TemplateName)] || !nasceuNoGelo(mob) {
		return
	}
	var tabela []addArma
	switch {
	case armasAmonFisicas[it.Index]:
		tabela = addAmonFisica
	case armasAmonMagicas[it.Index]:
		tabela = addAmonMagica
	default:
		return
	}
	// 32768 % 100 = 68: os valores de 0 a 67 saem uma vez a mais, e os dez
	// primeiros pagam 10,01%.
	if w.Rand().Intn(100) >= geloAmonAddAltoPct {
		return
	}
	linha := sortearAddArma(w, tabela)
	if it.Effects[0].Effect != efSanc {
		it.Effects[0] = world.Effect{Effect: efSanc, Value: 0}
	}
	it.Effects[1] = world.Effect{Effect: linha.efeito, Value: uint8(linha.valor)}
	if it.Effects[2].Effect == linha.efeito {
		it.Effects[2] = world.Effect{}
	}
}
