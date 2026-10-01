package handler

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O QUE ESTES TESTES GUARDAM: que a recusa de login FECHE a conexão depois de dizer o
// motivo. O login_recusa_test.go prova que o texto sai; aqui é o socket.
//
// A PRIMEIRA VERSÃO DESTE CONSERTO DEIXAVA O SOCKET ABERTO PARA SEMPRE, e a conta
// bloqueada era o caso. O fechamento é atrasado — a pessoa precisa de tempo para ler — e
// a guarda do fechaDepois só fecha se a sessão ainda não é de ninguém: sem conta e em
// UserAccept. No caminho da conta bloqueada o Mode ainda era UserLogin, posto quando o
// pedido saiu para o dbServer, então a guarda recusava fechar. Era uma regressão PIOR que
// o defeito original: antes a recusa era muda, mas a conexão caía.

// pacoteLido é um pacote como o cliente o recebeu, tipo e corpo.
type pacoteLido struct {
	tipo  protocol.Type
	corpo []byte
}

// pacotesAteFechar lê tudo até o servidor fechar DE VERDADE, e exige o fim de fio.
//
// EXIGE EOF E NÃO PRAZO ESTOURADO, e é essa a diferença que pega o defeito. O teste
// antigo do fechamento punha um prazo de leitura de um segundo e aceitava qualquer erro
// como prova de que a conexão caiu. Como o prazo do fechamento é de dez segundos, o erro
// que ele recebia era um TIMEOUT: o socket estava vivo, e o teste dizia que tinha caído.
// Um teste que aceita "não respondeu" como "fechou" passa igual com o servidor deixando a
// conexão aberta para sempre — foi exatamente o que aconteceu.
func pacotesAteFechar(t *testing.T, c net.Conn, espera time.Duration) []pacoteLido {
	t.Helper()
	var lidos []pacoteLido
	limite := time.Now().Add(espera)
	for {
		if err := c.SetReadDeadline(limite); err != nil {
			t.Fatal(err)
		}
		var sz [2]byte
		if _, err := io.ReadFull(c, sz[:]); err != nil {
			if ehPrazoEstourado(err) {
				t.Fatalf("o servidor NAO fechou a conexao em %v: o socket de uma recusa de "+
					"login ficou aberto. Pacotes lidos ate aqui: %v", espera, tiposDe(lidos))
			}
			if !ehFimDeFio(err) {
				t.Fatalf("lendo o tamanho: %v", err)
			}
			return lidos
		}
		buf := make([]byte, binary.LittleEndian.Uint16(sz[:]))
		copy(buf, sz[:])
		if _, err := io.ReadFull(c, buf[2:]); err != nil {
			t.Fatalf("lendo o corpo: %v", err)
		}
		h, corpo, _, err := protocol.Decode(buf)
		if err != nil {
			t.Fatalf("decodificando: %v", err)
		}
		lidos = append(lidos, pacoteLido{tipo: h.Type, corpo: corpo})
	}
}

func ehPrazoEstourado(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// ehFimDeFio aceita as formas em que um socket morto aparece. O fechamento limpo chega
// como EOF; fechar com bytes ainda a caminho do outro lado vira RST em vez de FIN, e no
// Windows isso não é io.EOF.
func ehFimDeFio(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, net.ErrClosed)
}

func tiposDe(ps []pacoteLido) []string {
	t := make([]string, 0, len(ps))
	for _, p := range ps {
		t = append(t, fmt.Sprintf("%#x", p.tipo))
	}
	return t
}

