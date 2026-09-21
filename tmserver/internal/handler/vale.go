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
	valePisoX = 2548
	valePisoY = 1740
	valeDestX = 2281
	valeDestY = 3688

	// valeSaidaX/Y é para onde a varredura manda quem está no Vale sem a fada: o
	// mesmo ponto de Azran que o comando /azran usa (chat.go), dentro da cidade e
	// longe de qualquer piso de teleporte — cair em cima de um mandaria o jogador
	// para outro lugar no passo seguinte.
	valeSaidaX = 2500
	valeSaidaY = 1716

	// valeSweepPeriod é de quantos em quantos tiques de 1 s a varredura roda.
	// Quatro segundos é o bastante para que tirar a fada signifique sair, sem
	// varrer a lista de jogadores a cada pulso.
	valeSweepPeriod = 4
)

// valeBox é a caixa do Vale Escondido, a mesma do Regions.txt
// ("2173, 3583, 2307, 3711 = Vale_Escondido"). A varredura usa a região inteira,
// e não o ponto de chegada, porque o que se quer é "não há ninguém sem fada
// dentro do Vale", não "ninguém aterrissou ali".
var valeBox = areaBox{2173, 3583, 2307, 3711}

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
	return e.Equip[fairyEquipSlot].Index == fadaDoValeIndex
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

// sweepVale tira do Vale quem está lá dentro sem a Fada do Vale.
//
// Esta parte NÃO é do legado, que só condiciona a entrada: quem entrasse e
// perdesse a fada ficava. É decisão desta operação (20/09/2026) — "só pode
// entrar com fada, nunca sem fada" — e ela é o que cobre os três casos que a
// porta sozinha deixa passar: quem entrou enquanto a rota estava sem condição,
// quem tira a fada depois de chegar, e quem está lá quando os sete dias dela
// acabam (tickFairies limpa o slot, ProcessSecMinTimer.cpp:612).
//
// Manda para Azran, de onde se entra, e avisa — um personagem que muda de mapa
// sozinho e em silêncio parece um servidor com defeito.
//
// LOOP ONLY.
func (d *Dispatcher) sweepVale(w *world.World) {
	if d.tickCount%valeSweepPeriod != 0 {
		return
	}
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		if !valeBox.contains(e.X, e.Y) || temFadaDoVale(e) {
			return
		}
		d.notify(w, s, NoticeValeSemFada)
		d.doTeleport(w, s, valeSaidaX, valeSaidaY)
		d.log.Info("tirado do Vale sem a Fada do Vale",
			"account", s.AccountName, "conn", s.Conn, "x", e.X, "y", e.Y)
	})
}
