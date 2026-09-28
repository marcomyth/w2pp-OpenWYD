package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// corrigeRepletionAberrante passa CorrigeDefesaCombinada nos itens que acabam de
// ser carregados — equipamento e bolsa no login do personagem, armazém no login
// da conta — e registra cada peça corrigida.
//
// No carregamento, e não por migração no banco: o laço é dono do estado vivo, e
// um UPDATE feito com o jogador online seria desfeito pelo próximo salvamento. Aqui
// a peça já sai do banco corrigida e é salva assim na saída.
//
// Pedido do Marco em 27/09/2026. O log do reroll mostrou 23 sorteios assim entre
// 04:01 e 04:40 do dia 27, e quatro peças que ficaram com o último deles.
func (d *Dispatcher) corrigeRepletionAberrante(items []world.Item, onde, conta, personagem string) int {
	n := 0
	for slot := range items {
		antes := items[slot]
		if !refine.CorrigeDefesaCombinada(&items[slot], d.itemPos[int(antes.Index)]) {
			continue
		}
		n++
		d.log.Warn("repletion: defesa combinada corrigida para 30",
			"conta", conta, "personagem", personagem, "onde", onde, "slot", slot,
			"item", antes.Index, "antes", antes.Effects, "depois", items[slot].Effects)
	}
	return n
}
