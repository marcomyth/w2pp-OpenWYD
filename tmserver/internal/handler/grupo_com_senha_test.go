package handler

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/acesso"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// ===========================================================================
// Testes de unidade: a régua da senha, a comparação e o corte dos argumentos.
// ===========================================================================

func TestSenhaDeGrupoValida(t *testing.T) {
	t.Parallel()
	casos := []struct {
		senha string
		quer  bool
		por   string
	}{
		{"abcd", true, "quatro e o minimo"},
		{"123456789012", true, "doze e o maximo"},
		{"Senha123", true, "letras e numeros, maiuscula serve"},
		{"abc", false, "tres e curta"},
		{"1234567890123", false, "treze e longa"},
		{"", false, "vazia"},
		{"abc def", false, "espaco quebraria a separacao dos argumentos"},
		{"senha!", false, "pontuacao nao"},
		{"senhá", false, "acento nao: o cliente manda cp1252 e duas maquinas podem divergir"},
		{"senha\x00", false, "byte nulo nao"},
	}
	for _, c := range casos {
		if got := senhaDeGrupoValida(c.senha); got != c.quer {
			t.Errorf("senhaDeGrupoValida(%q) = %v, queria %v (%s)", c.senha, got, c.quer, c.por)
		}
	}
}

func TestSenhaBate(t *testing.T) {
	t.Parallel()
	if !senhaBate("abcd1234", "abcd1234") {
		t.Error("a senha igual tinha de bater")
	}
	casos := []string{"abcd1233", "abcd123", "abcd12345", "", "ABCD1234"}
	for _, tentada := range casos {
		if senhaBate("abcd1234", tentada) {
			t.Errorf("senhaBate(guardada, %q) bateu e nao devia", tentada)
		}
	}
}

// TestArgumentosDoComandoCortaOsNulos guarda o detalhe que faria a senha nunca bater.
//
// O argumento chega como o campo de texto do sussurro, de tamanho fixo e cheio de
// zeros depois do que a pessoa digitou. Sem cortar no primeiro zero, a senha viria com
// uma cauda de bytes nulos grudada.
func TestArgumentosDoComandoCortaOsNulos(t *testing.T) {
	t.Parallel()
	cru := make([]byte, 64)
	copy(cru, "HeroB abcd1234")
	campos := argumentosDoComando(cru)
	if len(campos) != 2 || campos[0] != "HeroB" || campos[1] != "abcd1234" {
		t.Fatalf("campos = %q, queria [HeroB abcd1234]", campos)
	}
	if len(argumentosDoComando(make([]byte, 64))) != 0 {
		t.Error("campo todo nulo tinha de dar zero argumentos")
	}
}

// TestLimiteDeTentativasDeEntrar: cinco passam, a sexta nao, e um minuto depois
// volta a passar.
func TestLimiteDeTentativasDeEntrar(t *testing.T) {
	t.Parallel()
	d := &Dispatcher{tentativasDeGrupo: make(map[int]tentativasDeGrupo)}
	const conn = 7
	var agora uint32 = 1000

	for i := 1; i <= maxTentativasDeEntrar; i++ {
		if !d.podeTentarEntrarNoGrupo(agora, conn) {
			t.Fatalf("tentativa %d foi recusada e devia passar", i)
		}
	}
	if d.podeTentarEntrarNoGrupo(agora, conn) {
		t.Fatal("a sexta tentativa passou, e o limite e cinco")
	}
	// Dentro da janela continua fechado, mesmo andando o relógio um pouco.
	if d.podeTentarEntrarNoGrupo(agora+janelaDeTentativas-1, conn) {
		t.Fatal("ainda dentro da janela e passou")
	}
	// A janela é por personagem: outro conn não herda o castigo.
	if !d.podeTentarEntrarNoGrupo(agora, conn+1) {
		t.Fatal("o limite vazou para outro personagem")
	}
	if !d.podeTentarEntrarNoGrupo(agora+janelaDeTentativas, conn) {
		t.Fatal("passado um minuto, tinha de liberar")
	}
}

