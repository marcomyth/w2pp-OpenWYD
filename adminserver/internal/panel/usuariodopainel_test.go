package panel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"

	"github.com/jeanluca/w2pp-openwyd/internal/secret"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// fakePainel é o store dos usuários do painel, do jeito que estes testes precisam.
type fakePainel struct {
	// porLogin é o que existe. A chave é o login já em minúsculas, como o banco grava.
	porLogin map[string]store.AutenticacaoDoPainel
	erroLer  error

	// Observado.
	criados  []string
	ativados map[int64]bool
	senhas   []int64
	erroCria error
	erroAtiv error
}

func novoFakePainel() *fakePainel {
	return &fakePainel{
		porLogin: map[string]store.AutenticacaoDoPainel{},
		ativados: map[int64]bool{},
	}
}

// comUsuario põe um usuário com senha de verdade, cifrada pelo mesmo argon2id do sistema.
//
// SENHA CIFRADA DE VERDADE e não um hash de mentira: o que está sendo testado é o caminho
// que verifica senha, e um fake que devolve "confere" para qualquer coisa provaria que o
// teste roda, não que a senha importa.
func (f *fakePainel) comUsuario(t *testing.T, id int64, login, senha, papel string, ativo bool) {
	t.Helper()
	h, err := secret.HashSecret(senha)
	if err != nil {
		t.Fatal(err)
	}
	f.porLogin[login] = store.AutenticacaoDoPainel{
		ID: id, Login: login, Papel: papel, Ativo: ativo, Hash: h,
	}
}

func (f *fakePainel) UsuarioDoPainelPorLogin(_ context.Context, login string) (store.AutenticacaoDoPainel, error) {
	if f.erroLer != nil {
		return store.AutenticacaoDoPainel{}, f.erroLer
	}
	a, ok := f.porLogin[login]
	if !ok {
		return store.AutenticacaoDoPainel{}, store.ErrNotFound
	}
	return a, nil
}

func (f *fakePainel) ExisteUsuarioDoPainel(_ context.Context, login string) (bool, error) {
	if f.erroLer != nil {
		return false, f.erroLer
	}
	_, ok := f.porLogin[login]
	return ok, nil
}

func (f *fakePainel) ListarUsuariosDoPainel(context.Context) ([]store.UsuarioDoPainel, error) {
	var out []store.UsuarioDoPainel
	for _, a := range f.porLogin {
		out = append(out, store.UsuarioDoPainel{
			ID: a.ID, Login: a.Login, Papel: a.Papel, Ativo: a.Ativo,
		})
	}
	return out, nil
}

func (f *fakePainel) ContarUsuariosDoPainel(context.Context) (int, int, error) {
	total, admins := 0, 0
	for _, a := range f.porLogin {
		total++
		if a.Ativo && a.Papel == roleAdmin {
			admins++
		}
	}
	return total, admins, nil
}

func (f *fakePainel) CriarUsuarioDoPainel(_ context.Context, login, _, papel string, _ *int64) (store.UsuarioDoPainel, error) {
	if f.erroCria != nil {
		return store.UsuarioDoPainel{}, f.erroCria
	}
	f.criados = append(f.criados, login)
	return store.UsuarioDoPainel{ID: 99, Login: login, Papel: papel, Ativo: true}, nil
}

func (f *fakePainel) DefinirAtivoDoPainel(_ context.Context, id int64, ativo bool) error {
	if f.erroAtiv != nil {
		return f.erroAtiv
	}
	f.ativados[id] = ativo
	return nil
}

func (f *fakePainel) TrocarSenhaDoPainel(_ context.Context, id int64, _ string) error {
	f.senhas = append(f.senhas, id)
	return nil
}

// O USUÁRIO DO PAINEL ENTRA, e a sessão sabe que ele não é conta de jogo.
func TestUsuarioDoPainelEntra(t *testing.T) {
	f := novoFakePainel()
	f.comUsuario(t, 7, "hanna", "uma-senha-bem-longa", roleAdmin, true)
	h := painelComUsuarios(t, f, false)

	res := postLogin(h, "Hanna", "uma-senha-bem-longa")

	if res.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d, queria 303; corpo=%s", res.Code, res.Body.String())
	}
	// O login foi digitado com H maiúsculo de propósito: a tela normaliza, e se a gravação
	// e a busca discordassem no caso, o usuário nunca conseguiria entrar.
	if res.Header().Get("Location") != "/" {
		t.Errorf("destino = %q, queria /", res.Header().Get("Location"))
	}
}

// SEM SEGUNDA CHANCE: senha errada de usuário do painel NÃO tenta a conta de jogo.
//
// É o coração desta separação. Sem a trava, um usuário de painel que errasse a senha cairia
// no caminho antigo e poderia entrar como a CONTA DE JOGO homônima — duas pessoas
// diferentes, o mesmo nome no painel, permissões diferentes, e cada uma achando que é a
// dona daquele nome.
func TestSenhaErradaDoPainelNaoTentaAContaDeJogo(t *testing.T) {
	f := novoFakePainel()
	f.comUsuario(t, 7, "hanna", "a-senha-certa-longa", roleAdmin, true)
	// A conta de jogo "hanna" existe, é admin e a senha dela é outra. Se houvesse segunda
	// chance, esta senha entraria.
	contas := contaDeJogo(t, "hanna", "senha-da-conta-de-jogo", roleAdmin)
	h := painelComUsuariosEContas(t, f, contas, false)

	res := postLogin(h, "hanna", "senha-da-conta-de-jogo")

	if res.Code == http.StatusSeeOther && res.Header().Get("Location") == "/" {
		t.Fatal("entrou pela conta de jogo com o nome de um usuario do painel: a segunda chance existe")
	}
}

