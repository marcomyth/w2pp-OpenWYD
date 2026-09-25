package handler

import (
	"slices"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O spot de Caveira Lanc e Conj Caveira do 1º andar da Dungeon (x 300 a 490, y
// 3730 a 3880), pedido do Marco em 25/09/2026 com prints em (353,3756):
//
//   - os blocos dos dois passam a MaxNumMob x5 no NPCGener.txt (Caveira Lanc 45 ->
//     225, Conj Caveira 30 -> 150), e os de MinuteGenerate -1 passam a 1;
//   - os dois soltam Resto de Oriharucon e de Lactolerium e as Armas C, pela Mesa
//     (migração 0144); o add de cada arma sai daqui (caveirasFinish);
//   - um mini chefe, o Boss Conjurador (bloco 6149): o Conj Caveira com os números
//     do Troll Enigma, montado num Dragão Menor de nível 100 e com a Foice
//     Esqueleto +11. Volta 3 horas depois da morte e, como os chefes da lava
//     (dungeon_lava.go), some depois de 30 minutos sem luta e volta 3 horas depois
//     de ter nascido. Solta UMA Arma C por morte, com o add alto.
const (
	bossConjuradorTemplate = "Boss_Conjurador"
	// conjuradorHoras é a espera entre a morte e a volta, a espera depois do boot
	// e o ciclo de quem some sem luta.
	conjuradorHoras = 3
)

// caveirasDoSpot são os dois monstros do spot. A Mesa vale por template, e os
// dois só nascem no 1º andar da Dungeon — fora dois Conj Caveira seguidores de
// Elfo Negro (blocos 2097 e 2098), que levam o mesmo saque.
var caveirasDoSpot = map[string]bool{
	droprule.Canonical("Caveira_Lanc"): true,
	droprule.Canonical("Conj_Caveira"): true,
}

// As Armas C (EF_ITEMLEVEL 3), as duas de cada família. Físicas levam dano; as
// lanças e os cajados levam magia, como o bônus de drop do legado as separa
// (refine/dropbonus.go, nUnique 44 e 47) e como as Armas D do Acampamento Troll.
var (
	armasCFisicas = []int16{
		807, 808, // Maça Gótica, Martelo Mythril
		822, 823, // Arco Mythril, Arco Hidra
		837, 838, // Lança Invisível, Faca do Assassino
		867, 868, // Gladio, Lâmina Espiritual
		882, 883, // Shamir, Faca Astaroth
		908, 909, // Espada Larga, Lâmina Dupla
		933, 934, // Machado de Batalha, Grande Machado
	}
	armasCMagicas = []int16{
		852, 853, // Lança da Corrupção, Longinius
		897, 898, // Cajado do Santo, Varinha Alada
		901, // Cajado da Jóia Azul
	}
)

// Os adds pedidos, nos degraus do bônus de drop (dano de 9 em 9, magia de 4 em
// 4). Quanto maior o add mais raro, como nas outras escadas; os pesos são escolha
// daqui, o pedido não os deu.
//
//	monstros   física 45 · 54 · 63     mágica 20 · 24 · 28
//	chefe      física 72 · 81          mágica 32 · 36
var (
	addCaveiraFisica = []addArma{
		{60, efDamage, 45},
		{30, efDamage, 54},
		{10, efDamage, 63},
	}
	addCaveiraMagica = []addArma{
		{60, efMagic, 20},
		{30, efMagic, 24},
		{10, efMagic, 28},
	}
	addConjuradorFisica = []addArma{
		{60, efDamage, 72},
		{40, efDamage, 81},
	}
	addConjuradorMagica = []addArma{
		{60, efMagic, 32},
		{40, efMagic, 36},
	}
)

// carimbaAddArmaC põe o add de uma das duas tabelas numa Arma C, e diz se pôs.
//
// O slot 0 fica com o refino que o bônus de drop sorteou (+0 a +2); se o bônus pôs
// ali outra coisa, a arma sai +0, refinável. O slot 1 é o add sorteado e o 2 sai
// vazio: o add é o pedido, sem o segundo add aleatório do legado por cima.
func carimbaAddArmaC(w *world.World, it *world.Item, fisica, magica []addArma) bool {
	var tabela []addArma
	switch {
	case slices.Contains(armasCFisicas, it.Index):
		tabela = fisica
	case slices.Contains(armasCMagicas, it.Index):
		tabela = magica
	default:
		return false
	}
	linha := sortearAddArma(w, tabela)
	if it.Effects[0].Effect != efSanc {
		it.Effects[0] = world.Effect{Effect: efSanc, Value: 0}
	}
	it.Effects[1] = world.Effect{Effect: linha.efeito, Value: uint8(linha.valor)}
	it.Effects[2] = world.Effect{}
	return true
}

// caveirasFinish carimba o add de uma Arma C que um monstro do spot soltou pela
// Mesa, depois do bônus de drop comum.
func (d *Dispatcher) caveirasFinish(w *world.World, mob *world.Entity, it *world.Item) {
	if !caveirasDoSpot[droprule.Canonical(mob.TemplateName)] {
		return
	}
	carimbaAddArmaC(w, it, addCaveiraFisica, addCaveiraMagica)
}

// isBossConjurador diz se o monstro é o Boss Conjurador, pelo nome do arquivo do
// template, como a Mesa: um "/gm criar Boss_Conjurador" também paga.
func isBossConjurador(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(bossConjuradorTemplate)
}

// geradorDoBossConjurador diz se o bloco idx é o do Boss Conjurador.
func geradorDoBossConjurador(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(bossConjuradorTemplate)
}

// bossConjuradorSaque entrega UMA Arma C por morte, na bolsa de quem mata: a arma
// sai por igual entre as dezenove, com o add alto. A Mesa pode tirar uma delas do
// chefe (uma regra para o item o governa), e aí aquela morte não solta nada.
func (d *Dispatcher) bossConjuradorSaque(w *world.World, reward, mob *world.Entity) {
	if !isBossConjurador(mob) {
		return
	}
	d.entregaArmaCDeChefe(w, reward, mob)
}

// entregaArmaCDeChefe entrega UMA Arma C, sorteada por igual entre as dezenove,
// com o add alto do Boss Conjurador. A Gárgula Sábio chefe (gargula_sabio.go) usa
// a mesma. Uma regra da Mesa para a arma sorteada, neste monstro, a governa: aí
// aquela morte não solta nada.
func (d *Dispatcher) entregaArmaCDeChefe(w *world.World, reward, mob *world.Entity) {
	// 32768 % 19 = 12: as doze primeiras saem uma vez a mais em 1.724.
	armas := slices.Concat(armasCFisicas, armasCMagicas)
	arma := armas[w.Rand().Intn(len(armas))]
	if d.dropRules.Governs(mob.TemplateName, arma) {
		return
	}
	it := world.Item{Index: arma}
	carimbaAddArmaC(w, &it, addConjuradorFisica, addConjuradorMagica)
	d.putMobDrop(w, reward, it)
}

// ApplyBossConjuradorBoot segura o Boss Conjurador por conjuradorHoras depois que
// o servidor sobe, como os chefes da lava (ApplyChefesDaLavaBoot): o boot povoa
// todo bloco de uma vez, e sem isto um reinício valeria um chefe.
func (d *Dispatcher) ApplyBossConjuradorBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDoBossConjurador(w, idx) && w.DeferGenerator(idx, conjuradorHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("Boss Conjurador nasce horas depois do boot", "horas", conjuradorHoras, "blocos", blocos)
}
