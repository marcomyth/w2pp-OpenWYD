package handler

import (
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// pedidoDaLixeira monta o corpo do 0x0F50.
func pedidoDaLixeira(itens ...[2]int16) []byte {
	b := make([]byte, 4+4*len(itens))
	b[0] = uint8(len(itens))
	for i, it := range itens {
		p := 4 + 4*i
		b[p] = byte(it[0])
		b[p+1] = byte(it[0] >> 8)
		b[p+2] = byte(it[1])
		b[p+3] = byte(it[1] >> 8)
	}
	return b
}

// resultadoDaLixeira espera o 0x0F51 e lê o que veio nele.
//
// ELE FALHA O TESTE SE NÃO VIER, e isso é metade do que estes testes provam: a janela
// do cliente espera a resposta e, sem ela, escreve "sem resposta do servidor" depois
// de cinco segundos — um erro que não aconteceu.
func resultadoDaLixeira(t *testing.T, c net.Conn) (apagados int, recusas map[int16]uint8) {
	t.Helper()
	_, corpo, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, _ []byte) bool {
		return h.Type == protocol.MsgLixeiraResultado
	})
	if !ok {
		t.Fatal("o servidor nao respondeu o 0x0F51")
	}
	recusas = map[int16]uint8{}
	apagados = int(corpo[0])
	for i := 0; i < int(corpo[1]); i++ {
		p := 4 + 4*i
		recusas[int16(uint16(corpo[p])|uint16(corpo[p+1])<<8)] = corpo[p+2]
	}
	return apagados, recusas
}

// mesaDaLixeira sobe um servidor com um jogador em jogo.
func mesaDaLixeira(t *testing.T) (*servidorDoRelogio, net.Conn) {
	t.Helper()
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	t.Cleanup(func() { _ = c.Close() })
	drena(t, c)
	return srv, c
}

// O LOTE APAGA O QUE PASSA E DIZ POR QUE O RESTO FICOU.
//
// Item por item, e não tudo-ou-nada: um slot recusado no fim da lista não pode anular
// os apagamentos legítimos que vieram antes.
func TestLixeiraApagaOQuePassaEExplicaOResto(t *testing.T) {
	srv, c := mesaDaLixeira(t)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Carry[0] = world.Item{Index: 1100}
		e.Carry[1] = world.Item{Index: 2390}
		e.Carry[2] = world.Item{}
		e.Carry[3] = world.Item{Index: 500}
		d.lixeiraApaga(w, s, protocol.Header{}, pedidoDaLixeira(
			[2]int16{0, 1100}, // apaga
			[2]int16{1, 2390}, // apaga
			[2]int16{2, 700},  // vazio
			[2]int16{3, 999},  // o indice nao bate
		))
		if !e.Carry[0].Empty() || !e.Carry[1].Empty() {
			t.Error("os dois que podiam nao sairam da mochila")
		}
		if e.Carry[3].Index != 500 {
			t.Error("apagou o item cujo indice nao batia")
		}
	})

	apagados, recusas := resultadoDaLixeira(t, c)
	if apagados != 2 {
		t.Errorf("apagados = %d, quero 2", apagados)
	}
	if recusas[2] != protocol.LixeiraMotivoVazio {
		t.Errorf("motivo do slot 2 = %d, quero vazio(%d)", recusas[2], protocol.LixeiraMotivoVazio)
	}
	if recusas[3] != protocol.LixeiraMotivoOutroItem {
		t.Errorf("motivo do slot 3 = %d, quero outro item(%d)", recusas[3], protocol.LixeiraMotivoOutroItem)
	}
}