// TestEsqueceGrupoComSenhaLimpaOsDois: a limpeza que impede o pior caso aqui — um
// conn reciclado herdar a senha do jogador anterior.
func TestEsqueceGrupoComSenhaLimpaOsDois(t *testing.T) {
	t.Parallel()
	d := &Dispatcher{
		senhasDeGrupo:     map[int]string{4: "abcd1234"},
		tentativasDeGrupo: map[int]tentativasDeGrupo{4: {desde: 1, quantas: 3}},
	}
	d.esqueceGrupoComSenha(4)
	if _, ok := d.senhasDeGrupo[4]; ok {
		t.Error("a senha ficou")
	}
	if _, ok := d.tentativasDeGrupo[4]; ok {
		t.Error("a contagem ficou")
	}
}

// TestFrasesDoGrupoCabemNoPainel mede os BYTES em cp1252, que é o que vai no fio.
//
// MEDIDO E NÃO OLHADO: o painel corta em 94 bytes sem avisar, e as frases têm acento,
// então contar caracteres em Go daria outro número. Este teste passa pelo mesmo
// conversor que o servidor usa.
func TestFrasesDoGrupoCabemNoPainel(t *testing.T) {
	t.Parallel()
	const limite = 94 // protocol.messagePanelTextMax
	for _, frase := range frasesDoGrupoComSenha {
		if frase == "" {
			t.Error("frase vazia na lista")
			continue
		}
		noFio := protocol.ClientText(frase)
		if len(noFio) > limite {
			t.Errorf("%d bytes (limite %d): %q", len(noFio), limite, frase)
			continue
		}
		// PELO ENCODER DE VERDADE, e comparando os BYTES no fio: a frase tem acento,
		// e em cp1252 esses bytes não são UTF-8 válido, então voltar para string aqui
		// compararia lixo com lixo. O que importa é que nada foi cortado.
		corpo := protocol.EncodeMessagePanelBody(frase)
		if !bytes.Equal(corpo[:len(noFio)], noFio) {
			t.Errorf("o painel cortou ou mudou a frase: %q", frase)
		}
		if corpo[len(noFio)] != 0 {
			t.Errorf("a frase encostou no fim do campo sem terminador: %q", frase)
		}
	}
}

// TestListaDeFrasesEstaCompleta: uma frase nova que não entre na lista não é medida,
// e o teste de tamanho passaria sem olhar para ela.
func TestListaDeFrasesEstaCompleta(t *testing.T) {
	t.Parallel()
	const quantasConstantes = 21
	if len(frasesDoGrupoComSenha) != quantasConstantes {
		t.Fatalf("a lista tem %d frases e o bloco de constantes tem %d; "+
			"toda frase nova entra na lista, senão ninguém mede o tamanho dela",
			len(frasesDoGrupoComSenha), quantasConstantes)
	}
	vistas := map[string]bool{}
	for _, f := range frasesDoGrupoComSenha {
		if vistas[f] {
			t.Errorf("frase repetida na lista: %q", f)
		}
		vistas[f] = true
	}
}

// ===========================================================================
// Testes de ponta a ponta: os três comandos contra um servidor de verdade.
// ===========================================================================

// grupoCmd manda um comando de barra, que o cliente envia como sussurro para o nome
// do comando (o mesmo caminho do /create e dos teleportes).
func grupoCmd(t *testing.T, c net.Conn, cmd, argumentos string) {
	t.Helper()
	var body protocol.MsgWhisperBody
	copy(body.MobName[:], cmd)
	// String é um slice e não um vetor de tamanho fixo: um copy aqui não escreveria
	// nada, e o servidor receberia o comando sem argumento nenhum.
	body.String = []byte(argumentos)
	send(t, c, protocol.MsgMessageWhisper, body.Encode())
}

// painelDiz exige que o cliente receba esta frase no painel de texto.
//
// COMPARA OS BYTES DO FIO, e não a string: o painel viaja em cp1252, então os bytes
// que chegam NÃO são o texto UTF-8 da constante. Comparar string com string aqui
// falharia em toda frase com acento — e o jeito errado de "consertar" isso seria
// tirar os acentos das frases, ou seja, piorar o português do jogo para o teste
// passar.
//
// PROCURA EM VEZ DE EXIGIR A PRIMEIRA: os pacotes de grupo e os avisos para o líder
// chegam misturados, e um teste que exigisse a ordem exata quebraria a cada pacote
// novo que alguém passe a mandar, sem nada de errado ter acontecido.
func painelDiz(t *testing.T, c net.Conn, esperada string) {
	t.Helper()
	quer := protocol.ClientText(esperada)
	var vistas []string
	for i := 0; i < 12; i++ {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty != protocol.MsgMessagePanel {
			continue
		}
		texto := recortaAteNulo(payload)
		if bytes.Equal(texto, quer) {
			return
		}
		vistas = append(vistas, string(texto))
	}
	t.Fatalf("o painel nao disse %q; disse %q", esperada, vistas)
}

