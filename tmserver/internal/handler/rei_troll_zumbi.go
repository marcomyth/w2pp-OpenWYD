package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Rei Troll Zumbi (template Rei_Troll_Zumbi, bloco 6150 do NPCGener) é o mini
// chefe do 1º andar da Dungeon, pedido do Marco em 25/09/2026: nasce no salão do
// esqueleto de dragão, em (284,3753), volta de hora em hora e é para novato
// derrubar. O corpo é o do Troll Zumbi, maior (CON 1200), e os números são os
// dele medidos em Trolls: 12 de vida (66.000), dano 250 contra 193, defesa 1.000
// contra 900, nível 115 contra 102. Quem caça ali derruba um Troll em alguns
// segundos e o Rei em doze vezes isso. XP no teto de 10.000 da Dungeon (0091).
//
// Cada morte solta o pacote inteiro, sem sorteio, na bolsa de quem mata: a Moeda
// de 5 milhões, 5 Poeiras de Oriharucon, 3 de Lactolerium e 20 Âmagos de Dente
// de Sabre. O Carry do template é vazio, então é só isto que ele solta.
//
// Fica 1 h de fora depois do boot, pelo mesmo motivo do Boss Dragão Lich
// (ApplyBossDragaoLichBoot): o boot povoa todo bloco de uma vez, e sem isto cada
// reinício valeria um chefe.
const (
	reiTrollZumbiTemplate = "Rei_Troll_Zumbi"
	// reiTrollZumbiHoras é a espera entre a morte e a volta
	// (esperaDoRenascimento), e a espera depois do boot (ApplyReiTrollZumbiBoot).
	reiTrollZumbiHoras = 1
)

// reiTrollZumbiPacote é o que cada morte entrega, item e quantidade, na ordem
// em que entra na bolsa.
var reiTrollZumbiPacote = []struct {
	item       int16
	quantidade int
}{
	{itemMoeda5KK, 1}, // Moeda de Prata (5Mi)
	{412, 5},          // Poeira de Oriharucon
	{413, 3},          // Poeira de Lactolerium
	{2395, 20},        // Âmago de Dente de Sabre
}

// isReiTrollZumbi diz se o monstro é o Rei Troll Zumbi, pelo nome do arquivo do
// template, como a Mesa: um "/gm criar Rei_Troll_Zumbi" também paga.
func isReiTrollZumbi(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(reiTrollZumbiTemplate)
}

// geradorDoReiTrollZumbi diz se o bloco idx é o do Rei Troll Zumbi.
func geradorDoReiTrollZumbi(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(reiTrollZumbiTemplate)
}

// reiTrollZumbiSaque entrega o pacote da morte do Rei Troll Zumbi na bolsa de
// quem mata. Um item que a Mesa de Drops governa neste monstro (uma regra dele,
// ou um "*" a 0% que tire o item do mundo) sai do pacote: vale a Mesa, como no
// prêmio dos outros chefes (entregaPremioDeChefe).
func (d *Dispatcher) reiTrollZumbiSaque(w *world.World, reward, mob *world.Entity) {
	if !isReiTrollZumbi(mob) {
		return
	}
	for _, p := range reiTrollZumbiPacote {
		if d.dropRules.Governs(mob.TemplateName, p.item) {
			continue
		}
		it := world.Item{Index: p.item}
		if p.quantidade > 1 && isSplittable(it.Index) {
			setItemAmount(&it, p.quantidade)
		}
		d.putMobDrop(w, reward, it)
	}
}

// ApplyReiTrollZumbiBoot segura o Rei Troll Zumbi por reiTrollZumbiHoras depois
// que o servidor sobe.
func (d *Dispatcher) ApplyReiTrollZumbiBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDoReiTrollZumbi(w, idx) && w.DeferGenerator(idx, reiTrollZumbiHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("Rei Troll Zumbi nasce uma hora depois do boot", "horas", reiTrollZumbiHoras, "blocos", blocos)
}
