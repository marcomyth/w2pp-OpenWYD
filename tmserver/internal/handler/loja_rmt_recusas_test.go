package handler

import (
	"bytes"
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// lojaRMTdb monta o vendedor com dois slots no baú: um item solto e uma pilha.
func lojaRMTdb(item int16, pilha int16, qtd uint8) *fakeDB {
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7: {Slot: 0, Name: "Seller", Level: 1, HP: 1000, MaxHP: 1000, Coin: 1000},
	}
	var cargo world.CargoState
	cargo.Items[0] = world.Item{Index: item}
	cargo.Items[1] = world.Item{Index: pilha}
	cargo.Items[1].Effects[0] = world.Effect{Effect: efAmount, Value: qtd}
	db.accounts["tester"].cargo = cargo
	return db
}

// recebeu diz se a mensagem exata chegou ao jogador.
//
// A comparação é contra o corpo CODIFICADO e não contra o texto em Go: o
// servidor converte de UTF-8 para a tabela do cliente antes de mandar, então
// "não" não chega como os mesmos bytes que a constante tem aqui.
func recebeu(t *testing.T, c net.Conn, texto string) bool {
	t.Helper()
	esperado := protocol.EncodeMessagePanelBody(texto)
	for {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			return false
		}
		if ty == protocol.MsgMessagePanel && bytes.Equal(payload, esperado) {
			return true
		}
	}
}

// Pilha não vai a dinheiro real.
//
// O anúncio guarda a fotografia de UM item e a marca do escrow guarda UM id no
// slot do baú. Uma pilha de dez é um slot só, então "vendi três" não tem onde ser
// escrito — e venda parcial é exatamente o que o comprador espera de uma pilha,
// porque é assim que ela funciona em ouro.
//
// O teste exige as duas coisas: que a barraca NÃO suba, e que o vendedor saiba
// por quê. Recusa calada aqui é a pior: o jogador aperta montar, nada acontece, e
// ele não tem o que descrever no chamado.
func TestLojaAbrirRecusaPilhaEmRMT(t *testing.T) {
	const solto, pilha = int16(1030), int16(2020)
	addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 10))
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	mandaAbrirBarraca(t, c, "Loja", 1, 500, protocol.LojaMoedaRMT)

	if !recebeu(t, c, msgPilhaNaoVaiRMT) {
		t.Error("a pilha foi recusada em silêncio, ou não foi recusada")
	}
	// E a barraca não subiu: um LojaAbriu aqui significaria que a recusa aconteceu
	// depois de a prateleira já estar de pé.
	drena(t, c)
	if vitrine := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos); vitrine.Qtd != 0 {
		t.Errorf("a vitrine tem %d oferta(s); a barraca não devia ter subido", vitrine.Qtd)
	}
}

// E o contrário, que é o que dá valor ao teste de cima: item SOLTO em dinheiro
// real passa. Sem este caso, uma recusa geral de RMT passaria por correta.
func TestLojaAbrirAceitaItemSoltoEmRMT(t *testing.T) {
	const solto, pilha = int16(1030), int16(2020)
	addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 10))
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 500, protocol.LojaMoedaRMT)
}

// E a pilha continua vendível em OURO. A recusa é da combinação pilha+RMT, e não
// da pilha: cortar a venda de pilha em ouro seria tirar do jogo uma coisa que
// sempre funcionou.
func TestLojaAbrirAceitaPilhaEmOuro(t *testing.T) {
	const solto, pilha = int16(1030), int16(2020)
	addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 10))
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 1, 500, protocol.LojaMoedaOuro)
}

// Dinheiro real não se escolhe depois de a barraca estar de pé.
//
// Virar uma prateleira de ouro em dinheiro real por este caminho pularia tudo o
// que a montagem faz por um anúncio de verdade: a fotografia do item, a marca do
// escrow no slot do baú, a conferência da chave Pix. Sobraria uma oferta em reais
// sem anúncio nenhum atrás dela.
func TestLojaMoedaRecusaRMT(t *testing.T) {
	const solto, pilha = int16(1030), int16(2020)
	addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 10))
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	abreBarraca(t, c, "Loja", 0, 500, protocol.LojaMoedaOuro)
	drena(t, c)

	send(t, c, protocol.MsgLojaMoeda,
		(&protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaRMT}).Encode())

	if !recebeu(t, c, msgMoedaRMTSoNaMontagem) {
		t.Error("a troca para dinheiro real foi recusada em silêncio, ou não foi recusada")
	}
}

// E a troca entre as outras moedas continua funcionando. É o caso que impede que
// alguém "conserte" o guard recusando toda troca de moeda.
func TestLojaMoedaAindaTrocaEntreOuroECash(t *testing.T) {
	const solto, pilha = int16(1030), int16(2020)
	addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 10))
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	abreBarraca(t, c, "Loja", 0, 500, protocol.LojaMoedaOuro)
	drena(t, c)

	send(t, c, protocol.MsgLojaMoeda,
		(&protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaCash}).Encode())

	if recebeu(t, c, msgMoedaRMTSoNaMontagem) {
		t.Error("trocar para cash levou a recusa de dinheiro real")
	}
}
