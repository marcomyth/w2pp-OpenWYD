package panel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/jogo"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

type fakePasse struct {
	gravados map[int64]int16
	erro     error
}

func novoFakePasse() *fakePasse { return &fakePasse{gravados: map[int64]int16{}} }

func (f *fakePasse) DefinirPasseNivel(_ context.Context, accountID int64, nivel int16) error {
	if f.erro != nil {
		return f.erro
	}
	if f.gravados == nil {
		f.gravados = map[int64]int16{}
	}
	f.gravados[accountID] = nivel
	return nil
}

func painelComPasse(t *testing.T, p PasseDaConta, j Live) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		Passe: p, Jogo: j, Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h.Routes()
}

// O NÍVEL É GRAVADO, E O JOGO É AVISADO — nessa ordem.
func TestOPasseEhGravadoEOJogoAvisado(t *testing.T) {
	p := novoFakePasse()
	j := &fakeJogo{passeResp: jogo.Passe{Conectado: true, Personagem: "Heroina"}}
	h := painelComPasse(t, p, j)

	rec := postSigned(t, h, "/contas/ana/passe", url.Values{"nivel": {"3"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d: %s", rec.Code, primeiraLinha(rec.Body.String()))
	}
	if p.gravados[7] != 3 {
		t.Errorf("gravado = %v, quero 3 na conta 7", p.gravados)
	}
	if j.passeConta != "ana" || j.passeNivel != 3 {
		t.Errorf("o jogo recebeu conta=%q nivel=%d", j.passeConta, j.passeNivel)
	}
	if !strings.Contains(rec.Header().Get("Location"), "Heroina") {
		t.Errorf("o aviso nao diz quem viu a moldura mudar: %q", rec.Header().Get("Location"))
	}
}

// O JOGO FORA DO AR NÃO DESFAZ A GRAVAÇÃO.
//
// É a regra que separa as duas coisas: o banco é a verdade e o jogo é cortesia. Se
// uma falha de rede cancelasse a gravação, uma pessoa que pagou pelo passe ficaria
// sem ele porque o servidor de jogo espirrou.
func TestOJogoForaNaoDesfazAGravacao(t *testing.T) {
	p := novoFakePasse()
	j := &fakeJogo{passeErr: errors.New("o jogo nao respondeu")}
	h := painelComPasse(t, p, j)

	rec := postSigned(t, h, "/contas/ana/passe", url.Values{"nivel": {"2"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d; a falha do jogo virou erro de tela", rec.Code)
	}
	if p.gravados[7] != 2 {
		t.Fatalf("a gravacao foi desfeita: %v", p.gravados)
	}
	// E o aviso diz a verdade: gravado, e a moldura fica para o próximo login.
	if !strings.Contains(rec.Header().Get("Location"), "pr%C3%B3ximo+login") {
		t.Errorf("o aviso nao explica o que aconteceu: %q", rec.Header().Get("Location"))
	}
}

// SEM LINK COM O JOGO, GRAVA E DIZ A VERDADE em vez de prometer a moldura agora.
func TestSemLinkGravaEAvisaQueEhNoProximoLogin(t *testing.T) {
	p := novoFakePasse()
	h := painelComPasse(t, p, nil)

	rec := postSigned(t, h, "/contas/ana/passe", url.Values{"nivel": {"1"}})

	if rec.Code != http.StatusSeeOther || p.gravados[7] != 1 {
		t.Fatalf("codigo = %d, gravados = %v", rec.Code, p.gravados)
	}
	if !strings.Contains(rec.Header().Get("Location"), "pr%C3%B3ximo+login") {
		t.Errorf("destino = %q", rec.Header().Get("Location"))
	}
}

// O NÍVEL FORA DA FAIXA NÃO GRAVA NADA.
//
// O byte vai direto para o pacote do cliente; um número que ele não sabe desenhar
// apareceria como outra coisa qualquer.
func TestNivelForaDaFaixaNaoGrava(t *testing.T) {
	for _, v := range []string{"5", "-1", "9", "abc", ""} {
		p := novoFakePasse()
		h := painelComPasse(t, p, nil)

		rec := postSigned(t, h, "/contas/ana/passe", url.Values{"nivel": {v}})

		if rec.Code != http.StatusSeeOther {
			t.Errorf("nivel %q: codigo = %d", v, rec.Code)
		}
		if len(p.gravados) != 0 {
			t.Errorf("nivel %q gravou %v", v, p.gravados)
		}
	}
}

// E O ZERO GRAVA, porque tirar o passe é uma operação legítima — e um zero tratado
// como "campo vazio" deixaria a staff sem como tirar o que deu por engano.
func TestOZeroTiraOPasse(t *testing.T) {
	p := novoFakePasse()
	h := painelComPasse(t, p, nil)

	rec := postSigned(t, h, "/contas/ana/passe", url.Values{"nivel": {"0"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d", rec.Code)
	}
	nivel, gravou := p.gravados[7]
	if !gravou || nivel != 0 {
		t.Errorf("o zero nao foi gravado: %v", p.gravados)
	}
}

// A GRAVAÇÃO QUE FALHA NÃO AVISA O JOGO, senão a moldura mudaria na tela de alguém
// sem nada por trás — e sumiria no próximo login, sem ninguém entender.
func TestGravacaoQueFalhaNaoAvisaOJogo(t *testing.T) {
	p := novoFakePasse()
	p.erro = errors.New("o banco caiu")
	j := &fakeJogo{}
	h := painelComPasse(t, p, j)

	rec := postSigned(t, h, "/contas/ana/passe", url.Values{"nivel": {"4"}})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("codigo = %d, quero 500", rec.Code)
	}
	if j.passeConta != "" {
		t.Errorf("avisou o jogo depois de a gravacao falhar: conta=%q", j.passeConta)
	}
}

// E o store recusa o nível inválido por conta própria, sem depender da tela.
func TestOStoreRecusaNivelInvalido(t *testing.T) {
	for _, n := range []int16{-1, 5, 100} {
		if !errors.Is(erroDoNivel(n), store.ErrPasseNivelInvalido) {
			t.Errorf("nivel %d nao foi recusado", n)
		}
	}
}

// erroDoNivel exercita só a conferência de faixa do store, sem banco: a função
// recusa antes de tocar no pool, e é isso que este teste amarra.
func erroDoNivel(n int16) error {
	var s store.Store
	return s.DefinirPasseNivel(context.Background(), 1, n)
}
