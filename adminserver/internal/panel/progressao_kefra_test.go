package panel

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// mortesDaProgressao é o total de mortes do plano como o template escreve.
var mortesDaProgressao = regexp.MustCompile(`<div class="rot">Mortes</div>\s*<div class="val">(\d+)</div>`)

// planoComEventos abre o plano de progressão de uma rota só com a Kentania (nível
// 10, 5000 de XP no fake) e devolve o total de mortes e a página. ev nil é um
// painel sem a leitura dos eventos.
func planoComEventos(t *testing.T, ev Eventos) (int64, string) {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		MesaXP: newFakeMesa(), GameData: newFakeGameData(), Eventos: ev,
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rotas := h.Routes()
	c := sessionCookie(postLogin(rotas, "chefe", testPassword))
	if c == nil {
		t.Fatal("o login não devolveu cookie")
	}
	req := httptest.NewRequest(http.MethodGet, "/rates/progressao?horas=6&meta=45&mob=Kentania&zona=0&desde=1", nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	rotas.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	corpo := rec.Body.String()
	m := mortesDaProgressao.FindStringSubmatch(corpo)
	if m == nil {
		t.Fatal("a página não mostrou o total de mortes do plano")
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		t.Fatalf("mortes %q: %v", m[1], err)
	}
	return n, corpo
}

func eventosComAChave(ligada bool) *fakeEventos {
	cfg := domain.DefaultWorldEventConfig()
	cfg.KefraLiveEnabled = ligada
	return &fakeEventos{cfg: cfg}
}

// TestProgressaoUsaAChaveDaKefraDoBanco: o plano gravava KefraLive: true direto no
// código, então mostrava sempre a XP inteira, enquanto o padrão do banco (0039) é
// metade. Com a chave desligada o mesmo plano precisa de mais mortes, e a página
// diz com qual XP a conta foi feita.
func TestProgressaoUsaAChaveDaKefraDoBanco(t *testing.T) {
	metade, corpoMetade := planoComEventos(t, eventosComAChave(false))
	inteira, corpoInteira := planoComEventos(t, eventosComAChave(true))
	if metade <= inteira {
		t.Errorf("com a chave desligada o plano deu %d mortes e com ela ligada %d; metade da XP tinha de pedir mais mortes",
			metade, inteira)
	}
	if !strings.Contains(corpoMetade, "Conta feita com metade da XP") {
		t.Error("com a chave desligada a página não diz que a conta usa metade da XP")
	}
	if !strings.Contains(corpoInteira, "Conta feita com a XP inteira") {
		t.Error("com a chave ligada a página não diz que a conta usa a XP inteira")
	}
}

// TestProgressaoSemEventosFicaNaMetade: sem a leitura dos eventos o plano usa o
// padrão do banco, metade da XP, e não a XP inteira.
func TestProgressaoSemEventosFicaNaMetade(t *testing.T) {
	sem, _ := planoComEventos(t, nil)
	desligada, _ := planoComEventos(t, eventosComAChave(false))
	if sem != desligada {
		t.Errorf("sem eventos o plano deu %d mortes, com a chave desligada %d; tinham de ser iguais", sem, desligada)
	}
}
