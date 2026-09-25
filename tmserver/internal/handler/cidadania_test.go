package handler

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// ouviuDoServidor espera uma linha de texto do servidor que contenha `pedaco`.
//
// Ler a MENSAGEM, e nao so o estado, e o que prova a divergencia que a Hanna pediu:
// no legado quem nao pode pagar nao recebe resposta nenhuma.
func ouviuDoServidor(t *testing.T, c net.Conn, pedaco string) bool {
	t.Helper()
	_, _, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		return (h.Type == protocol.MsgMessageChat || h.Type == protocol.MsgMessagePanel) &&
			strings.Contains(decodePanel(p), pedaco)
	})
	return ok
}

// A CIDADANIA VEM ANTES DA ALMA, que é a regra do legado: um case só, a cidadania
// primeiro, e a Alma como continuação de quem não foi atendido.
func TestACidadaniaEhCobradaEGravada(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Citizen = 0
		e.Coin = cidadaniaCusto + 500

		if !d.comprouCidadania(w, s, e) {
			t.Fatal("a cidadania nao atendeu quem podia pagar; o pedido teria caido na Alma")
		}
		if e.Citizen != uint8(d.serverIndex+1) {
			t.Errorf("Citizen = %d, queria %d (o indice do servidor mais um)", e.Citizen, d.serverIndex+1)
		}
		if e.Coin != 500 {
			t.Errorf("ouro = %d, queria 500 (cobrou %d?)", e.Coin, cidadaniaCusto)
		}
	})
}

// SEM OURO NÃO COBRA E NÃO DÁ, e — divergência pedida pela Hanna — não fica calado.
func TestSemOuroNaoViraCidadao(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Citizen = 0
		e.Coin = cidadaniaCusto - 1

		// A COMPRA NAO ACONTECE e, o que importa, ela NAO se declara atendida: e o
		// que deixa a Alma ser tentada depois, como no legado.
		if d.comprouCidadania(w, s, e) {
			t.Fatal("disse que vendeu a cidadania para quem nao tinha ouro")
		}
		// O clique inteiro, pelo caminho que o jogador usa.
		d.kibita(w, s, e)
		if e.Citizen != 0 {
			t.Error("virou cidadao sem pagar")
		}
		if e.Coin != cidadaniaCusto-1 {
			t.Errorf("mexeu no ouro de quem nao pagou: %d", e.Coin)
		}
	})
	if !ouviuDoServidor(t, c, "4000000") {
		t.Error("nao disse quanto custa; no legado este caso e silencioso, e a Hanna pediu para avisar")
	}
}

// QUEM JÁ É CIDADÃO NÃO PAGA DE NOVO, e ouve QUAL cidadania tem — é o que diz se
// vale usar o /tirarcidadania.
func TestQuemJaEhCidadaoNaoPagaDeNovo(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Citizen = 9 // de outro canal
		e.Coin = cidadaniaCusto * 2

		if d.comprouCidadania(w, s, e) {
			t.Fatal("vendeu cidadania para quem ja era cidadao")
		}
		d.kibita(w, s, e)
		if e.Citizen != 9 {
			t.Errorf("trocou a cidadania de quem ja tinha: %d", e.Citizen)
		}
		if e.Coin != cidadaniaCusto*2 {
			t.Errorf("cobrou de quem ja era cidadao: %d", e.Coin)
		}
	})
	// A frase do legado que nunca era usada por este ramo (514).
	if !ouviuDoServidor(t, c, "outro servidor") {
		t.Error("nao disse que a cidadania e de outro servidor")
	}
}

// O COMANDO ZERA, E NÃO DEVOLVE O OURO — igual ao legado. Sem ele a cidadania seria
// porta de mao unica, porque a compra exige Citizen == 0.
func TestTirarCidadaniaZeraSemDevolverOuro(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Citizen = uint8(d.serverIndex + 1)
		e.Coin = 1000

		d.tirarCidadania(w, s)

		if e.Citizen != 0 {
			t.Errorf("Citizen = %d, queria 0", e.Citizen)
		}
		if e.Coin != 1000 {
			t.Errorf("devolveu ouro: %d", e.Coin)
		}
		// E depois de zerar, dá para comprar de novo: é o caminho de troca de canal.
		e.Coin = cidadaniaCusto
		if !d.comprouCidadania(w, s, e) || e.Citizen == 0 {
			t.Error("depois de tirar, nao consegue virar cidadao de novo")
		}
	})
}

// E QUEM NÃO TEM CIDADANIA OUVE ISSO ao usar o comando, em vez de nada acontecer.
func TestTirarCidadaniaSemTerNaoQuebra(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Citizen = 0
		d.tirarCidadania(w, s)
		if e.Citizen != 0 {
			t.Errorf("Citizen = %d", e.Citizen)
		}
	})
}
