package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Boss Dragão Lich (template Boss_Dragao_Lich, bloco 6146 do NPCGener) é o
// chefe do 3º andar da Dungeon, pedido da equipe em 25/09/2026 no molde do Boss
// Mantícora: o corpo do Dragão Lich, 1,8x maior (CON 2000), com o nível, a
// defesa e o divisor de dano do Cav. Lugefer (÷250) e a vida e o dano do Boss
// Mantícora. Nasce no salão de cima dos Dragões Lich e volta de 4 em 4 horas,
// como os chefes do Gelo e o FrenzyDemonLord, e fica 4 horas de fora depois do
// boot.
//
// A Pedra de Dragão Lich sai do Dragão comum na 0140 e passa a ser deste chefe,
// pela Mesa de Drops a 0,99% por morte, a régua das outras Pedras Arch (0136).
// Além dela, cada morte solta UMA coisa, sorteada entre os quatro prêmios do
// Boss Mantícora sem a pedra: a Barra de Prata de 50Mi ou um pacote de âmagos
// (20 Cavalo Equipado, 40 Cavalo Leve ou 60 Fantasma), N ou B em metade das
// vezes.
const (
	bossDragaoLichTemplate = "Boss_Dragao_Lich"
	// bossDragaoLichHoras é a espera entre a morte e a volta
	// (esperaDoRenascimento), e a espera depois do boot (ApplyBossDragaoLichBoot).
	bossDragaoLichHoras = 4

	itemPedraDeDragaoLich = 1754

	// bossDragaoLichBase é a base do sorteio: os quatro pesos de 45.
	bossDragaoLichBase = 180
)

// bossDragaoLichPremios são os prêmios do Boss Mantícora, menos a pedra, que
// neste chefe é a regra da Mesa.
var bossDragaoLichPremios = bossManticoraPremios[:len(bossManticoraPremios)-1]

// isBossDragaoLich diz se o monstro é o Boss Dragão Lich, pelo nome do arquivo
// do template, como a Mesa: um "/gm criar Boss_Dragao_Lich" também paga.
func isBossDragaoLich(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(bossDragaoLichTemplate)
}

// geradorDoBossDragaoLich diz se o bloco idx é o do Boss Dragão Lich.
func geradorDoBossDragaoLich(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(bossDragaoLichTemplate)
}

// bossDragaoLichSaque entrega o prêmio da morte do Boss Dragão Lich na bolsa de
// quem mata (entregaPremioDeChefe). A pedra não passa por aqui: é a Mesa.
func (d *Dispatcher) bossDragaoLichSaque(w *world.World, reward, mob *world.Entity) {
	if !isBossDragaoLich(mob) {
		return
	}
	d.entregaPremioDeChefe(w, reward, mob, bossDragaoLichPremios, bossDragaoLichBase)
}

// ApplyBossDragaoLichBoot segura o Boss Dragão Lich por bossDragaoLichHoras
// depois que o servidor sobe, pelo mesmo motivo do Gelo (ApplyGeloChefesBoot):
// o boot povoa todo bloco de uma vez, e sem isto um reinício valeria um chefe.
func (d *Dispatcher) ApplyBossDragaoLichBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDoBossDragaoLich(w, idx) && w.DeferGenerator(idx, bossDragaoLichHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("Boss Dragão Lich nasce horas depois do boot", "horas", bossDragaoLichHoras, "blocos", blocos)
}

// O Boss Hidra Dourada (template Boss_Hidra_Dourada, bloco 6161 do NPCGener) é o
// chefe do 2º andar da Dungeon, pedido do Marco em 25/09/2026 no molde do Boss
// Dragão Lich: o corpo da Hidra Dourada, maior (CON 2000), com os números do Boss
// Mantícora — nível, defesa, vida (64.000 ÷250 = 16 mi reais) e dano 6.000 —, a
// XP no teto de 10.000 da Dungeon (0091) e o Carry vazio. Nasce em (740,3776),
// onde a Hidra comum liderava cinco Guer Caveira; o bloco 2099 ficou só com a
// escolta, o Guer_Caveira_Escolta, com 6x a vida e o dano do comum. Volta de 4 em
// 4 horas e fica 4 horas de fora depois do boot.
//
// O saque é o do Boss Mantícora trocando a pedra: a Pedra do Esqueleto pela Mesa
// a 0,99% por morte (0149), a régua das Pedras Arch, e UM dos quatro prêmios do
// código a 25% cada (bossDragaoLichPremios).
const (
	bossHidraDouradaTemplate = "Boss_Hidra_Dourada"
	// bossHidraDouradaHoras é a espera entre a morte e a volta
	// (esperaDoRenascimento), e a espera depois do boot (ApplyBossHidraDouradaBoot).
	bossHidraDouradaHoras = 4

	itemPedraDoEsqueleto = 1753
)

// isBossHidraDourada diz se o monstro é o Boss Hidra Dourada, pelo nome do
// arquivo do template, como a Mesa: um "/gm criar Boss_Hidra_Dourada" também paga.
func isBossHidraDourada(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(bossHidraDouradaTemplate)
}

// geradorDoBossHidraDourada diz se o bloco idx é o do Boss Hidra Dourada.
func geradorDoBossHidraDourada(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(bossHidraDouradaTemplate)
}

// bossHidraDouradaSaque entrega o prêmio da morte do Boss Hidra Dourada na bolsa
// de quem mata (entregaPremioDeChefe). A pedra não passa por aqui: é a Mesa.
func (d *Dispatcher) bossHidraDouradaSaque(w *world.World, reward, mob *world.Entity) {
	if !isBossHidraDourada(mob) {
		return
	}
	d.entregaPremioDeChefe(w, reward, mob, bossDragaoLichPremios, bossDragaoLichBase)
}

// ApplyBossHidraDouradaBoot segura o Boss Hidra Dourada por bossHidraDouradaHoras
// depois que o servidor sobe, como o Boss Dragão Lich (ApplyBossDragaoLichBoot).
func (d *Dispatcher) ApplyBossHidraDouradaBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDoBossHidraDourada(w, idx) && w.DeferGenerator(idx, bossHidraDouradaHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("Boss Hidra Dourada nasce horas depois do boot", "horas", bossHidraDouradaHoras, "blocos", blocos)
}