// drenaPainel consome a próxima frase do painel sem olhar qual é, para o passo
// seguinte do teste começar com a fila limpa.
func drenaPainel(t *testing.T, c net.Conn) {
	t.Helper()
	for i := 0; i < 12; i++ {
		ty, _, ok := readMaybe(t, c)
		if !ok {
			return
		}
		if ty == protocol.MsgMessagePanel {
			return
		}
	}
}

// recortaAteNulo corta no primeiro zero, sem passar por string: o conteúdo é
// cp1252 e converter para string aqui não ajudaria em nada.
func recortaAteNulo(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return b[:i]
	}
	return b
}

func TestGrupoComSenhaCriarEEntrar(t *testing.T) {
	addr, stop, _ := startServerClock(t, partyDB())
	defer stop()
	lider := enterWorldAs(t, addr, "tester") // conn 1, Hero
	defer lider.Close()
	membro := enterWorldAs(t, addr, "tradeb") // conn 2, HeroB
	defer membro.Close()

	grupoCmd(t, lider, "/criargrupo", "abcd1234")
	painelDiz(t, lider, msgGrupoCriado)

	// A senha errada é recusada, e a frase diz o motivo.
	grupoCmd(t, membro, "/entrar", "Hero errada99")
	painelDiz(t, membro, msgGrupoSenhaErrada)

	// A certa entra, e o cliente recebe o pacote de grupo.
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	painelDiz(t, membro, msgGrupoEntrou)
}

// TestGrupoComSenhaQualquerMembroServeDeEndereco: quem entra conhece o amigo, não
// necessariamente o líder.
func TestGrupoComSenhaQualquerMembroServeDeEndereco(t *testing.T) {
	addr, stop, _ := startServerClock(t, partyDB())
	defer stop()
	lider := enterWorldAs(t, addr, "tester")
	defer lider.Close()
	membro := enterWorldAs(t, addr, "tradeb")
	defer membro.Close()
	terceiro := enterWorldAs(t, addr, "third") // conn 3, HeroC
	defer terceiro.Close()

	grupoCmd(t, lider, "/criargrupo", "abcd1234")
	drenaPainel(t, lider)
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	drenaPainel(t, membro)

	// Agora o terceiro entra citando HeroB, que é MEMBRO e não líder.
	grupoCmd(t, terceiro, "/entrar", "HeroB abcd1234")
	painelDiz(t, terceiro, msgGrupoEntrou)
}

func TestGrupoComSenhaRecusas(t *testing.T) {
	addr, stop, _ := startServerClock(t, partyDB())
	defer stop()
	lider := enterWorldAs(t, addr, "tester")
	defer lider.Close()
	membro := enterWorldAs(t, addr, "tradeb")
	defer membro.Close()

	// Senha fora da régua na criação.
	grupoCmd(t, lider, "/criargrupo", "abc")
	painelDiz(t, lider, msgGrupoSenhaInvalida)
	// Sem argumento: a frase ensina a usar.
	grupoCmd(t, lider, "/criargrupo", "")
	painelDiz(t, lider, msgGrupoComoUsarCriar)
	// Nome que não existe.
	grupoCmd(t, membro, "/entrar", "NinguemAqui abcd1234")
	painelDiz(t, membro, msgGrupoNomeNaoEncontrado)
	// Grupo sem senha.
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	painelDiz(t, membro, msgGrupoNaoTemSenha)
	// Você mesmo.
	grupoCmd(t, lider, "/criargrupo", "abcd1234")
	drenaPainel(t, lider)
	grupoCmd(t, lider, "/entrar", "Hero abcd1234")
	painelDiz(t, lider, msgGrupoVoceMesmo)
	// Já está em grupo: o membro entra e tenta de novo.
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	painelDiz(t, membro, msgGrupoEntrou)
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	painelDiz(t, membro, msgGrupoVoceJaEstaEmGrupo)
	// Membro não pode criar grupo com senha: tem de sair primeiro.
	grupoCmd(t, membro, "/criargrupo", "outra123")
	painelDiz(t, membro, msgGrupoVoceNaoEOLider)
}