// exigeUmSoPainel conta os PACOTES: exatamente um 0x0101 com o texto certo, e ZERO 0x0102.
//
// O ZERO É O CORAÇÃO DO CONSERTO. O defeito era o notify mandar um 0x0102 MsgMessageBoxOk
// de quatro bytes ANTES do texto — no legado essa mensagem tem 116 bytes de corpo, e os
// quatro bytes eram um código do nosso próprio iota, que não significa nada para o
// cliente. Conferir "veio um 0x0101" não bastaria: o 0x0101 vinha depois do pacote
// quebrado, e era o quebrado que deixava a tela muda. A prova tem de ser pela AUSÊNCIA.
func exigeUmSoPainel(t *testing.T, lidos []pacoteLido, esperado string) {
	t.Helper()
	var paineis []pacoteLido
	caixas := 0
	for _, p := range lidos {
		switch p.tipo {
		case protocol.MsgMessagePanel:
			paineis = append(paineis, p)
		case protocol.MsgMessageBoxOk:
			caixas++
		}
	}
	if caixas != 0 {
		t.Errorf("veio %d MsgMessageBoxOk (%#x) na recusa de login: e a caixa de tamanho "+
			"errado que o cliente nao sabe ler, e ela deixa a recusa muda",
			caixas, protocol.MsgMessageBoxOk)
	}
	if len(paineis) != 1 {
		t.Fatalf("veio %d painel de texto (%#x), queria exatamente 1. Pacotes: %v",
			len(paineis), protocol.MsgMessagePanel, tiposDe(lidos))
	}
	quer := protocol.ClientText(esperado)
	if len(paineis[0].corpo) < len(quer) || string(paineis[0].corpo[:len(quer)]) != string(quer) {
		t.Errorf("a recusa disse %q, queria %q", decodePanel(paineis[0].corpo), esperado)
	}
}

// A CONTA BLOQUEADA ERA A QUE NUNCA FECHAVA. Este é o teste do defeito.
func TestARecusaDeContaBloqueadaFechaOSocket(t *testing.T) {
	addr, stop := servidorTrancadoComPrazo(t, newDB(), false, 200*time.Millisecond)
	defer stop()
	c := dial(t, addr)
	defer func() { _ = c.Close() }()

	send(t, c, protocol.MsgAccountLogin, loginBody("banned", "x", protocol.AppVersion))
	exigeUmSoPainel(t, pacotesAteFechar(t, c, 5*time.Second), msgLoginBloqueada)
}

// A VERSÃO VELHA: o mesmo, no caminho que roda ANTES da checagem de modo — lá a sessão
// pode estar em qualquer estado, inclusive já logada, se um cliente remendado mandar um
// login com a versão errada depois de entrar.
func TestARecusaDeVersaoFechaOSocket(t *testing.T) {
	addr, stop := servidorTrancadoComPrazo(t, newDB(), false, 200*time.Millisecond)
	defer stop()
	c := dial(t, addr)
	defer func() { _ = c.Close() }()

	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", 1234))
	exigeUmSoPainel(t, pacotesAteFechar(t, c, 5*time.Second), msgLoginVersao)
}

// O ERRO DE BANCO: o caminho que não tinha frase NENHUMA, nem no Language.txt nem no
// noticeText. Agora tem frase e fecha.
func TestOErroDeBancoFechaOSocket(t *testing.T) {
	addr, stop := servidorTrancadoComPrazo(t, bancoQueFalha{}, false, 200*time.Millisecond)
	defer stop()
	c := dial(t, addr)
	defer func() { _ = c.Close() }()

	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
	exigeUmSoPainel(t, pacotesAteFechar(t, c, 5*time.Second), msgLoginErroDeBanco)
}