// O ÍNDICE QUE NÃO BATE É A RAZÃO DE ESTE PACOTE EXISTIR.
//
// Entre marcar e confirmar, um drop ou um arrastar muda o que está no slot. Apagar o
// que está lá agora seria apagar o item errado, e não tem volta.
func TestLixeiraNaoApagaItemQueMudouDeLugar(t *testing.T) {
	srv, c := mesaDaLixeira(t)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		// O jogador marcou o 1100; quando confirmou, o slot já era outro item.
		e.Carry[4] = world.Item{Index: 2222}
		d.lixeiraApaga(w, s, protocol.Header{}, pedidoDaLixeira([2]int16{4, 1100}))
		if e.Carry[4].Index != 2222 {
			t.Fatalf("apagou o item errado: slot = %d", e.Carry[4].Index)
		}
	})

	apagados, recusas := resultadoDaLixeira(t, c)
	if apagados != 0 {
		t.Errorf("apagados = %d, quero 0", apagados)
	}
	if recusas[4] != protocol.LixeiraMotivoOutroItem {
		t.Errorf("motivo = %d", recusas[4])
	}
}

// O MESMO SLOT DUAS VEZES: a primeira vale, a segunda é RECUSA DE REPETIDO.
//
// Sem isto, a segunda passagem veria o slot já vazio e diria "vazio" — um motivo que
// descreve o efeito da primeira linha, e não o erro de quem montou o lote.
func TestLixeiraDizRepetidoENaoVazio(t *testing.T) {
	srv, c := mesaDaLixeira(t)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Carry[5] = world.Item{Index: 1100}
		d.lixeiraApaga(w, s, protocol.Header{}, pedidoDaLixeira([2]int16{5, 1100}, [2]int16{5, 1100}))
	})

	apagados, recusas := resultadoDaLixeira(t, c)
	if apagados != 1 {
		t.Errorf("apagados = %d, quero 1", apagados)
	}
	if recusas[5] != protocol.LixeiraMotivoRepetido {
		t.Errorf("motivo = %d, quero repetido(%d)", recusas[5], protocol.LixeiraMotivoRepetido)
	}
}

// EM TROCA, O LOTE INTEIRO CAI E A TROCA CONTINUA.
//
// O apagar de um item só cancela a troca; aqui isso seria desproporcional — a pessoa
// clicou na lixeira, não na troca, e perderia o que já tinha combinado com outra.
func TestLixeiraEmTrocaRecusaTudoENaoCancelaATroca(t *testing.T) {
	srv, c := mesaDaLixeira(t)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Carry[0] = world.Item{Index: 1100}
		s.Trade.Active = true
		d.lixeiraApaga(w, s, protocol.Header{}, pedidoDaLixeira([2]int16{0, 1100}))
		if e.Carry[0].Empty() {
			t.Error("apagou durante a troca")
		}
		if !s.Trade.Active {
			t.Error("a troca foi cancelada; a lixeira nao pode fazer isso")
		}
	})

	apagados, recusas := resultadoDaLixeira(t, c)
	if apagados != 0 {
		t.Errorf("apagados = %d", apagados)
	}
	if recusas[0] != protocol.LixeiraMotivoEmTroca {
		t.Errorf("motivo = %d, quero em troca(%d)", recusas[0], protocol.LixeiraMotivoEmTroca)
	}
}

// O PACOTE MALFORMADO NÃO APAGA NADA E NÃO INVENTA SLOTS.
//
// Sem confiança nos números do pacote, listar os slots recusados seria repetir
// números que podem não ser os que o jogador marcou.
func TestLixeiraPacoteRuimNaoApagaENaoListaSlots(t *testing.T) {
	srv, c := mesaDaLixeira(t)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Carry[0] = world.Item{Index: 1100}
		// Diz que são dois e traz um.
		ruim := pedidoDaLixeira([2]int16{0, 1100}, [2]int16{1, 1100})
		d.lixeiraApaga(w, s, protocol.Header{}, ruim[:len(ruim)-4])
		if e.Carry[0].Empty() {
			t.Error("apagou com o pacote malformado")
		}
	})

	apagados, recusas := resultadoDaLixeira(t, c)
	if apagados != 0 || len(recusas) != 0 {
		t.Errorf("apagados=%d recusas=%d; o pacote ruim nao produz nem uma coisa nem outra",
			apagados, len(recusas))
	}
}
