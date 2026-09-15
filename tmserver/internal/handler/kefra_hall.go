package handler

import (
	"fmt"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Hall do Kefra é pago em entradas, não em ouro, e as entradas vêm de um item
// comprado. São duas peças do legado que nunca foram portadas, e uma sem a outra
// não serve para nada:
//
//   - o porteiro (NPC Sobrevivente, em 2370,3888) troca um Pergaminho_Selado por
//     100 entradas (_MSG_Quest.cpp:2598-2623);
//   - o piso em 2364,3892 gasta uma entrada e leva para dentro do Hall
//     (GetFunc.cpp:994-1004).
//
// Sem isto a área do Kefra era inalcançável a pé: do pouso do /kefra não há
// caminho até lá (medido sobre o AttributeMap, com a regra de passo do route.Next).
const (
	// hallDoKefraPiso* é o piso que leva para dentro. O legado arredonda a
	// posição para baixo até o múltiplo de 4 antes de comparar
	// (`xv = (*x) & 0xFFFC`, GetFunc.cpp:784-785), então o piso é o bloco de
	// 4x4 casas que começa aqui, não uma casa só.
	hallDoKefraPisoX = 2364
	hallDoKefraPisoY = 3892

	// hallDoKefraDest* é onde o jogador cai, com o espalhamento de três casas
	// que toda rota do legado usa (`2364 + rand() % 3`).
	hallDoKefraDestX = 2364
	hallDoKefraDestY = 3906

	// itemPergaminhoSelado é o item que o porteiro troca (ItemList.csv:5801).
	itemPergaminhoSelado = 4127

	// entradasDoPergaminho é quanto um Pergaminho vale (_MSG_Quest.cpp:2614).
	entradasDoPergaminho = 100

	// gradeSobrevivente é o EF_GRADE0 que faz do NPC o porteiro do Kefra
	// (_MSG_Quest.cpp:85-86).
	gradeSobrevivente = 22
)

// msgEntradasDoKefra é _DN_CHANGE_COUNT (Language.txt:420). A linha tem %d, então
// say e noticeLine a recusam de propósito — elas não formatam nada.
const (
	chaveEntradasDoKefra = "_DN_CHANGE_COUNT"
	msgEntradasDoKefra   = "Você pode usar isto %d vezes."
)

// noPisoDoHallDoKefra diz se a posição está no bloco de 4x4 que leva ao Hall.
func noPisoDoHallDoKefra(x, y int16) bool {
	return x&^3 == hallDoKefraPisoX && y&^3 == hallDoKefraPisoY
}

// destinoDoHallDoKefra sorteia onde o jogador cai dentro do Hall.
func destinoDoHallDoKefra(intn func(int) int) (int16, int16) {
	return hallDoKefraDestX + int16(intn(3)), hallDoKefraDestY + int16(intn(3))
}

// entraNoHallDoKefra é o piso do Hall. Devolve true quando o piso é dele — aí o
// chamador não procura mais rota nenhuma.
//
// LOOP ONLY.
func (d *Dispatcher) entraNoHallDoKefra(w *world.World, s *world.Session, e *world.Entity) bool {
	if !noPisoDoHallDoKefra(e.X, e.Y) {
		return false
	}
	// Sem entrada, nada acontece. No legado a condição do ticket faz parte do
	// teste do piso, então quem pisa sem entrada não casa com rota nenhuma e fica
	// onde está, calado. O piso continua sendo nosso: devolver false aqui jogaria
	// a posição na tabela de rotas, que não responde por ela.
	if e.KefraTicket <= 0 {
		return true
	}
	e.KefraTicket--
	// O gasto não vai ao banco na hora, de propósito: o legado também não grava, e
	// uma queda entre o gasto e o save periódico devolve a entrada ao jogador em
	// vez de tomá-la. O crédito é o contrário, ver sobreviventeDoKefra.
	d.avisaEntradasDoKefra(w, s, e.KefraTicket)
	x, y := destinoDoHallDoKefra(w.Rand().Intn)
	d.doTeleport(w, s, x, y)
	return true
}

// sobreviventeDoKefra é o clique no porteiro: um Pergaminho_Selado por 100
// entradas (_MSG_Quest.cpp:2598-2623). Sem o item na bolsa não acontece nada.
//
// LOOP ONLY.
func (d *Dispatcher) sobreviventeDoKefra(w *world.World, s *world.Session, e *world.Entity) {
	slot := d.slotDaChave(e, itemPergaminhoSelado)
	if slot < 0 {
		return
	}
	// O legado limpa a casa inteira (BASE_ClearItem, _MSG_Quest.cpp:2611). O
	// Pergaminho_Selado não empilha — a linha dele no ItemList.csv não tem
	// EF_AMOUNT —, então consumeOneItem tira exatamente a mesma coisa, e continua
	// certo se algum dia empilhar, em vez de apagar a pilha toda.
	consumeOneItem(&e.Carry[slot])
	d.sendSlot(w, s, world.ItemPlaceCarry, slot, e.Carry[slot])
	e.KefraTicket += entradasDoPergaminho
	// Gravado na hora, como a Escritura do Pesadelo: o pergaminho é comprado, e
	// perder 100 entradas para uma queda antes do save periódico é dinheiro do
	// jogador.
	w.SaveCharacterAsync(s)
	d.avisaEntradasDoKefra(w, s, e.KefraTicket)
	d.log.Info("entradas do Hall do Kefra entregues",
		"conta", s.AccountName, "entregues", entradasDoPergaminho, "total", e.KefraTicket)
}

// avisaEntradasDoKefra diz quantas entradas o jogador tem agora. A linha do
// Language.txt só é usada se tiver exatamente o %d que ela precisa; qualquer
// outra coisa cairia num Sprintf errado e mostraria %!d(MISSING) ao jogador.
func (d *Dispatcher) avisaEntradasDoKefra(w *world.World, s *world.Session, restam int32) {
	texto := msgEntradasDoKefra
	if t, ok := d.lang.Text(chaveEntradasDoKefra); ok && strings.Count(t, "%") == 1 && strings.Contains(t, "%d") {
		texto = t
	}
	sendClientMessage(w, s, fmt.Sprintf(texto, restam))
}
