package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Vale Escondido é fechado: só entra quem carrega a Fada do Vale.
//
// A rota de Azran é a terceira das rotas CONDICIONAIS de GetTeleportPosition, ao
// lado das duas do Kefra, e por isso não cabe na teleportTable, que é consulta
// pura. O legado escreve o destino apenas quando o slot da fada segura o item
// 3916 (GetFunc.cpp:924-931):
//
//	else if (xv == 2548 && yv == 1740) // Azran to vale
//	{
//	    if (pMob[conn].MOB.Equip[13].sIndex == 3916)
//	    { *x = 2281 + rand() % 3; *y = 3688 + rand() % 3; }
//	}
//
// Sem a fada, x e y voltam como entraram e o jogador fica onde está. Este port
// tinha a rota na tabela SEM a condição, então qualquer um pisava e ia parar num
// vale que é a casa de dois chefes de milhões de HP.
const (
	// itemFadaDoVale é o 3916 "Fada_do_Vale(7dias)" do ItemList.csv — a chave, e a
	// única: nenhuma das outras fadas abre o Vale.
	itemFadaDoVale = 3916

	valePisoX = 2548
	valePisoY = 1740
	valeDestX = 2281
	valeDestY = 3688
)

// noPisoDoVale diz se a posição está no bloco 4x4 do piso de Azran. O legado
// arredonda para múltiplo de 4 antes de comparar (GetFunc.cpp:784-785), então é
// bloco e não casa.
func noPisoDoVale(x, y int16) bool {
	return x&^3 == valePisoX && y&^3 == valePisoY
}

// destinoDoVale sorteia onde o jogador cai, com o espalhamento de três casas de
// toda rota do legado (GetFunc.cpp:928-929).
func destinoDoVale(intn func(int) int) (int16, int16) {
	return valeDestX + int16(intn(3)), valeDestY + int16(intn(3))
}

// temFadaDoVale diz se o slot da fada segura a Fada do Vale. O legado compara o
// sIndex direto, sem olhar prazo nem efeito: a fada vencida já saiu do slot pelo
// caminho normal dos itens com prazo.
func temFadaDoVale(e *world.Entity) bool {
	return e.Equip[fairyEquipSlot].Index == itemFadaDoVale
}

// entraNoVale é o piso de Azran que leva ao Vale Escondido. Devolve true quando o
// piso é dele — aí o chamador não procura mais rota nenhuma.
//
// Sem a fada o legado fica mudo, porque lá a posição simplesmente não casa com
// rota alguma. Aqui dizemos o motivo: um piso que não faz nada é indistinguível
// de um teleporte quebrado, e este é o piso de uma área que o jogador PRECISA
// saber como abrir.
//
// LOOP ONLY.
func (d *Dispatcher) entraNoVale(w *world.World, s *world.Session, e *world.Entity) bool {
	if !noPisoDoVale(e.X, e.Y) {
		return false
	}
	if !temFadaDoVale(e) {
		d.notify(w, s, NoticeValeSemFada)
		return true // o piso é nosso, e sem a fada não leva a lugar nenhum
	}
	x, y := destinoDoVale(w.Rand().Intn)
	d.doTeleport(w, s, x, y)
	return true
}