// O TETO: da terceira recusa em diante o socket cai NA HORA.
//
// Sem teto, um cliente remendado repete a recusa e mantém o socket de pé de graça. Com o
// acesso restrito ligado a senha está CERTA — a recusa é sobre ONDE a pessoa está
// entrando —, então dá para repetir à vontade no mesmo socket: é por aí que o abuso passa.
//
// O PRAZO AQUI É LONGO DE PROPÓSITO e o teste espera menos que ele: se a terceira recusa
// fechasse pelo prazo em vez de na hora, a leitura estouraria. Um prazo curto faria este
// teste passar sem o teto existir — seria o instrumento medindo a si mesmo.
func TestOTerceiroFechamentoNaoEsperaOPrazo(t *testing.T) {
	const prazo = 5 * time.Second
	addr, stop := servidorTrancadoComPrazo(t, newDB(), true, prazo)
	defer stop()
	c := dial(t, addr)
	defer func() { _ = c.Close() }()

	// As duas primeiras: texto, e o socket segue de pé.
	for i := 1; i <= limiteDeRecusasNoSocket-1; i++ {
		send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
		if texto := leTextoAtePainel(t, c); !strings.Contains(texto, "acesso restrito") {
			t.Fatalf("recusa %d disse %q, queria a do acesso restrito", i, texto)
		}
	}
	comeco := time.Now()
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
	lidos := pacotesAteFechar(t, c, prazo-time.Second)
	if levou := time.Since(comeco); levou >= prazo {
		t.Errorf("a terceira recusa levou %v para fechar, o prazo e %v: ela esperou o "+
			"prazo, o teto nao pegou", levou, prazo)
	}
	// E o teto não pode comer o texto: a pessoa ainda tem de saber por que caiu.
	if len(lidos) == 0 {
		t.Error("a terceira recusa fechou sem dizer nada")
	}
}

// UM AGENDAMENTO SÓ POR SOCKET: o prazo conta da PRIMEIRA recusa, e a segunda não estica
// a vida da conexão.
//
// ERA ISSO QUE O CÓDIGO ANTIGO FAZIA. Cada recusa agendava a sua espera, e a guarda de
// então comparava o contador de recusas: a espera da PRIMEIRA acordava, via que havia uma
// recusa mais nova e desistia, e quem fechava era sempre a ÚLTIMA. Duas recusas, e o
// socket vivia um prazo inteiro depois da segunda. Repetindo a recusa, uma pessoa esticava
// a conexão para sempre — e cada repetição deixava mais uma goroutine dormindo.
//
// A PRIMEIRA VERSÃO DESTE TESTE NÃO PROVAVA NADA, e descobri isso plantando o defeito de
// volta: eu tirava só a marca de "já tem um agendado" e deixava a guarda nova, e então o
// fechamento da PRIMEIRA recusa acontecia igual — a segunda espera acordava num socket já
// morto e o World.Go a descartava. O defeito de verdade é a marca MAIS a guarda pelo
// contador, e o limite de tempo tinha de separar 1 prazo de 1,5 prazo, o que a folga de
// então não fazia. Com o prazo em 2 s a separação é de 500 ms de cada lado.
func TestASegundaRecusaNaoEsticaAVidaDoSocket(t *testing.T) {
	// O prazo é longo de propósito: o que se mede é a DIFERENÇA entre fechar em 1 prazo
	// (certo) e em 1,5 prazo (defeito), e ela tem de ser maior que a lentidão do CI.
	const prazo = 2 * time.Second
	const limite = prazo + prazo/4 // 2,5 s: meio caminho entre 2 s e 3 s
	addr, stop := servidorTrancadoComPrazo(t, newDB(), true, prazo)
	defer stop()
	c := dial(t, addr)
	defer func() { _ = c.Close() }()

	comeco := time.Now()
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
	leTextoAtePainel(t, c)
	// A segunda entra na METADE do prazo da primeira.
	time.Sleep(prazo / 2)
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
	leTextoAtePainel(t, c)
	pacotesAteFechar(t, c, 2*prazo)
	if levou := time.Since(comeco); levou >= limite {
		t.Errorf("o socket viveu %v, mais que o limite de %v (o prazo e %v): a segunda "+
			"recusa reagendou o fechamento e esticou a conexao", levou, limite, prazo)
	}
}

// bancoQueFalha devolve erro no login, que é o caminho do NoticeDBError.
type bancoQueFalha struct{ world.NopPersistence }

func (bancoQueFalha) AccountLogin(context.Context, string, string, int64) (world.LoginOutcome, error) {
	return world.LoginOutcome{}, errors.New("banco fora do ar")
}
