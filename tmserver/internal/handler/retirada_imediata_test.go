package handler

import (
	"context"
	"io"
	"log/slog"
	"testing"

	gamev1 "github.com/jeanluca/w2pp-openwyd/api/game/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/control"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A RETIRADA IMEDIATA, provada contra o servidor montado.
//
// O teste vive aqui e não no pacote control porque o que importa é o efeito no
// baú VIVO de uma sessão de verdade, e é neste pacote que existe uma sessão de
// verdade. O control sozinho só consegue provar mundo vazio.

// controlDeTeste monta o servidor de controle contra o mundo que o teste já tem
// de pé.
func controlDeTeste(t *testing.T, w *world.World) *control.Server {
	t.Helper()
	s, err := control.NewServer(w, "segredo-de-teste",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(*world.World, *world.Session, int16, int16) {}, control.Overlays{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

// O ITEM VENDIDO SAI NA HORA, sem o vendedor precisar sair e voltar.
//
// A marca do escrow deixa o item inerte, então tirá-lo tarde não é risco — é
// DESCONFORTO, e o desconforto é a razão de existir disto. O vendedor vê no baú
// um item que já não é dele, já pago e já entregue a outra pessoa. Ele conta os
// itens, conclui que sumiu um, e abre chamado sobre um sistema que está certo.
func TestRetiradaImediataTiraOItemVendidoSemRelogin(t *testing.T) {
	// O banco só passa a dizer "o slot 3 vendeu" DEPOIS de o vendedor estar em
	// jogo. Com a lista já preenchida no login, o item sairia lá e não haveria o
	// que retirar aqui — o teste mediria a retirada do login, não esta.
	db := bancoComVendido(3, nil)
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)
	db.defineSlotsVendidos(7, []int16{3})

	// Antes: o item vendido ainda está lá, porque o login já passou.
	if bau := bauDoVendedor(t, w); bau.Items[3].Empty() {
		t.Fatal("o cenario nao se montou: o item ja tinha saido")
	}

	resp, err := controlDeTeste(t, w).SettleRmtSaleNow(context.Background(),
		&gamev1.SettleRmtSaleNowRequest{AccountName: "tester"})
	if err != nil {
		t.Fatalf("SettleRmtSaleNow: %v", err)
	}

	if !resp.GetFound() {
		t.Error("nao achou o vendedor que esta em jogo")
	}
	if resp.GetRemoved() != 1 {
		t.Errorf("retirou %d item(ns), quero 1", resp.GetRemoved())
	}
	bau := bauDoVendedor(t, w)
	if !bau.Items[3].Empty() {
		t.Errorf("o slot 3 ainda tem o item %d, que ja foi pago e entregue a outra pessoa",
			bau.Items[3].Index)
	}
	if bau.Items[5].Empty() {
		t.Error("o vizinho sem marca saiu junto")
	}
	// O AVISO NÃO É CORTESIA. Sem ele o vendedor vê um espaço a mais no baú, acha
	// que sumiu um item, e abre chamado — e quem atender não vai ter o que olhar.
	if !recebeu(t, c, "1 item(ns) que você vendeu por dinheiro real saíram do baú.") {
		t.Error("o item saiu e o vendedor nao foi avisado")
	}
}

// SLOT SEM MARCA NÃO É ESVAZIADO, mesmo que o banco o nomeie.
//
// A lista vem de FORA do laço e o baú pode ter mudado entre a leitura e a
// passada. Apagar um item que não era o vendido seria tirar do jogador uma coisa
// que ele nunca vendeu — o erro mais caro deste caminho, porque é silencioso e
// não tem volta.
func TestRetiradaImediataNaoEsvaziaSlotSemMarca(t *testing.T) {
	db := bancoComVendido(3, nil)
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)
	db.defineSlotsVendidos(7, []int16{5}) // o 5 tem item, mas nao tem marca

	resp, err := controlDeTeste(t, w).SettleRmtSaleNow(context.Background(),
		&gamev1.SettleRmtSaleNowRequest{AccountName: "tester"})
	if err != nil {
		t.Fatalf("SettleRmtSaleNow: %v", err)
	}

	if resp.GetRemoved() != 0 {
		t.Errorf("retirou %d item(ns) de um slot sem marca", resp.GetRemoved())
	}
	bau := bauDoVendedor(t, w)
	if bau.Items[5].Empty() {
		t.Error("esvaziou o slot 5, que nao tinha marca nenhuma")
	}
	if bau.Items[3].Empty() {
		t.Error("esvaziou o slot 3, que o banco NAO nomeou")
	}
	// E sem retirada, sem aviso: avisar aqui faria o vendedor achar que vendeu.
	if recebeu(t, c, "0 item(ns) que você vendeu por dinheiro real saíram do baú.") {
		t.Error("avisou sem ter retirado nada")
	}
}

// Sem venda pendente: acha o vendedor, retira zero, e não fala com ele.
//
// É o caso mais comum de todos, porque a chamada roda por confirmação de
// pagamento e a maioria das contas não tem nada pendente.
func TestRetiradaImediataSemVendaNaoFalaComOVendedor(t *testing.T) {
	db := bancoComVendido(3, nil)
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	resp, err := controlDeTeste(t, w).SettleRmtSaleNow(context.Background(),
		&gamev1.SettleRmtSaleNowRequest{AccountName: "tester"})
	if err != nil {
		t.Fatalf("SettleRmtSaleNow: %v", err)
	}

	if !resp.GetFound() || resp.GetRemoved() != 0 {
		t.Errorf("found=%v removed=%d; quero achado e zero", resp.GetFound(), resp.GetRemoved())
	}
	for {
		ty, _, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgMessagePanel {
			t.Error("falou com o vendedor sem ter retirado nada")
		}
	}
	if bau := bauDoVendedor(t, w); bau.Items[3].Empty() {
		t.Error("retirou o item sem o banco dizer que a venda aconteceu")
	}
}

// Vendedor fora do jogo NÃO é falha: a marca fica, o item continua inerte, e o
// próximo login faz o mesmo trabalho. Devolver erro aqui poria vermelho no painel
// por uma coisa que ninguém fez de errado.
func TestRetiradaImediataDeVendedorForaDoJogoNaoEErro(t *testing.T) {
	_, stop, w := startServerNovato(t, bancoComVendido(3, []int16{3}))
	defer stop()

	resp, err := controlDeTeste(t, w).SettleRmtSaleNow(context.Background(),
		&gamev1.SettleRmtSaleNowRequest{AccountName: "tester"})
	if err != nil {
		t.Fatalf("SettleRmtSaleNow: %v", err)
	}

	if resp.GetFound() {
		t.Error("achou um vendedor que nao esta em jogo")
	}
	if resp.GetRemoved() != 0 {
		t.Errorf("retirou %d item(ns) de quem nao esta em jogo", resp.GetRemoved())
	}
}
