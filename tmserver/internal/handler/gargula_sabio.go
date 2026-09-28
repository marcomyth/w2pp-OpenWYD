package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A Gárgula Sábio do 2º andar da Dungeon e os guardas dela, pedido do Marco em
// 25/09/2026 com print em (865,3879). Só o bloco da foto (2540, em (872,3876)):
// a outra Gárgula Sábio do andar (bloco 2478) e os Golens de Fogo soltos ficam
// como estavam, e por isso os dois usam cópias dos templates — a Mesa e a ficha
// valem por template. O nome de dentro das cópias é o do original, então o
// jogador vê a mesma Gárgula Sábio e o mesmo Golem de Fogo.
//
//   - A Gárgula (Gargula_Sabio_Chefe): forte, mas abaixo dos mini chefes da lava
//     (300 mil de vida real, dano 1.800) — 150 mil de vida, dano 1.200, defesa
//     2.800 e resistência 15, contra 30 mil, 800, 2.600 e 5-10 da Gárgula comum.
//     Volta 1 hora depois da morte e fica 1 hora de fora depois do boot. Cada
//     morte solta UMA Arma C com o add alto do Boss Conjurador (72/81 de dano ou
//     32/36 de magia), como pedido.
//   - Os guardas (Golem_Guarda, bloco 6162): os Golens de Fogo que nasciam como
//     seguidores dela, agora num bloco próprio no mesmo ponto — o bloco antigo
//     trazia uma Gárgula nova a cada guarda morto. +50% de vida (54 mil reais) e
//     de dano (1.065) e defesa 2.300. Voltam pela fila da morte, como os bichos
//     sem período. O saque é a Mesa (migração 0151): Poeiras de Oriharucon e de
//     Lactolerium, as Jóias e a Moeda de Prata (1Mi).
const (
	gargulaSabioChefeTemplate = "Gargula_Sabio_Chefe"
	golemGuardaTemplate       = "Golem_Guarda"
	// gargulaSabioHoras é a espera entre a morte e a volta
	// (esperaDoRenascimento), e a espera depois do boot (ApplyGargulaSabioBoot).
	gargulaSabioHoras = 1
)

// isGargulaSabioChefe diz se o monstro é a Gárgula Sábio chefe, pelo nome do
// arquivo do template, como a Mesa: um "/gm criar Gargula_Sabio_Chefe" também paga.
func isGargulaSabioChefe(mob *world.Entity) bool {
	return droprule.Canonical(mob.TemplateName) == droprule.Canonical(gargulaSabioChefeTemplate)
}

// geradorDaGargulaSabio diz se o bloco idx é o da Gárgula Sábio chefe.
func geradorDaGargulaSabio(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(gargulaSabioChefeTemplate)
}

// gargulaSabioSaque entrega UMA Arma C com o add alto na bolsa de quem mata
// (entregaArmaCDeChefe).
func (d *Dispatcher) gargulaSabioSaque(w *world.World, reward, mob *world.Entity) {
	if !isGargulaSabioChefe(mob) {
		return
	}
	d.entregaArmaCDeChefe(w, reward, mob)
}

// ApplyGargulaSabioBoot segura a Gárgula Sábio chefe por gargulaSabioHoras depois
// que o servidor sobe, como os outros chefes: o boot povoa todo bloco de uma vez,
// e sem isto cada reinício valeria uma Arma C.
func (d *Dispatcher) ApplyGargulaSabioBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDaGargulaSabio(w, idx) && w.DeferGenerator(idx, gargulaSabioHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("Gárgula Sábio chefe nasce uma hora depois do boot", "horas", gargulaSabioHoras, "blocos", blocos)
}
