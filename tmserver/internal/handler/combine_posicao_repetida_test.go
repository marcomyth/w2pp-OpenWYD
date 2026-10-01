package handler

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A DUPLICAÇÃO QUE ESTES TESTES FECHAM.
//
// Cada célula do pacote de combinação era conferida sozinha: o slot cabe no
// inventário, e o item descrito bate com o que está lá. Duas células apontando para o
// MESMO slot passam nas duas conferências — as duas descrevem o item de verdade.
//
// O estrago vinha depois: a receita recebia DOIS itens, e o consumo limpava
// `e.Carry[sl]` uma vez por posição ativa — o mesmo slot, duas vezes, o que custa UM
// item. Uma Safira paga, duas contadas.
//
// O cliente nunca manda isso: ele monta as células a partir de slots distintos da
// grade. A repetição só chega num pacote forjado.

// servidorDeCombine sobe um mundo com uma família de teste e DEVOLVE O MUNDO, para o
// teste poder olhar a bolsa em vez de deduzir pelo que veio no fio.
//
// OLHAR A BOLSA É O PONTO. "Nada foi consumido" é uma afirmação sobre o estado do
// jogador, e provar isso pela ausência de um pacote seria provar o contrário do que
// interessa: um dia o servidor pode deixar de mandar aquele pacote por outro motivo, e
// o teste continuaria verde com o item sumido.
func servidorDeCombine(t *testing.T, db world.Persistence) (string, func(), *world.World) {
	t.Helper()
	return servidorDeCombineComLog(t, db, io.Discard)
}

// servidorDeCombineComLog é o mesmo, com o log indo para onde o teste quiser.
func servidorDeCombineComLog(t *testing.T, db world.Persistence, saida io.Writer) (string, func(), *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(saida, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fam := CombineFamily{
		Name:  "test",
		Rate:  func([]world.Item) int { return 100 },
		Apply: func([]world.Item) world.Item { return world.Item{Index: 9999} },
	}
	d := New(Config{Log: log, CombineFamilies: map[protocol.Type]CombineFamily{protocol.MsgCombineItem: fam}})
	w := world.New(world.Config{GridDim: 16}, log, db, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor nao parou")
		}
	}, w
}

// esperaARespostaDoCombine lê até o CombineComplete e devolve o veredito.
//
// SEM ISTO OS TESTES DESTE ARQUIVO MENTEM, e eu descobri do jeito certo — um deles
// falhou. O send só entrega o pacote ao socket; o servidor processa noutra goroutine.
// Olhar a bolsa logo depois de mandar é olhar ANTES de o servidor agir, e aí um teste
// que espera "o item continua lá" passa mesmo que o servidor o tivesse consumido um
// milissegundo depois. Passaria pela razão errada, que é a pior forma de passar.
//
// Esperar o CombineComplete é o ponto de sincronia que o próprio servidor oferece: ele
// sai depois de a máquina ter decidido tudo.
func esperaARespostaDoCombine(t *testing.T, c net.Conn) int32 {
	t.Helper()
	p, _, _ := readOutcome(t, c)
	return parmOf(t, p)
}

// pacoteForjado monta o pacote que o cliente nunca manda: duas células no MESMO slot.
//
// AS DUAS DESCREVEM O ITEM DE VERDADE daquele slot, e é isso que torna o ataque
// possível — um pacote com item mentiroso já era barrado pelo sameItem.
func pacoteForjado(t *testing.T, c net.Conn, ty protocol.Type, slot int, indice int16) {
	t.Helper()
	var body protocol.MsgCombineItemBody
	body.Item[0] = protocol.WireItem{Index: indice}
	body.InvenPos[0] = uint8(slot)
	body.Item[1] = protocol.WireItem{Index: indice}
	body.InvenPos[1] = uint8(slot)
	send(t, c, ty, body.Encode())
}

// bolsaDoHeroi lê um slot da bolsa dentro do laço do mundo.
func bolsaDoHeroi(t *testing.T, w *world.World, slot int) world.Item {
	t.Helper()
	var it world.Item
	noLacoDoMundo(t, w, func(w *world.World) {
		e := w.Entity(1)
		if e == nil {
			t.Fatal("o heroi nao esta no mundo")
		}
		it = e.Carry[slot]
	})
	return it
}

// TestPosicaoRepetidaERecusadaNaMaquinaGenerica cobre o caminho do combineItem, por
// onde passa a maioria das máquinas.
func TestPosicaoRepetidaERecusadaNaMaquinaGenerica(t *testing.T) {
	addr, stop, w := servidorDeCombine(t, combineDB())
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	antes := bolsaDoHeroi(t, w, 0)
	if antes.Index != 1100 {
		t.Fatalf("a bolsa comecou errada: slot 0 = %d, queria 1100", antes.Index)
	}

	pacoteForjado(t, c, protocol.MsgCombineItem, 0, 1100)
	if parm := esperaARespostaDoCombine(t, c); parm != combineInvalid {
		t.Errorf("o servidor respondeu %d; a recusa tinha de ser combineInvalid(%d)", parm, combineInvalid)
	}

	// O ITEM TEM DE CONTINUAR LÁ. Esta é a asserção que importa: se ele sumiu, a
	// combinação aconteceu, e uma combinação que acontece com uma entrada só é
	// exatamente a duplicação.
	depois := bolsaDoHeroi(t, w, 0)
	if depois.Index != 1100 {
		t.Fatalf("o slot 0 virou %d: o item foi CONSUMIDO num pacote que devia ser recusado",
			depois.Index)
	}
	// E o resultado não pode ter nascido em lugar nenhum da bolsa.
	for slot := 0; slot < 8; slot++ {
		if bolsaDoHeroi(t, w, slot).Index == 9999 {
			t.Fatalf("o item resultado apareceu no slot %d: a combinacao rodou", slot)
		}
	}
}

