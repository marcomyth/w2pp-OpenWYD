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

// O PREÇO MÍNIMO EM DINHEIRO REAL É R$ 5,00, e a recusa é na montagem.
//
// SUBIU DE R$ 1,00 quando a taxa da casa entrou: com R$ 0,80 fixos por venda, R$ 1,00
// deixaria o vendedor com R$ 0,15 — uma venda que entrega quinze centavos é uma
// reclamação, não uma venda. O cliente novo também trava, mas quem manda é o servidor:
// um cliente remendado não pode criar anúncio de um centavo.
//
// 499 e 500 são os dois lados exatos da linha. Testar 1 e 1000 não provaria onde ela
// está, e "onde está a linha" é a única coisa que pode sair errada num mínimo.
func TestLojaAbrirRecusaPrecoAbaixoDoMinimoEmRMT(t *testing.T) {
	const solto, pilha = int16(1030), int16(2020)

	t.Run("499 centavos recusa", func(t *testing.T) {
		addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 1))
		defer stop()
		c := enterWorldAs(t, addr, "tester")
		defer c.Close()
		drena(t, c)

		mandaAbrirBarraca(t, c, "Loja", 0, 499, protocol.LojaMoedaRMT)

		if !recebeu(t, c, msgPrecoMinimoRMT) {
			t.Error("499 centavos passou, ou foi recusado em silêncio")
		}
	})

	t.Run("500 centavos aceita", func(t *testing.T) {
		addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 1))
		defer stop()
		c := enterWorldAs(t, addr, "tester")
		defer c.Close()
		drena(t, c)

		mandaAbrirBarraca(t, c, "Loja", 0, 500, protocol.LojaMoedaRMT)

		// O MÍNIMO NÃO PODE RECUSAR O PRÓPRIO MÍNIMO. É o erro de um a menos que um
		// teste de "1 recusa, 1000 aceita" nunca encontraria.
		if recebeu(t, c, msgPrecoMinimoRMT) {
			t.Error("500 centavos foi recusado; o mínimo está excluindo o próprio valor")
		}
	})

	// O TETO, pelos dois lados. Ele NASCEU com a taxa da casa e antes não existia
	// nenhum — a falta dele era mais perigosa que a falta do mínimo, porque R$ 50.000
	// num campo de preço é um erro de digitação plausível e do outro lado sai um Pix
	// de verdade da conta de alguém.
	t.Run("50000 centavos aceita", func(t *testing.T) {
		addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 1))
		defer stop()
		c := enterWorldAs(t, addr, "tester")
		defer c.Close()
		drena(t, c)

		mandaAbrirBarraca(t, c, "Loja", 0, 50_000, protocol.LojaMoedaRMT)

		if recebeu(t, c, msgPrecoMaximoRMT) {
			t.Error("R$ 500,00 foi recusado; o teto está excluindo o próprio valor")
		}
	})

	t.Run("50001 centavos recusa", func(t *testing.T) {
		addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 1))
		defer stop()
		c := enterWorldAs(t, addr, "tester")
		defer c.Close()
		drena(t, c)

		mandaAbrirBarraca(t, c, "Loja", 0, 50_001, protocol.LojaMoedaRMT)

		if !recebeu(t, c, msgPrecoMaximoRMT) {
			t.Error("um centavo acima do teto passou, ou foi recusado em silêncio")
		}
	})

	// E O OURO NÃO É TOCADO: o mínimo e o teto são do dinheiro real, e uma prateleira
	// de um gold — ou de dois milhões — continua valendo.
	t.Run("ouro de 1 continua valendo", func(t *testing.T) {
		addr, stop, _ := startServerClock(t, lojaRMTdb(solto, pilha, 1))
		defer stop()
		c := enterWorldAs(t, addr, "tester")
		defer c.Close()
		drena(t, c)

		mandaAbrirBarraca(t, c, "Loja", 0, 1, protocol.LojaMoedaOuro)

		if recebeu(t, c, msgPrecoMinimoRMT) {
			t.Error("o mínimo do dinheiro real vazou para o ouro")
		}
	})
}
