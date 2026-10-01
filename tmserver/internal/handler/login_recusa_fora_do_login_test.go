package handler

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O QUE ESTES TESTES GUARDAM: que a recusa de login NÃO DESFAÇA UMA SESSÃO QUE JÁ É DE
// ALGUÉM.
//
// O fechamento atrasado da recusa zera a conta e o modo da sessão — é o que deixa o socket
// de pé sem ele ser de ninguém. Isso só é verdade para quem ainda está na tela de login. A
// checagem de VERSÃO roda antes da checagem de modo, então um 0x20D com a versão errada
// chega a ela em qualquer estado, e a primeira versão do conserto zerava a sessão de quem
// já estava JOGANDO:
//
//   - o World só grava a saída de quem está em UserPlay com conta, e só solta a carga de
//     quem tem conta. Com a sessão zerada, o socket caía pelo prazo SEM GRAVAR o
//     personagem nem a carga. Na main o mesmo pacote grava: lá é texto e Close na hora;
//   - e, com o socket ainda de pé, um login certo recarregava a conta do banco por cima do
//     que não foi gravado: um rollback a pedido do cliente;
//   - com um login ainda no banco, o reset devolvia o modo a UserAccept com a resposta do
//     primeiro login por chegar. Ela chegava, a sessão ia para a tela de personagens e o
//     fechamento agendado desistia: a recusa não recusava nada.
//
// A regra que os três trancam: fora da tela de login a recusa manda o texto e fecha NA
// HORA, pelo Close de sempre, com a sessão intacta.

// gravacoesDaSaida conta quantas gravações de personagem e de carga o banco já recebeu.
func gravacoesDaSaida(db *fakeDB) (personagens, cargas int) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return len(db.savedChars), len(db.savedCargos)
}