// TestPosicaoRepetidaERecusadaNoEhre cobre o OUTRO ponto de chamada do mesmo
// conferidor, o das máquinas variantes.
//
// DUAS FAMÍLIAS DE PROPÓSITO, e não por zelo: o conferidor é compartilhado, mas é
// chamado de três lugares diferentes (a máquina genérica, o Odin e as variantes).
// Testar só um deixaria os outros dois sem rede — e foi por um caminho não coberto que
// o furo existiu por tanto tempo.
func TestPosicaoRepetidaERecusadaNoEhre(t *testing.T) {
	// O LOG É A ÚNICA PROVA HONESTA AQUI, e eu descobri isso plantando o defeito de
	// volta: sem o conserto, o Ehre AINDA recusa este pacote — por não achar receita —
	// e a bolsa fica intacta do mesmo jeito. Ou seja, um teste que olhasse só o
	// veredito e a bolsa passava com o furo aberto. O que distingue "recusado porque a
	// posição repetiu" de "recusado porque a receita não existe" é a linha de log, que
	// só o conserto escreve.
	var registro bytes.Buffer
	addr, stop, w := servidorDeCombineComLog(t, combineDB(), &registro)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	pacoteForjado(t, c, protocol.MsgCombineItemEhre, 1, 2442)
	if parm := esperaARespostaDoCombine(t, c); parm != combineInvalid {
		t.Errorf("o Ehre respondeu %d; a recusa tinha de ser combineInvalid(%d)", parm, combineInvalid)
	}
	if got := bolsaDoHeroi(t, w, 1).Index; got != 2442 {
		t.Fatalf("o slot 1 virou %d: o Ehre consumiu num pacote que devia ser recusado", got)
	}

	// Parar o servidor antes de ler garante que tudo já foi escrito.
	c.Close()
	stop()
	escrito := registro.String()
	if !strings.Contains(escrito, "combine: posicao repetida") {
		t.Fatalf("o Ehre nao recusou pela posicao repetida; recusou por outro motivo. Log: %s", escrito)
	}
	// A LINHA, E NÃO O LOG INTEIRO. Olhar o log todo dá falso positivo: em nível
	// Debug ele tem portas, contagens de bytes e horários, e o índice do item aparece
	// ali por coincidência — foi o que aconteceu na primeira versão deste teste.
	linha := linhaDoLog(escrito, "combine: posicao repetida")
	if linha == "" {
		t.Fatalf("nao achei a linha no log. Log: %s", escrito)
	}
	// A máquina tem de estar nomeada, senão o log não serve para achar o caso.
	if !strings.Contains(linha, "familia=Ehre") {
		t.Errorf("a linha nao diz qual maquina foi: %s", linha)
	}
	// E tem de dizer de quem e onde, que é o que serve para achar a pessoa.
	for _, chave := range []string{"conn=", "slot=", "celula="} {
		if !strings.Contains(linha, chave) {
			t.Errorf("a linha nao tem %q: %s", chave, linha)
		}
	}
	// E NÃO PODE DIZER QUE ITEM É: um log de anti-fraude precisa de quem e onde; o
	// item não acrescenta nada e só espalha dado do jogador por mais um arquivo.
	if strings.Contains(linha, "2442") {
		t.Errorf("a linha vazou o item: %s", linha)
	}
}

// TestDuasPosicoesDIFERENTESContinuamPassando é a outra metade, e sem ela o conserto
// não estaria provado.
//
// UM CONSERTO QUE RECUSA TUDO TAMBÉM FAZ OS TESTES ACIMA PASSAREM. Este teste é o que
// separa "recusa o pacote forjado" de "quebrou a combinação para todo mundo": duas
// células em slots DIFERENTES são o uso normal, e têm de continuar funcionando.
func TestDuasPosicoesDIFERENTESContinuamPassando(t *testing.T) {
	addr, stop, w := servidorDeCombine(t, combineDB())
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	combineFrame(t, c) // slots 0 e 1, itens 1100 e 2442 — o pacote legítimo
	if parm := esperaARespostaDoCombine(t, c); parm != combineSuccess {
		t.Fatalf("o combine legitimo respondeu %d, queria sucesso(%d)", parm, combineSuccess)
	}

	// Com a taxa em 100 a combinação tem de ter acontecido: as duas entradas somem e
	// o resultado nasce.
	if got := bolsaDoHeroi(t, w, 0).Index; got != 9999 {
		t.Errorf("o resultado nao apareceu no slot 0: achei %d, queria 9999", got)
	}
	if got := bolsaDoHeroi(t, w, 1).Index; got != 0 {
		t.Errorf("o slot 1 nao foi consumido: achei %d, queria vazio", got)
	}
}

// linhaDoLog devolve a primeira linha do log que contém marca.
func linhaDoLog(log, marca string) string {
	for _, l := range strings.Split(log, "\n") {
		if strings.Contains(l, marca) {
			return l
		}
	}
	return ""
}
