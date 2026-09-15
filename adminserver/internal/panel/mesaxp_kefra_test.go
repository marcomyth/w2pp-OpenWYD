package panel

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// caixaXPInteiraMarcada é a caixa da Kefra do simulador como o template escreve
// quando ela vem marcada.
const caixaXPInteiraMarcada = `name="kefra" value="1" checked`

func newTestPanelMesaComEventos(t *testing.T, ev Eventos) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		MesaXP: newFakeMesa(), Eventos: ev,
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

// TestSimuladorComecaComAChaveDaKefraDoBanco: a primeira abertura do simulador
// mostra a XP que o servidor entrega. A caixa vinha sempre marcada, o que simulava
// XP inteira enquanto o padrão do banco (0039) é metade.
func TestSimuladorComecaComAChaveDaKefraDoBanco(t *testing.T) {
	for _, c := range []struct {
		nome   string
		ligada bool
	}{{"chave desligada", false}, {"chave ligada", true}} {
		t.Run(c.nome, func(t *testing.T) {
			cfg := domain.DefaultWorldEventConfig()
			cfg.KefraLiveEnabled = c.ligada
			h := newTestPanelMesaComEventos(t, &fakeEventos{cfg: cfg})
			corpo := abrirMesa(t, h, "?zona=0&evolucao=2").Body.String()
			if marcada := strings.Contains(corpo, caixaXPInteiraMarcada); marcada != c.ligada {
				t.Errorf("caixa de XP inteira marcada = %v, mas a chave no banco está %v", marcada, c.ligada)
			}
		})
	}
}

// TestSimuladorSemAChaveComecaNaMetade: sem a leitura dos eventos, ou com ela
// falhando, a caixa vem desmarcada, que é o padrão do banco.
func TestSimuladorSemAChaveComecaNaMetade(t *testing.T) {
	semEventos := newTestPanelMesa(t, roleAdmin, newFakeMesa(), newFakeAudit())
	if strings.Contains(abrirMesa(t, semEventos, "?zona=0&evolucao=2").Body.String(), caixaXPInteiraMarcada) {
		t.Error("sem a configuração dos eventos a caixa de XP inteira veio marcada")
	}

	cfg := domain.DefaultWorldEventConfig()
	cfg.KefraLiveEnabled = true
	falhando := newTestPanelMesaComEventos(t, &fakeEventos{cfg: cfg, lerErr: errors.New("fake: banco fora")})
	if strings.Contains(abrirMesa(t, falhando, "?zona=0&evolucao=2").Body.String(), caixaXPInteiraMarcada) {
		t.Error("com a leitura dos eventos falhando a caixa de XP inteira veio marcada")
	}
}

// TestSimularUsaACaixaEnviada: depois de simular vale o que veio no formulário,
// e não o banco.
func TestSimularUsaACaixaEnviada(t *testing.T) {
	cfg := domain.DefaultWorldEventConfig()
	cfg.KefraLiveEnabled = true
	h := newTestPanelMesaComEventos(t, &fakeEventos{cfg: cfg})

	sem := abrirMesa(t, h, "?simular=1&zona=0&evolucao=2&mob_exp=20000&mob_nivel=371&nivel=371").Body.String()
	if strings.Contains(sem, caixaXPInteiraMarcada) {
		t.Error("simulação enviada sem a caixa voltou com a caixa de XP inteira marcada")
	}
	com := abrirMesa(t, h, "?simular=1&kefra=1&zona=0&evolucao=2&mob_exp=20000&mob_nivel=371&nivel=371").Body.String()
	if !strings.Contains(com, caixaXPInteiraMarcada) {
		t.Error("simulação enviada com a caixa voltou com a caixa de XP inteira desmarcada")
	}
}

// TestRotuloDaCaixaFalaEmXP: o rótulo diz o que a caixa faz com a XP, como a
// tela de eventos, em vez de "Kefra viva", que no legado quer dizer metade.
func TestRotuloDaCaixaFalaEmXP(t *testing.T) {
	h := newTestPanelMesa(t, roleAdmin, newFakeMesa(), newFakeAudit())
	corpo := abrirMesa(t, h, "?zona=0&evolucao=2").Body.String()
	if !strings.Contains(corpo, "XP inteira (Kefra derrotado; com o Kefra vivo o jogo corta a XP pela metade)") {
		t.Error("o rótulo da caixa não fala em XP inteira")
	}
	if strings.Contains(corpo, "Kefra viva") {
		t.Error("o rótulo antigo \"Kefra viva\" continua na página")
	}
}
