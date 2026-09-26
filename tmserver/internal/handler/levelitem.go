package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A entrega de item por nível (DoItemLevel, Server.cpp:9744).
//
// Ao subir de nível o jogo escolhe uma peça pela ficha e a põe no ARMAZÉM da
// conta — não na bolsa. É o que veste o personagem no caminho do 29 ao 254:
// cinco peças de set, doze armas e uma montaria, a Cavalo_Leve_N do nível 149.
//
// Estava no conteúdo e nunca foi lido: o LevelItem.txt viaja no Release desde
// sempre, o legado o carrega em três lugares, e do lado Go não havia uma única
// referência ao arquivo. Por isso ninguém recebia nada.

// construcaoDoPersonagem é o `type` do legado: qual atributo BASE é o maior.
// Força vence empate com Int e Des; Int vence Des; e qualquer outra coisa —
// Constituição maior, ou empate entre os que sobraram — cai no 3. A ordem das
// comparações é a do original (Server.cpp:9748-9758), e ela não é simétrica:
// dois atributos empatados no topo não caem no 0 nem no 1, caem no 3.
//
// Lê o BaseScore de propósito. Os campos vivos (e.Str, e.Int…) são base mais
// equipamento, e usá-los faria a peça do nível depender do que o jogador está
// vestindo na hora — inclusive da peça anterior desta mesma entrega.
func construcaoDoPersonagem(e *world.Entity) int {
	f, i, d, c := e.BaseStr, e.BaseInt, e.BaseDex, e.BaseCon
	switch {
	case f > i && f > d && f > c:
		return 0
	case i > f && i > d && i > c:
		return 1
	case d > f && d > i && d > c:
		return 2
	default:
		return 3
	}
}

// entregaItemDeNivel dá ao personagem a peça do nível que ele acabou de
// alcançar, se houver uma.
//
// Chamado UMA vez por nível cruzado, de dentro do laço do applyLevelUps. O legado
// sobe um nível por chamada de CheckGetLevel (CMob.cpp:1113) e chama DoItemLevel
// logo depois de cada uma, então quem pula sete níveis recebe a peça de cada
// nível que entrega. Este comentário já disse o contrário ("recebe a peça do nível
// em que parou"), o código seguiu o comentário, e a peça dos níveis do meio sumia
// calada.
//
// A peça vai para o armazém da conta, como no original (ITEM_PLACE_CARGO), numa
// das vagas que o legado usa (vagaParaItemDeNivel). Se não couber, o legado
// desiste calado e o item some. Aqui o item também não é
// entregue — a fidelidade da regra fica —, mas fica registrado com nome, conta e
// item, porque um item que some sem rastro é um chamado de suporte sem resposta,
// e a equipe já tem uma lista desses para investigar.
func (d *Dispatcher) entregaItemDeNivel(w *world.World, s *world.Session, e *world.Entity) {
	if d.levelItems == nil || s == nil || e == nil {
		return
	}
	// Só o Mortal recebe. Os oito pontos do legado que sobem nível chamam
	// DoItemLevel atrás de `if ClassMaster == MORTAL` (SendFunc.cpp:1042,
	// _MSG_Attack.cpp:1772 e _MSG_UseItem.cpp:1028, 1045, 1080, 2397, 2435 e
	// 2475). Sem isto, um Arch ou um Celestial que passasse pelo 29 recebia a peça.
	if e.ClassMaster != classMasterMortal {
		return
	}
	item, construcao := pecaDoNivel(d.levelItems, e)
	if item.Empty() {
		return
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		d.log.Warn("item de nível: a conta não tem armazém carregado",
			"conn", s.Conn, "personagem", e.Name, "nivel", e.Level, "item", item.Index)
		return
	}
	vaga := vagaParaItemDeNivel(cargo)
	if vaga < 0 {
		d.log.Warn("item de nível: armazém cheio, o item não foi entregue",
			"conn", s.Conn, "conta", s.AccountID, "personagem", e.Name,
			"nivel", e.Level, "item", item.Index)
		return
	}
	it := world.Item{Index: item.Index}
	for i, ef := range item.Effects {
		it.Effects[i] = world.Effect{Effect: ef[0], Value: ef[1]}
	}
	cargo.Items[vaga] = it
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(world.ItemPlaceCargo, vaga, itemToSel(it)))
	sendClientMessage(w, s, "Um item chegou ao seu armazém. ["+d.itemName(item.Index)+"]")
	d.log.Info("item de nível entregue",
		"conn", s.Conn, "conta", s.AccountID, "personagem", e.Name, "classe", e.Class,
		"construcao", construcao, "nivel", e.Level, "item", item.Index, "vaga", vaga)
}

// As construções que o LevelItem conhece (content.LevelItems): 0 Força, 1 Int,
// 2 Destreza e 3 o resto.
const (
	construcaoInt        = 1
	construcaoDestreza   = 2
	construcaoIndefinida = 3
)

// ladoMago é para onde vai quem não tem construção definida: a coluna de Int, e
// depois a de Destreza, que é onde o arquivo guarda o lado "MAG" da Huntress
// ("#ARMA HT MAG NO LEVEL 155" é TYPE 3 — as Presas de Behemoth e o Dianus).
var ladoMago = []int{construcaoInt, construcaoDestreza}

// pecaDoNivel escolhe a peça do nível pela construção do personagem, e devolve
// também a construção que acabou valendo, para o log.
//
// DIVERGÊNCIA DELIBERADA, pedida pelo Marco em 26/09/2026: quem não tem
// construção definida recebe pelo lado mago. O personagem nasce com os quatro
// atributos iguais (12/12/12/12, dbserver CreateCharacter) e subir de nível não
// mexe neles — só dá pontos livres. Quem é upado sem distribuir fica empatado,
// cai no balde 3, e o arquivo não tem peça nenhuma para o 3: no legado ele
// passava do 29 ao 254 sem receber um set nem uma arma. "Precisa receber, podemos
// optar nesse caso pelo lado mago."
//
// Só o balde 3 cai para o lado mago. Quem tem construção (Força, Int ou
// Destreza) recebe o que a tabela dá a ela, inclusive nada — o TK, a FM e o BM
// de Destreza não têm coluna no arquivo, e isso é decisão de conteúdo, não daqui.
func pecaDoNivel(t *content.LevelItems, e *world.Entity) (content.LevelItem, int) {
	construcao := construcaoDoPersonagem(e)
	item := t.Para(int(e.Class), construcao, e.Level)
	if !item.Empty() || construcao != construcaoIndefinida {
		return item, construcao
	}
	for _, lado := range ladoMago {
		if item := t.Para(int(e.Class), lado, e.Level); !item.Empty() {
			return item, lado
		}
	}
	return item, construcao
}

// vagaParaItemDeNivel é a primeira vaga livre do armazém entre as que o legado
// usa para esta entrega: o DoItemLevel percorre i < MAX_CARGO - 2
// (Server.cpp:9762), então as duas últimas nunca recebem a peça e, com as outras
// ocupadas, o original desiste. O firstEmptyCargoSlot vai até o fim e fica como
// está, porque o kit de novato também o usa.
func vagaParaItemDeNivel(c *world.CargoState) int {
	for i := 0; i < world.MaxCargo-2; i++ {
		if c.Items[i].Empty() {
			return i
		}
	}
	return -1
}