// TestGrupoComSenhaLimitePeloChat: a sexta tentativa é recusada pelo limite, e a
// frase é a do limite e não a da senha — a diferença é o que impede o atacante de
// saber se errou a senha ou bateu no limite.
func TestGrupoComSenhaLimitePeloChat(t *testing.T) {
	addr, stop, clock := startServerClock(t, partyDB())
	defer stop()
	lider := enterWorldAs(t, addr, "tester")
	defer lider.Close()
	membro := enterWorldAs(t, addr, "tradeb")
	defer membro.Close()

	grupoCmd(t, lider, "/criargrupo", "abcd1234")
	drenaPainel(t, lider)

	// As cinco primeiras são recusadas pela SENHA, e é isso que prova que o limite
	// ainda não entrou em cena.
	for range maxTentativasDeEntrar {
		grupoCmd(t, membro, "/entrar", "Hero errada99")
		painelDiz(t, membro, msgGrupoSenhaErrada)
	}
	grupoCmd(t, membro, "/entrar", "Hero errada99")
	painelDiz(t, membro, msgGrupoMuitasTentativas)
	// E a senha CERTA também é recusada enquanto o limite vale: parar antes de
	// comparar é o que tira a informação de quem está tentando adivinhar.
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	painelDiz(t, membro, msgGrupoMuitasTentativas)
	// Um minuto depois, libera.
	clock.Store(clock.Load() + janelaDeTentativas)
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	painelDiz(t, membro, msgGrupoEntrou)
}

// TestGrupoComSenhaCheio: com o grupo lotado a recusa diz que está cheio.
func TestGrupoComSenhaCheio(t *testing.T) {
	addr, stop, _, mundo := servidorDeGrupo(t, partyDB())
	defer stop()
	lider := enterWorldAs(t, addr, "tester")
	defer lider.Close()
	grupoCmd(t, lider, "/criargrupo", "abcd1234")
	drenaPainel(t, lider)

	// Lota a lista do líder direto na Entity: entrar doze jogadores de verdade
	// levaria doze logins, e o que este teste mede é a recusa e não o login.
	encheGrupoDoLider(t, mundo, 1)

	membro := enterWorldAs(t, addr, "tradeb")
	defer membro.Close()
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	painelDiz(t, membro, msgGrupoCheio)
}

func TestGrupoComSenhaTransLiderLevaASenha(t *testing.T) {
	addr, stop, _ := startServerClock(t, partyDB())
	defer stop()
	lider := enterWorldAs(t, addr, "tester")
	defer lider.Close()
	membro := enterWorldAs(t, addr, "tradeb")
	defer membro.Close()
	terceiro := enterWorldAs(t, addr, "third")
	defer terceiro.Close()

	grupoCmd(t, lider, "/criargrupo", "abcd1234")
	drenaPainel(t, lider)
	grupoCmd(t, membro, "/entrar", "Hero abcd1234")
	drenaPainel(t, membro)

	grupoCmd(t, lider, "/translider", "HeroB")
	painelDiz(t, lider, msgGrupoLiderancaPassada)

	// A SENHA FOI COM A LIDERANÇA: o terceiro entra citando o novo líder, com a
	// mesma senha de antes.
	grupoCmd(t, terceiro, "/entrar", "HeroB abcd1234")
	painelDiz(t, terceiro, msgGrupoEntrou)
	// E o líder antigo deixou de ser endereço de senha: ele agora é membro.
	grupoCmd(t, lider, "/criargrupo", "outra123")
	painelDiz(t, lider, msgGrupoVoceNaoEOLider)
}

