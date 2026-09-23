package handler

import (
	"bytes"
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const anuncioVendidoDeTeste = int64(4242)

// bancoComVendido monta a conta do vendedor com o item vendido ainda no baú,
// marcado, e o banco dizendo qual slot esvaziar.
func bancoComVendido(slotMarcado int16, slotsQueOBancoDiz []int16) *fakeDB {
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7: {Slot: 0, Name: "Vendedor", Level: 1, HP: 1000, MaxHP: 1000},
	}
	var cargo world.CargoState
	cargo.Items[slotMarcado] = world.Item{Index: 1030, AnuncioRMT: anuncioVendidoDeTeste}
	cargo.Items[5] = world.Item{Index: 2020} // um vizinho sem marca, para provar que ele fica
	db.accounts["tester"].cargo = cargo
	db.slotsVendidos = map[int64][]int16{7: slotsQueOBancoDiz}
	return db
}

// bauDoVendedor lê o baú DE DENTRO do laço, que é quem é dono dele. Ler de fora
// passaria nos testes e mentiria sobre o desenho.
func bauDoVendedor(t *testing.T, w *world.World) world.CargoState {
	t.Helper()
	var copia world.CargoState
	achou := false
	noLacoDoMundo(t, w, func(w *world.World) {
		if c := w.Cargo(7); c != nil {
			copia, achou = *c, true
		}
	})
	if !achou {
		t.Fatal("o bau da conta nao foi carregado")
	}
	return copia
}

// entraNaConta faz só o login de CONTA, que é onde o baú é carregado e onde a
// retirada acontece. Não entra em jogo de propósito: o `enterWorldAs` comum
// engoliria o aviso ao passar para a tela de personagem, e é justamente o aviso
// que estes testes querem ver.
func entraNaConta(t *testing.T, addr string) net.Conn {
	t.Helper()
	c := dial(t, addr)
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
	if ty, _ := read(t, c); ty != protocol.MsgCNFAccountLogin {
		t.Fatalf("login de conta falhou: %#x", ty)
	}
	return c
}

// O item vendido sai do baú no login, e o vendedor é avisado.
//
// A marca do escrow segurou o item intocável desde que o pagamento entrou. Aqui
// ela cumpre a última função dela: dizer qual slot esvaziar quando o dono
// finalmente aparece.
//
// O aviso não é cortesia. Sem ele o vendedor conta os itens, acha que sumiu um, e
// abre chamado — e quem for atender não vai ter o que olhar, porque está tudo
// certo.
func TestItemVendidoSaiDoBauNoLogin(t *testing.T) {
	addr, stop, w := startServerNovato(t, bancoComVendido(3, []int16{3}))
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()

	avisado := recebeu(t, c, "1 item(ns) que você vendeu por dinheiro real saíram do baú.")

	bau := bauDoVendedor(t, w)
	if !bau.Items[3].Empty() {
		t.Errorf("o slot 3 ainda tem item %d; a venda ja tinha sido paga e entregue",
			bau.Items[3].Index)
	}
	if bau.Items[5].Empty() {
		t.Error("o vizinho sem marca sumiu junto")
	}
	if !avisado {
		t.Error("o item saiu e o vendedor nao foi avisado")
	}
}

// E o que o teste de cima não prova sozinho: sem venda nenhuma, NADA sai.
//
// É o caso que impede um conserto preguiçoso — limpar o slot sempre que o banco
// mandar uma lista, ou pior, limpar por engano — de passar por correto.
func TestSemVendaNadaSaiDoBau(t *testing.T) {
	addr, stop, w := startServerNovato(t, bancoComVendido(3, nil))
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()
	drena(t, c)

	bau := bauDoVendedor(t, w)
	if bau.Items[3].Empty() {
		t.Error("o item marcado saiu sem o banco ter dito que a venda aconteceu")
	}
	if bau.Items[5].Empty() {
		t.Error("o item sem marca saiu")
	}
}

// O SLOT SEM MARCA NÃO É ESVAZIADO, mesmo que o banco o nomeie.
//
// A lista é lida FORA do laço, e entre a leitura e a passada o slot pode ter
// mudado. Apagar um item que não era o vendido seria tirar do jogador uma coisa
// que ele nunca vendeu — o erro mais caro que esta função pode cometer, porque é
// silencioso e não tem volta.
func TestSlotSemMarcaNaoEEsvaziado(t *testing.T) {
	addr, stop, w := startServerNovato(t, bancoComVendido(3, []int16{5}))
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()
	drena(t, c)

	bau := bauDoVendedor(t, w)
	if bau.Items[5].Empty() {
		t.Error("o slot 5 nao tinha marca nenhuma e foi esvaziado assim mesmo")
	}
	if bau.Items[3].Empty() {
		t.Error("o slot 3, que o banco NAO nomeou, foi esvaziado")
	}
}

// Slot fora da faixa não derruba nada. Vem do banco como número, e número de
// fora não pode virar pânico dentro do laço que é dono do mundo inteiro.
func TestSlotForaDaFaixaNaoDerruba(t *testing.T) {
	addr, stop, w := startServerNovato(t, bancoComVendido(3, []int16{-1, 9999, 3}))
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()
	drena(t, c)

	bau := bauDoVendedor(t, w)
	if !bau.Items[3].Empty() {
		t.Error("o slot valido da lista nao foi esvaziado")
	}
}

// Repetir a retirada é normal, e a segunda vez não pode avisar.
//
// A lista de slots é lida FORA do laço e pode chegar depois de o item já ter
// saído — o save da passada anterior alcançou o banco, a consulta não. A segunda
// passada encontra o slot VAZIO e não faz nada. Se avisasse assim mesmo, o
// vendedor leria duas vezes que vendeu, e acharia que vendeu dois.
func TestSlotJaVazioNaoAvisa(t *testing.T) {
	db := bancoComVendido(3, []int16{9}) // o 9 nunca teve nada
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()

	if recebeu(t, c, "1 item(ns) que você vendeu por dinheiro real saíram do baú.") {
		t.Error("avisou por um slot que já estava vazio")
	}
	bau := bauDoVendedor(t, w)
	if bau.Items[3].Empty() || bau.Items[5].Empty() {
		t.Error("um slot que o banco nao nomeou foi esvaziado")
	}
}

// O aviso chega CODIFICADO para o cliente, como qualquer mensagem de painel — é o
// mesmo cuidado do aviso do escrow: comparar contra o texto em Go passaria mesmo
// se a conversão estivesse quebrada.
func TestAvisoDaRetiradaChegaCodificado(t *testing.T) {
	addr, stop, _ := startServerNovato(t, bancoComVendido(3, []int16{3}))
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()

	esperado := protocol.EncodeMessagePanelBody(
		"1 item(ns) que você vendeu por dinheiro real saíram do baú.")
	achou := false
	for {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgMessagePanel && bytes.Equal(payload, esperado) {
			achou = true
		}
	}
	if !achou {
		t.Error("o aviso nao chegou com os bytes que o cliente sabe ler")
	}
}
