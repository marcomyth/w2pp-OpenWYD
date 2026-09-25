package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/acesso"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// startServerRMT sobe um servidor com o mercado no estado pedido.
//
// Harness próprio porque os outros abrem o mercado para os testes de venda funcionarem, e
// aqui o estado É o que está sendo testado. Um teste da trava que rodasse no harness
// aberto passaria dizendo o contrário do que mediu.
func startServerRMT(t *testing.T, persist world.Persistence, estado acesso.EstadoRMT) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) },
		CombatRules: regraSemEscala(), RMT: estado})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, persist, d.Handle)
	w.SetSessionEndHandler(d.SessionEnd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		_ = ln.Close()
		<-done
	}
}

// mandaAbrirRMT pede a barraca com uma prateleira em dinheiro real.
func mandaAbrirRMT(t *testing.T, c net.Conn) {
	t.Helper()
	abrir := protocol.LojaAbrirBody{Titulo: "Barraca"}
	for i := range abrir.Slots {
		abrir.Slots[i].CargoPos = -1
	}
	abrir.Slots[0] = protocol.LojaAbrirSlot{
		CargoPos: 0, Moeda: protocol.LojaMoedaRMT, Preco: store.PrecoMinimoRMTCentavos}
	send(t, c, protocol.MsgLojaAbrir, abrir.Encode())
}

// COM O MERCADO FECHADO, A BARRACA EM DINHEIRO REAL NÃO SOBE.
//
// Este é o estado PADRÃO, e é o que vale em produção a partir deste PR: a Hanna decidiu
// lançar o mercado depois, numa atualização do launcher. Antes disto nada impedia anunciar
// — a mensagem de "ainda não está aberta" só existia enquanto o cobrador não estava
// ligado, e quando ele ligou a porta ficou aberta sem ninguém decidir isso.
func TestFechadoRecusaMontarBarracaRMT(t *testing.T) {
	const item = int16(1030)
	addr, stop := startServerRMT(t, autotradeDB(item), acesso.RMTFechado)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	mandaAbrirRMT(t, vendedor)

	// A recusa é uma mensagem ao jogador, e a barraca NÃO sobe.
	if !recebeu(t, vendedor, msgRMTNaoEstaAberta) {
		t.Error("nao recebeu a recusa do mercado fechado")
	}
}

// E A COMPRA TAMBÉM NÃO PASSA — medido com uma barraca DE PÉ e um comprador de verdade.
//
// Não é redundante com o teste de cima, e a diferença vale dinheiro: um anúncio aberto
// antes do fechamento continua na vitrine, e um cliente remendado pode mandar a compra de
// um anúncio que descobriu de outro jeito. Travar só a montagem deixaria o que já estava
// dentro comprável, e aí entra dinheiro real num mercado que a Hanna decidiu fechar.
//
// O CENÁRIO É O ESTADO "staff", e é o único jeito de montar isto honestamente: com o
// mercado FECHADO ninguém consegue subir a barraca, então não haveria o que comprar. Aqui
// o vendedor tem cargo e anuncia; o comprador é jogador comum e leva a recusa.
func TestCompraRMTRecusadaParaQuemNaoPodeUsarOMercado(t *testing.T) {
	const item = int16(1030)
	db := autotradeDB(item)
	// O vendedor é da casa; o comprador, não. O papel vem do login, e não de nada que o
	// cliente mande.
	db.accounts["tester"].role = "admin"
	db.accounts["tradeb"].role = "player"

	falso := comCobradorFalso(t)
	addr, stop := startServerRMT(t, db, acesso.RMTStaff)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	barraca := abreBarraca(t, vendedor, "Loja", 0, store.PrecoMinimoRMTCentavos,
		protocol.LojaMoedaRMT)
	drena(t, comprador)

	compra := protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaRMT}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())

	// A PROVA É O COBRADOR NÃO TER SIDO CHAMADO, e não a frase.
	//
	// Eu tinha escrito este teste só olhando a mensagem, e ele PASSAVA com a trava
	// desligada: sem cobrador montado, o caminho da cobrança manda a MESMA frase
	// (ErrPixNaoLigado). Os dois caminhos eram indistinguíveis pelo que chega ao cliente,
	// e o teste dizia "trava funcionando" enquanto media outra coisa. Descobri sabotando.
	//
	// Com um cobrador montado, a diferença fica nítida: se a trava falhar, a cobrança é
	// PEDIDA, e é isso que se conta aqui.
	if falso.chamadas != 0 {
		t.Errorf("a cobranca foi pedida %d vez(es) com o mercado fechado para o comprador",
			falso.chamadas)
	}
	if !recebeu(t, comprador, msgRMTNaoEstaAberta) {
		t.Error("o comprador comum nao levou a recusa do mercado fechado")
	}
}

// cobradorQueContra registra se alguém pediu uma cobrança. Não abre nada: o teste que o
// usa quer saber se o caminho FOI percorrido, e não o que ele produziria.
type cobradorQueContra struct{ chamadas int }

func (c *cobradorQueContra) AbrirCobranca(context.Context, int64, int64, string) (CobrancaAberta, error) {
	c.chamadas++
	return CobrancaAberta{CobrancaID: 1, CodigoPix: "00020126BR",
		ExpiraEm: time.Now().Add(JanelaDeCobranca)}, nil
}

// comCobradorFalso monta o cobrador e o desmonta no fim.
//
// A variável é de PACOTE (cobradorPix), então trocá-la sem devolver ao que era vazaria para
// os outros testes do pacote — e um teste que muda estado global sem limpar é o que faz
// outro falhar na ordem errada.
func comCobradorFalso(t *testing.T) *cobradorQueContra {
	t.Helper()
	antes := cobradorPix
	falso := &cobradorQueContra{}
	cobradorPix = falso
	t.Cleanup(func() { cobradorPix = antes })
	return falso
}

// COM "staff", JOGADOR COMUM NÃO PASSA.
//
// É o estado que a equipe vai usar para testar antes de abrir. Se ele deixasse jogador
// passar, o teste da equipe seria o lançamento sem ninguém ter decidido.
func TestStaffRecusaJogadorComum(t *testing.T) {
	const item = int16(1030)
	addr, stop := startServerRMT(t, autotradeDB(item), acesso.RMTStaff)
	defer stop()
	// enterWorldAs entra como conta comum: o papel vem do banco no login, e o harness não
	// dá cargo a ninguém.
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	mandaAbrirRMT(t, vendedor)

	if !recebeu(t, vendedor, msgRMTNaoEstaAberta) {
		t.Error("jogador comum passou com o mercado em staff")
	}
}

// COM "aberto", A BARRACA SOBE.
//
// O espelho dos de cima, e ele é o que impede o pior defeito possível deste PR: uma trava
// que recusa SEMPRE. Sem este teste, um erro no predicado passaria como "trava
// funcionando" — os três testes de recusa ficariam verdes e o mercado nunca abriria.
func TestAbertoDeixaMontarBarracaRMT(t *testing.T) {
	const item = int16(1030)
	addr, stop := startServerRMT(t, autotradeDB(item), acesso.RMTAberto)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	mandaAbrirRMT(t, vendedor)

	// MsgLojaAbriu é a prova de que subiu.
	readUntil(t, vendedor, protocol.MsgLojaAbriu)
}