func TestGrupoComSenhaTransLiderRecusas(t *testing.T) {
	addr, stop, _, mundo := servidorDeGrupo(t, partyDB())
	defer stop()
	lider := enterWorldAs(t, addr, "tester")
	defer lider.Close()
	fora := enterWorldAs(t, addr, "tradeb")
	defer fora.Close()

	// Sem grupo nenhum.
	grupoCmd(t, lider, "/translider", "HeroB")
	painelDiz(t, lider, msgGrupoSemMembros)
	// Com grupo, mas para quem não é membro.
	grupoCmd(t, lider, "/criargrupo", "abcd1234")
	drenaPainel(t, lider)
	encheGrupoDoLider(t, mundo, 1)
	grupoCmd(t, lider, "/translider", "HeroB")
	painelDiz(t, lider, msgGrupoNaoEMembro)
}

// TestGrupoComSenhaASenhaNaoVaiParaLogNenhum é o teste que guarda a promessa.
//
// O LOG É CAPTURADO DE VERDADE, e não inspecionado no olho: a garantia é "a senha não
// sai em log nenhum", e a única forma de sustentar isso é ler tudo o que o servidor
// escreveu e procurar a senha lá dentro. Um teste que só olhasse a linha "chat command"
// deixaria passar uma linha nova escrita amanhã por outra pessoa.
func TestGrupoComSenhaASenhaNaoVaiParaLogNenhum(t *testing.T) {
	const senha = "zZq7Kx2pLw"

	var buf bytes.Buffer
	addr, stop := servidorComLogCapturado(t, partyDB(), &buf)
	defer stop()
	lider := enterWorldAs(t, addr, "tester")
	defer lider.Close()
	membro := enterWorldAs(t, addr, "tradeb")
	defer membro.Close()

	grupoCmd(t, lider, "/criargrupo", senha)
	drenaPainel(t, lider)
	grupoCmd(t, membro, "/entrar", "Hero "+senha)
	drenaPainel(t, membro)
	grupoCmd(t, membro, "/entrar", "Hero errou"+senha)
	drenaPainel(t, membro)
	grupoCmd(t, lider, "/translider", "HeroB")
	drenaPainel(t, lider)

	escrito := buf.String()
	if strings.Contains(escrito, senha) {
		t.Fatalf("A SENHA APARECEU NO LOG. Log:\n%s", escrito)
	}
	// E a prova de que o log estava ligado e vendo os comandos: sem isto, um logger
	// que não escreveu nada passaria o teste sem provar nada.
	if !strings.Contains(escrito, "criargrupo") {
		t.Fatalf("o log nao registrou o comando, entao este teste nao provou nada. Log:\n%s", escrito)
	}
}

// ===========================================================================
// Apoio dos testes de ponta a ponta.
// ===========================================================================

// servidorComLogCapturado é o startServerClock com o log indo para um buffer.
func servidorComLogCapturado(t *testing.T, persist world.Persistence, buf io.Writer) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	// Debug para pegar TUDO: uma senha que vazasse só numa linha de depuração
	// continuaria sendo uma senha vazada no arquivo de log de produção.
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) },
		CombatRules: regraSemEscala(), RMT: acesso.RMTAberto})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, persist, d.Handle)
	w.SetSessionEndHandler(d.SessionEnd)
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
	}
}

// servidorDeGrupo é o startServerClock que também devolve o mundo, para os testes
// que precisam lotar um grupo sem fazer doze logins.
func servidorDeGrupo(t *testing.T, persist world.Persistence) (string, func(), *atomic.Uint32, *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) },
		CombatRules: regraSemEscala(), RMT: acesso.RMTAberto})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, persist, d.Handle)
	w.SetSessionEndHandler(d.SessionEnd)
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
	}, clock, w
}

// encheGrupoDoLider ocupa as vagas do grupo do líder com conns falsos.
//
// CONNS FALSOS DE PROPÓSITO: o que estes testes medem é a recusa por lotação e a
// recusa de quem não é membro, e doze logins de verdade só trariam doze vezes mais
// tempo de teste e nenhuma cobertura nova.
// DENTRO DO LAÇO, como todo acesso a estado de mundo neste servidor.
func encheGrupoDoLider(t *testing.T, w *world.World, liderConn int) {
	t.Helper()
	noLacoDoMundo(t, w, func(w *world.World) {
		le := w.Entity(liderConn)
		if le == nil {
			t.Fatal("o lider nao esta no mundo")
		}
		for i := range le.PartyList {
			if le.PartyList[i] == 0 {
				le.PartyList[i] = 900 + i // conn que nao existe
			}
		}
	})
}
