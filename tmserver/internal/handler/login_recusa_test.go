package handler

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// O QUE ESTES TESTES GUARDAM: que toda recusa de login chegue ao jogador como TEXTO, e
// que nenhuma mande a caixa de mensagem quebrada.
//
// O DEFEITO: o notify manda primeiro um 0x0102 MsgMessageBoxOk de quatro bytes com um
// código do NOSSO iota, e o legado nunca manda 0x0102 no login — lá a MsgMessageBoxOk
// tem 116 bytes de corpo. O cliente recebia uma caixa de tamanho errado com um número
// que não significa nada para ele, e a pessoa via a tela muda: o jogo recusava e não
// dizia por quê.
//
// O único caminho que funcionava era o do acesso restrito, que manda só o 0x0101. Foi
// ele que mostrou onde estava o erro, e é o que as sete recusas passam a fazer.

// TestNenhumaRecusaDeLoginMandaACaixaQuebrada é a prova pela AUSÊNCIA do pacote.
//
// LÊ O FONTE, e digo por quê: subir o servidor e forçar cada uma das sete recusas exige
// sete montagens diferentes (versão errada, três senhas erradas, banco caído, conta
// bloqueada...), e o que se quer provar é uma coisa só e simples — que ninguém no login
// chama o notify. Um teste tosco que roda em milissegundos e pega a regressão vale mais
// que um elegante que ninguém escreve.
func TestNenhumaRecusaDeLoginMandaACaixaQuebrada(t *testing.T) {
	t.Parallel()
	fonte := leFonteDoLogin(t)
	// A REGRA É SOBRE QUEM ESTÁ ENTRANDO, e não sobre o arquivo inteiro.
	//
	// Sobra um notify no login.go de propósito: o que avisa a sessão ANTIGA quando outro
	// login toma a conta dela. Aquela pessoa está EM CENA — em UserPlay ou UserSelChar —
	// e lá a caixa do cliente se comporta de outro jeito. O defeito era com quem ainda
	// está na tela de login, e é só isso que este teste tranca.
	if contaOcorrencias(fonte, "d.notify(w, s,") != 0 {
		t.Error("o login voltou a chamar d.notify na sessao de quem esta entrando: isso manda " +
			"o 0x0102 de quatro bytes, que o cliente nao sabe ler, e a recusa volta a ser muda")
	}
	// E a prova positiva: as sete recusas usam o caminho de texto.
	//
	// Duas portas, e o que importa e a SOMA: o recusaEFecha e a recusa que termina a
	// conexao (versao, erro de banco, conta bloqueada) e chama o recusaLogin por dentro;
	// as outras quatro deixam a pessoa tentar de novo e chamam o recusaLogin direto.
	comFecho := contaOcorrencias(fonte, "d.recusaEFecha(")
	soTexto := contaOcorrencias(fonte, "d.recusaLogin(")
	if comFecho+soTexto != 7 {
		t.Errorf("achei %d recusaEFecha + %d recusaLogin = %d, esperava 7 -- se uma recusa "+
			"ficou de fora, ela e a que vai aparecer muda para o jogador",
			comFecho, soTexto, comFecho+soTexto)
	}
	// E NENHUM caminho pode agendar o fechamento a mao. Era isso que deixava o socket da
	// conta bloqueada aberto para sempre: quem chama o fechaDepois direto nao devolve a
	// sessao ao estado de antes do login, e a guarda do fechaDepois entao recusa fechar.
	// O fechaPorRecusa e quem devolve, e e por onde todo fechamento passa.
	if n := contaOcorrencias(fonte, "d.fechaDepois("); n != 0 {
		t.Errorf("o login chama d.fechaDepois direto %d vezes: o fechamento tem de passar "+
			"pelo recusaEFecha ou pelo fechaPorRecusa", n)
	}
}

// TestAsFrasesDeReservaDoLoginCabemNoPainel mede os BYTES em cp1252.
//
// MEDIDO E NÃO OLHADO: o painel corta em 94 bytes sem avisar, e as frases têm acento, o
// que faz contar caracteres em Go dar outro número. Uma frase cortada no meio é pior que
// uma frase curta — e aqui ela é a última coisa que o jogador lê antes de o jogo fechar.
func TestAsFrasesDeReservaDoLoginCabemNoPainel(t *testing.T) {
	t.Parallel()
	const limite = 94
	frases := map[string]string{
		"versao":        msgLoginVersao,
		"aguarde":       msgLoginAguarde,
		"tres erros":    msgLoginTresErros,
		"senha":         msgLoginSenha,
		"sem conta":     msgLoginSemConta,
		"bloqueada":     msgLoginBloqueada,
		"erro de banco": msgLoginErroDeBanco,
	}
	if len(frases) != 7 {
		t.Fatalf("sao %d frases e as recusas sao 7", len(frases))
	}
	for nome, f := range frases {
		if f == "" {
			t.Errorf("%s: frase vazia, e uma reserva vazia nao manda pacote nenhum", nome)
			continue
		}
		noFio := protocol.ClientText(f)
		if len(noFio) > limite {
			t.Errorf("%s: %d bytes no fio, limite %d: %q", nome, len(noFio), limite, f)
		}
	}
}

// TestOErroDeBancoTemFrase guarda o caso que não tinha nenhuma.
//
// O NoticeDBError não estava no Language.txt nem no noticeText: quem caía nele via a tela
// muda mesmo depois de o 0x0101 passar a sair, porque o texto era vazio e um
// sendClientMessage vazio não manda nada. A reserva é o que fecha isso.
func TestOErroDeBancoTemFrase(t *testing.T) {
	t.Parallel()
	if msgLoginErroDeBanco == "" {
		t.Fatal("o erro de banco ficou sem frase, e e justamente o que nao tinha")
	}
	// E ela não pode mandar a pessoa conferir a senha: o erro não foi dela.
	for _, proibido := range []string{"senha", "Senha"} {
		if contaOcorrencias(msgLoginErroDeBanco, proibido) != 0 {
			t.Errorf("a frase do erro de banco fala de %q, e o erro nao foi do jogador: %q",
				proibido, msgLoginErroDeBanco)
		}
	}
}

func leFonteDoLogin(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("login.go")
	if err != nil {
		t.Fatalf("lendo o login.go: %v", err)
	}
	// Tira os comentários: um d.notify MENCIONADO num comentário não manda pacote, e
	// este arquivo fala bastante do notify justamente para explicar por que não o usa.
	return comentarioDeLinhaNoLogin.ReplaceAllString(string(b), "")
}

var comentarioDeLinhaNoLogin = regexp.MustCompile(`(?m)^\s*//.*$`)

func contaOcorrencias(s, sub string) int { return strings.Count(s, sub) }
