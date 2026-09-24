package world

import "sort"

// A TRAVESSIA DAS PRATELEIRAS, e ela mora aqui porque as barracas moram aqui: o
// AutoTrade é estado de sessão, do laço, e este pacote é quem o possui.
//
// Ela ficava no handler, e saiu de lá quando o servidor de controle passou a
// precisar da mesma leitura para a vitrine do site. Duas travessias das mesmas
// prateleiras divergiriam no dia em que só uma fosse corrigida, e a divergência aqui
// é alguém ver na página um item que o jogo não vende, ou não ver um que ele vende.

// OfertaDoMercado é uma prateleira como o servidor a conhece por dentro, com tudo
// o que qualquer leitor pode querer — inclusive o que NÃO vai para tela nenhuma.
//
// UMA TRAVESSIA SÓ, e este tipo existe por causa disso. Duas leituras das mesmas
// prateleiras — a do jogo e a do site — divergiriam no dia em que só uma fosse
// corrigida, e a divergência aqui é alguém ver na página um item que o jogo não
// vende, ou não ver um que ele vende.
type OfertaDoMercado struct {
	// ContaVendedor NÃO vai para o site. Quem chama usa para perguntar ao banco se
	// aquela prateleira já tem alguém pagando, e joga fora antes de responder.
	ContaVendedor int64
	// CargoPos, com a conta, nomeia a linha do anúncio no banco.
	CargoPos int

	Personagem string
	Barraca    int32
	Slot       int8
	Indice     int16
	Refino     uint8
	Qtd        uint8
	Moeda      uint8
	Preco      int32
	// Cidade é o índice da vila, o mesmo que o imposto usa; -1 fora de cidade.
	Cidade int
	// AbertaHaSegundos é DURAÇÃO e não instante, e a diferença importa: o relógio da
	// barraca é o contador de tiques do laço, e uma data feita dele estaria errada no
	// primeiro reinício do servidor.
	AbertaHaSegundos int64
	// Perto é 1 quando dá para comprar sem andar. Ver o comentário no laço.
	Perto uint8
}

// OfertasDoMercado percorre as barracas abertas e devolve uma linha por item à
// venda, em ordem estável (barraca, depois posição).
//
// É A ÚNICA TRAVESSIA das prateleiras no servidor. A vitrine do jogo e a do site
// saem daqui, cada uma estreitando o que precisa.
func OfertasDoMercado(w *World) []OfertaDoMercado {
	var todas []OfertaDoMercado
	agora := w.Now()
	w.ForEachSession(func(s *Session, e *Entity) {
		if s == nil || s.AutoTrade == nil || e == nil {
			return
		}
		barraca := int32(s.IDDaBarraca())
		cidade := Village(e.X, e.Y)
		// A subtração é entre tiques do MESMO laço, então ela é segura mesmo com o
		// contador dando a volta: o resultado de uint32 - uint32 continua sendo a
		// distância. Negativo não acontece; zero é uma barraca recém-aberta.
		aberta := int64(agora-s.AutoTrade.OpenedAt) / 1000
		if aberta < 0 {
			aberta = 0
		}
		for i := range s.AutoTrade.Slots {
			sl := s.AutoTrade.Slots[i]
			if sl.CargoPos < 0 || sl.Item.Empty() {
				continue
			}
			todas = append(todas, OfertaDoMercado{
				ContaVendedor:    s.AccountID,
				CargoPos:         sl.CargoPos,
				Personagem:       e.Name,
				Barraca:          barraca,
				Slot:             int8(i),
				Indice:           sl.Item.Index,
				Refino:           sl.Item.Refino(),
				Qtd:              sl.Item.QuantidadeNaVitrine(),
				Moeda:            s.AutoTrade.Moeda[i],
				Preco:            sl.Price,
				Cidade:           cidade,
				AbertaHaSegundos: aberta,
				// O mercado é do servidor inteiro: barraca de qualquer cidade entra na
				// vitrine, e comprar não exige chegar perto (decisão da Josiel,
				// 19/09/2026). O que a distância ainda decide é o imposto, que se
				// reparte entre a cidade da barraca e a de quem compra — ver lojacompra.
				Perto: 1,
			})
		}
	})
	sort.Slice(todas, func(a, b int) bool {
		if todas[a].Barraca != todas[b].Barraca {
			return todas[a].Barraca < todas[b].Barraca
		}
		return todas[a].Slot < todas[b].Slot
	})
	return todas
}

// IDDaBarraca é o corpo que representa o vendedor na vitrine: o clone, quando ele
// existe, e a conexão quando a barraca caiu na pose antiga.
//
// Veio do handler junto com a travessia. O clone é preferido porque é ele que fica
// de pé no mapa enquanto o vendedor anda.
func (s *Session) IDDaBarraca() int {
	if s.AutoTrade != nil && s.AutoTrade.CloneID >= MaxUser {
		return s.AutoTrade.CloneID
	}
	return s.Conn
}

// Refino é o "+N" do item, que mora em EF_SANC.
func (it Item) Refino() uint8 {
	for _, ef := range it.Effects {
		if ef.Effect == efSancNaVitrine {
			return ef.Value
		}
	}
	return 0
}

// QuantidadeNaVitrine é o tamanho da pilha, preso entre 1 e 255 porque é assim que
// ele cabe na vitrine do cliente.
//
// Item sem EF_AMOUNT é UM, e não zero: é a mesma leitura do resto do servidor, e um
// zero aqui faria a vitrine anunciar "0 unidades" de um item que existe.
func (it Item) QuantidadeNaVitrine() uint8 {
	n := 1
	for _, ef := range it.Effects {
		if ef.Effect == efAmountNaVitrine {
			n = int(ef.Value)
		}
	}
	if n < 1 {
		return 1
	}
	if n > 255 {
		return 255
	}
	return uint8(n)
}

// Os dois efeitos que a vitrine lê. Vêm do pilha/refine e estão repetidos aqui pelo
// mesmo motivo que já estão repetidos em meia dúzia de lugares do servidor: são
// números do formato do item, fixos desde o cliente de 2003.
const (
	efSancNaVitrine   = 43 // EF_SANC
	efAmountNaVitrine = 61 // EF_AMOUNT
)