// esperaGravacoes espera a gravação de saída, que acontece FORA do laço: o socket cai
// antes de ela confirmar, então olhar o contador logo depois do fim de fio é uma corrida.
// Devolve o que achou quando os dois contadores andaram, ou quando o prazo acabou.
func esperaGravacoes(db *fakeDB, antesP, antesC int, espera time.Duration) (personagens, cargas int) {
	limite := time.Now().Add(espera)
	for {
		personagens, cargas = gravacoesDaSaida(db)
		if (personagens > antesP && cargas > antesC) || time.Now().After(limite) {
			return personagens, cargas
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// leAteCair lê pacotes até o socket morrer ou o prazo acabar, e diz qual dos dois foi.
//
// NÃO É O pacotesAteFechar, e de propósito: aquele derruba o teste no prazo estourado, e
// aqui o teste precisa seguir para dizer O QUE o socket vivo deixou acontecer. E ele
// aceita qualquer erro que não seja prazo como fim de fio: quando o servidor fecha com um
// pacote do cliente ainda por ler, o que chega é um RST, e o nome desse erro muda de
// sistema para sistema. O que não muda é que prazo estourado quer dizer socket VIVO.
func leAteCair(t *testing.T, c net.Conn, espera time.Duration) (tipos []protocol.Type, caiu bool) {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(espera)); err != nil {
		t.Fatal(err)
	}
	for {
		var sz [2]byte
		if _, err := io.ReadFull(c, sz[:]); err != nil {
			return tipos, !ehPrazoEstourado(err)
		}
		buf := make([]byte, binary.LittleEndian.Uint16(sz[:]))
		copy(buf, sz[:])
		if _, err := io.ReadFull(c, buf[2:]); err != nil {
			return tipos, !ehPrazoEstourado(err)
		}
		h, _, _, err := protocol.Decode(buf)
		if err != nil {
			t.Fatalf("decodificando: %v", err)
		}
		tipos = append(tipos, h.Type)
	}
}

func chegouNaTelaDePersonagens(tipos []protocol.Type) bool {
	for _, ty := range tipos {
		if ty == protocol.MsgCNFAccountLogin {
			return true
		}
	}
	return false
}

// EM JOGO, um login com a versão errada: o socket cai e a saída é GRAVADA.
//
// É o teste do defeito mais caro. A sessão zerada não era gravada por ninguém — nem o
// personagem, nem a carga —, e o que a pessoa fez desde a última gravação sumia.
func TestARecusaDeVersaoEmJogoGravaASaida(t *testing.T) {
	db := newDB()
	addr, stop := servidorTrancadoComPrazo(t, db, false, 200*time.Millisecond)
	defer stop()
	c := enterWorld(t, addr)
	defer func() { _ = c.Close() }()

	antesP, antesC := gravacoesDaSaida(db)
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", 1234))
	// O texto ainda sai: quem está com o cliente velho tem de saber por que caiu.
	exigeUmSoPainel(t, pacotesAteFechar(t, c, 5*time.Second), msgLoginVersao)

	depoisP, depoisC := esperaGravacoes(db, antesP, antesC, 3*time.Second)
	if depoisP != antesP+1 {
		t.Errorf("gravacoes do personagem: %d -> %d, queria %d -> %d: a recusa de versao "+
			"fechou a sessao de quem estava em jogo SEM gravar o personagem",
			antesP, depoisP, antesP, antesP+1)
	}
	if depoisC != antesC+1 {
		t.Errorf("gravacoes da carga: %d -> %d, queria %d -> %d: a recusa de versao "+
			"fechou a sessao de quem estava em jogo SEM gravar a carga",
			antesC, depoisC, antesC, antesC+1)
	}
}

// EM JOGO, a versão errada e logo atrás um login CERTO no mesmo socket: o segundo não
// pode recarregar a conta do banco.
//
// O PRAZO É LONGO DE PROPÓSITO e a leitura espera menos que ele. Com o fechamento por
// prazo, o socket ficava de pé tempo de sobra para o segundo login passar; fechando na
// hora, o segundo pacote chega a um socket morto e ninguém o lê. Um prazo curto esconderia
// o defeito atrás da sorte de o fechamento chegar primeiro.
//
// Os dois pacotes vão numa escrita só: é como o cliente remendado faria, e tira do teste a
// dúvida de o segundo Write encontrar o socket já fechado.
func TestARecusaDeVersaoEmJogoNaoDeixaRecarregarAConta(t *testing.T) {
	const prazo = 5 * time.Second
	db := newDB()
	addr, stop := servidorTrancadoComPrazo(t, db, false, prazo)
	defer stop()
	c := enterWorld(t, addr)
	defer func() { _ = c.Close() }()

	antesP, antesC := gravacoesDaSaida(db)
	errada, err := protocol.Encode(protocol.Header{Type: protocol.MsgAccountLogin},
		loginBody("tester", "secret", 1234), 7)
	if err != nil {
		t.Fatal(err)
	}
	certa, err := protocol.Encode(protocol.Header{Type: protocol.MsgAccountLogin},
		loginBody("tester", "secret", protocol.AppVersion), 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(append(errada, certa...)); err != nil {
		t.Fatal(err)
	}

	tipos, caiu := leAteCair(t, c, prazo-3*time.Second)
	if !caiu {
		t.Errorf("o socket de quem estava em jogo continuou de pe depois da recusa de "+
			"versao (o prazo e %v): a recusa tinha de fechar na hora", prazo)
	}
	if chegouNaTelaDePersonagens(tipos) {
		t.Error("o login certo no mesmo socket chegou na tela de personagens: a conta foi " +
			"recarregada do banco com o personagem anterior ainda em memoria, e isso e um " +
			"rollback a pedido do cliente")
	}
	// E a saída foi gravada UMA vez: a sessão que estava em jogo saiu pelo caminho normal.
	depoisP, depoisC := esperaGravacoes(db, antesP, antesC, 3*time.Second)
	if depoisP != antesP+1 || depoisC != antesC+1 {
		t.Errorf("gravacoes do personagem %d -> %d e da carga %d -> %d, queria uma a mais "+
			"de cada: a sessao em jogo saiu sem gravar", antesP, depoisP, antesC, depoisC)
	}
}

// bancoComPortao segura o AccountLogin até o teste soltar, e avisa quando o pedido chegou.
//
// É como se põe um login "no banco" de verdade: o Mode fica em UserLogin e a resposta
// fica por chegar. Um sleep no lugar do aviso mediria a pressa da máquina.
type bancoComPortao struct {
	*fakeDB
	chegou chan struct{}
	portao chan struct{}
	solta  *sync.Once
}

func novoBancoComPortao() bancoComPortao {
	return bancoComPortao{fakeDB: newDB(), chegou: make(chan struct{}, 4),
		portao: make(chan struct{}), solta: new(sync.Once)}
}

func (b bancoComPortao) abre() { b.solta.Do(func() { close(b.portao) }) }

func (b bancoComPortao) AccountLogin(ctx context.Context, nome, senha string, epoca int64) (world.LoginOutcome, error) {
	b.chegou <- struct{}{}
	<-b.portao
	return b.fakeDB.AccountLogin(ctx, nome, senha, epoca)
}

// COM UM LOGIN NO BANCO, chega um 0x20D de versão errada: a conexão acaba ALI.
//
// O que o defeito deixava passar: a recusa devolvia o Mode a UserAccept com a resposta do
// primeiro login por chegar. Ela chegava, punha a sessão na tela de personagens, e o
// fechamento agendado olhava a sessão logada e desistia. A pessoa leu "sua versão está
// velha" e entrou assim mesmo, num socket que não ia fechar.
func TestARecusaDeVersaoComLoginNoBancoFechaOSocket(t *testing.T) {
	db := novoBancoComPortao()
	// Solta o portão de qualquer jeito: uma goroutine presa nele sobraria do teste.
	defer db.abre()
	addr, stop := servidorTrancadoComPrazo(t, db, false, 300*time.Millisecond)
	defer stop()
	c := dial(t, addr)
	defer func() { _ = c.Close() }()

	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
	select {
	case <-db.chegou:
	case <-time.After(5 * time.Second):
		t.Fatal("o primeiro login nao chegou ao banco")
	}
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", 1234))
	esperaRecusaDeLogin(t, c, msgLoginVersao)

	// AGORA o banco responde ao primeiro login.
	db.abre()
	tipos, caiu := leAteCair(t, c, 3*time.Second)
	if chegouNaTelaDePersonagens(tipos) {
		t.Error("o banco respondeu e a sessao entrou na tela de personagens depois de ser " +
			"recusada pela versao: a recusa nao recusou")
	}
	if !caiu {
		t.Error("o socket continuou de pe depois da recusa de versao com um login no " +
			"banco (o prazo e 300ms): a conexao nao foi encerrada")
	}
}
