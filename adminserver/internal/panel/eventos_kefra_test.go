package panel

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestQuadroDoKefraMostraONomeDaGuilda: com a lista de guildas, o quadro diz o
// nome de quem matou; sem ela, fica o número.
func TestQuadroDoKefraMostraONomeDaGuilda(t *testing.T) {
	cfg := domain.DefaultWorldEventConfig()
	cfg.KefraLiveEnabled, cfg.KefraGuildID = true, 7
	for _, c := range []struct {
		nome    string
		guildas Guildas
		quer    string
	}{
		{"com a lista de guildas", &fakeGuildas{guildas: []domain.Guild{{ID: 7, Name: "Os Sete"}}}, "derrotado pela guilda Os Sete"},
		{"sem a lista de guildas", nil, "derrotado pela guilda 7"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			conf := Config{
				Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
				Eventos:  &fakeEventos{cfg: cfg},
				Sessions: session.New(time.Hour),
				Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
			}
			if c.guildas != nil {
				conf.Guildas = c.guildas
			}
			h, err := New(conf)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			body := getSignedIn(t, h.Routes(), "/eventos").Body.String()
			if !strings.Contains(body, c.quer) {
				t.Errorf("o quadro do Kefra não diz %q", c.quer)
			}
		})
	}
}

// TestFormularioDosEventosNaoMexeNaKefra: o estado do Kefra também é gravado pelo
// jogo (a morte do chefe e a terça). Um formulário de eventos aberto antes de o
// Kefra morrer não pode, ao salvar outra coisa, gravar de volta o valor velho —
// nem o que a caixa dissesse.
func TestFormularioDosEventosNaoMexeNaKefra(t *testing.T) {
	for _, c := range []struct {
		nome        string
		noBanco     bool
		caixaNoForm bool
	}{
		{"derrotado no banco, formulário sem a caixa", true, false},
		{"vivo no banco, formulário com a caixa", false, true},
	} {
		t.Run(c.nome, func(t *testing.T) {
			cfg := domain.DefaultWorldEventConfig()
			cfg.KefraLiveEnabled = c.noBanco
			ev := &fakeEventos{cfg: cfg}
			post, token := signedInPost(t, newTestPanelEventos(t, roleAdmin, ev, newFakeAudit()))

			form := url.Values{"csrf": {token}, "torre": {"1"}, "torre_hora": {"20"}, "chefes_horas": {"24"}}
			if c.caixaNoForm {
				form.Set("kefra", "1")
			}
			if rec := post("/eventos", form); rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
			}
			if len(ev.gravado) != 1 {
				t.Fatalf("gravações = %d, want 1", len(ev.gravado))
			}
			if got := ev.gravado[0].KefraLiveEnabled; got != c.noBanco {
				t.Errorf("o formulário gravou Kefra derrotado = %v; o banco tinha %v", got, c.noBanco)
			}
			if len(ev.kefra) != 0 {
				t.Errorf("o formulário chamou a gravação do Kefra: %+v", ev.kefra)
			}
		})
	}
}

// TestAdminMarcaOEstadoDoKefra: a correção manual é uma ação à parte, gravada
// pelo mesmo caminho do jogo, com a fonte "painel", e fica na auditoria.
func TestAdminMarcaOEstadoDoKefra(t *testing.T) {
	for _, c := range []struct {
		estado    string
		derrotado bool
	}{
		{"derrotado", true},
		{"vivo", false},
	} {
		t.Run(c.estado, func(t *testing.T) {
			cfg := domain.DefaultWorldEventConfig()
			cfg.KefraLiveEnabled = !c.derrotado
			ev := &fakeEventos{cfg: cfg}
			log := newFakeAudit()
			post, token := signedInPost(t, newTestPanelEventos(t, roleAdmin, ev, log))

			rec := post("/eventos/kefra", url.Values{"csrf": {token}, "estado": {c.estado}})
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
			}
			if len(ev.kefra) != 1 {
				t.Fatalf("gravações do Kefra = %d, want 1", len(ev.kefra))
			}
			g := ev.kefra[0]
			if g.derrotado != c.derrotado || g.guilda != 0 || g.fonte != "painel" || g.ator == 0 {
				t.Errorf("gravação = %+v, want derrotado=%v guilda=0 fonte=painel e o admin como ator", g, c.derrotado)
			}
			if len(ev.gravado) != 0 {
				t.Error("a ação do Kefra regravou o formulário inteiro dos eventos")
			}
			// A constante é audit.ActionSetKefra; o literal deixa este teste compilar
			// antes de ela existir.
			if len(log.written) != 1 || log.written[0].Action != "SET_KEFRA" {
				t.Fatalf("auditoria = %+v, want uma linha SET_KEFRA", log.written)
			}
		})
	}
}

func TestModeradorNaoMarcaOEstadoDoKefra(t *testing.T) {
	ev := &fakeEventos{cfg: domain.DefaultWorldEventConfig()}
	post, token := signedInPost(t, newTestPanelEventos(t, roleModerator, ev, newFakeAudit()))
	rec := post("/eventos/kefra", url.Values{"csrf": {token}, "estado": {"derrotado"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(ev.kefra) != 0 {
		t.Error("um moderador gravou o estado do Kefra")
	}
}

func TestEstadoDoKefraInvalidoERecusado(t *testing.T) {
	ev := &fakeEventos{cfg: domain.DefaultWorldEventConfig()}
	post, token := signedInPost(t, newTestPanelEventos(t, roleAdmin, ev, newFakeAudit()))
	for _, estado := range []string{"", "abacaxi", "1"} {
		rec := post("/eventos/kefra", url.Values{"csrf": {token}, "estado": {estado}})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("estado %q: status = %d, want 400", estado, rec.Code)
		}
	}
	if len(ev.kefra) != 0 {
		t.Errorf("gravou com estado inválido: %+v", ev.kefra)
	}
}

// TestAPaginaTemAAcaoDoKefraForaDoFormulario: a caixa saiu do formulário e a ação
// tem o próprio formulário; o moderador vê o estado e não vê os botões.
func TestAPaginaTemAAcaoDoKefraForaDoFormulario(t *testing.T) {
	ev := &fakeEventos{cfg: domain.DefaultWorldEventConfig()}
	admin := getSignedIn(t, newTestPanelEventos(t, roleAdmin, ev, newFakeAudit()), "/eventos").Body.String()
	if strings.Contains(admin, `name="kefra"`) {
		t.Error("a caixa kefra continua no formulário dos eventos")
	}
	if !strings.Contains(admin, `action="/eventos/kefra"`) {
		t.Error("a página não tem a ação própria do Kefra")
	}
	mod := getSignedIn(t, newTestPanelEventos(t, roleModerator, ev, newFakeAudit()), "/eventos").Body.String()
	if strings.Contains(mod, `action="/eventos/kefra"`) {
		t.Error("o moderador vê os botões do Kefra, que só vão dar 403")
	}
	if !strings.Contains(mod, "Kefra vivo") {
		t.Error("o moderador não vê em que estado o Kefra está")
	}
}