// USUÁRIO DESATIVADO NÃO ENTRA, mesmo com a senha certa.
func TestUsuarioDesativadoNaoEntra(t *testing.T) {
	f := novoFakePainel()
	f.comUsuario(t, 7, "antiga", "uma-senha-bem-longa", roleAdmin, false)
	h := painelComUsuarios(t, f, false)

	res := postLogin(h, "antiga", "uma-senha-bem-longa")

	if res.Code == http.StatusSeeOther && res.Header().Get("Location") == "/" {
		t.Error("usuario desativado entrou")
	}
}

// COM A CHAVE LIGADA, A CONTA DE JOGO NÃO ENTRA MAIS.
//
// É o estado final que a Hanna liga quando tiver criado os usuários dela. Antes disso os
// dois caminhos convivem, senão o deploy trancaria para fora quem criaria o primeiro.
func TestChaveLigadaFechaOCaminhoAntigo(t *testing.T) {
	f := novoFakePainel()
	contas := contaDeJogo(t, "chefe", "senha-da-conta-de-jogo", roleAdmin)
	h := painelComUsuariosEContas(t, f, contas, true)

	res := postLogin(h, "chefe", "senha-da-conta-de-jogo")

	if res.Code == http.StatusSeeOther && res.Header().Get("Location") == "/" {
		t.Error("a conta de jogo entrou com W2PP_PAINEL_SO_USUARIO ligado")
	}
}

// E COM A CHAVE DESLIGADA A CONTA DE JOGO CONTINUA ENTRANDO.
//
// O espelho do teste de cima, e ele impede o pior defeito desta mudança: uma separação que
// fecha os DOIS caminhos. Sem este teste, um erro no desvio passaria como "chave
// funcionando" e ninguém conseguiria abrir o painel.
func TestChaveDesligadaDeixaAContaDeJogoEntrar(t *testing.T) {
	f := novoFakePainel()
	contas := contaDeJogo(t, "chefe", "senha-da-conta-de-jogo", roleAdmin)
	h := painelComUsuariosEContas(t, f, contas, false)

	res := postLogin(h, "chefe", "senha-da-conta-de-jogo")

	if res.Code != http.StatusSeeOther || res.Header().Get("Location") != "/" {
		t.Errorf("a conta de jogo NAO entrou com a chave desligada: codigo=%d destino=%q",
			res.Code, res.Header().Get("Location"))
	}
}

// A FALHA AO PROCURAR O USUÁRIO NÃO DEIXA CAIR NO CAMINHO ANTIGO.
//
// Se o banco falhar na pergunta "este nome é de painel?", a resposta certa é erro e não
// "então não é". Tratar a falha como "não é" abriria a segunda chance justamente quando o
// sistema está ruim — e é nesse momento que ninguém está olhando.
func TestFalhaAoProcurarNaoAbreOCaminhoAntigo(t *testing.T) {
	f := novoFakePainel()
	f.erroLer = errors.New("banco caiu")
	contas := contaDeJogo(t, "chefe", "senha-da-conta-de-jogo", roleAdmin)
	h := painelComUsuariosEContas(t, f, contas, false)

	res := postLogin(h, "chefe", "senha-da-conta-de-jogo")

	if res.Code != http.StatusInternalServerError {
		t.Errorf("codigo = %d, queria 500: falha na leitura nao pode virar 'nao e de painel'", res.Code)
	}
}

// painelComUsuarios monta o painel com a separação e SEM conta de jogo nenhuma.
func painelComUsuarios(t *testing.T, p UsuariosDoPainel, soUsuario bool) http.Handler {
	t.Helper()
	return painelComUsuariosEContas(t, p, newFakeAccounts(roleAdmin), soUsuario)
}

// painelComUsuariosEContas monta o painel com os dois caminhos de login montados.
func painelComUsuariosEContas(t *testing.T, p UsuariosDoPainel, acc *fakeAccounts, soUsuario bool) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: acc, Writer: newFakeWriter(), Audit: newFakeAudit(),
		Painel: p, SoUsuarioDoPainel: soUsuario,
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

// contaDeJogo devolve um fakeAccounts com uma conta cuja senha é a informada.
//
// SENHA PRÓPRIA e não a do hashOnce compartilhado: o teste da segunda chance precisa que a
// senha da conta de jogo seja DIFERENTE da do usuário do painel, senão ele passaria por
// coincidência — as duas iguais entrariam por qualquer um dos caminhos.
func contaDeJogo(t *testing.T, nome, senha, papel string) *fakeAccounts {
	t.Helper()
	h, err := secret.HashSecret(senha)
	if err != nil {
		t.Fatal(err)
	}
	acc := newFakeAccounts(papel)
	acc.rows[nome] = store.AccountAuth{ID: 42, PassHash: h, Role: papel}
	return acc
}
