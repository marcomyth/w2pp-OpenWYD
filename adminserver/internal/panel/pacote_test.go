package panel

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/donate"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
)

func newTestPanelPacote(t *testing.T, role string, cart *fakeCarteira, log AuditLog) http.Handler {
	t.Helper()
	g := newFakeGameData()
	g.itens = append(g.itens,
		gamedata.Item{Index: 3991, Name: "Dragao_Vermelho", DisplayName: "Dragão Vermelho"},
		gamedata.Item{Index: 3305, Name: "Bau_do_Apoiador", DisplayName: "Baú do Apoiador"},
	)
	h, err := New(Config{
		Accounts:   withTarget(role),
		Writer:     newFakeWriter(),
		GameData:   g,
		Entregas:   &fakeEntregas{},
		Carteira:   cart,
		Audit:      log,
		Sessions:   session.New(time.Hour),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

// TestPacoteEnviaEAudita: o admin escolhe o pacote e o motivo, a carteira
// recebe quem enviou e para quem, e a auditoria do painel guarda o pacote, os
// Rcoins e as linhas da fila.
func TestPacoteEnviaEAudita(t *testing.T) {
	cart := &fakeCarteira{saldo: 500}
	log := newFakeAudit()
	post, token := signedInPost(t, newTestPanelPacote(t, roleAdmin, cart, log))

	rec := post("/contas/ana/pacote", url.Values{
		"csrf": {token}, "pacote": {"apoiador-supremo"}, "motivo": {"influencer Fulano"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(cart.envios) != 1 {
		t.Fatalf("envios = %d, want 1", len(cart.envios))
	}
	// O ator vem da sessão, nunca do formulário.
	if e := cart.envios[0]; e.ator != 42 || e.conta != 7 || e.pacote != "apoiador-supremo" || e.motivo != "influencer Fulano" {
		t.Errorf("envio = %+v, want ator 42, conta 7, apoiador-supremo, influencer Fulano", e)
	}

	regs := log.recorded()
	if len(regs) != 1 || regs[0].Action != audit.ActionSendSupporterPack || regs[0].TargetID != 7 {
		t.Fatalf("auditoria = %+v, want um SEND_SUPPORTER_PACK na conta 7", regs)
	}
	novo, _ := regs[0].New.(map[string]any)
	if novo["pacote"] != "apoiador-supremo" || novo["rcoins"] != int32(20000) {
		t.Errorf("auditoria sem o pacote ou os Rcoins: %v", novo)
	}

	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "aba=itens") || !strings.Contains(loc, "20000 Rcoins") {
		t.Errorf("redirect = %q, want a aba Itens dizendo os Rcoins", loc)
	}
}

// TestPacoteSoAdmin: moderador não envia. O pacote cria moeda de donate, e o
// ajuste de saldo, que faz o mesmo, já é só de admin.
func TestPacoteSoAdmin(t *testing.T) {
	cart := &fakeCarteira{}
	log := newFakeAudit()
	h := newTestPanelPacote(t, "moderator", cart, log)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/pacote", url.Values{
		"csrf": {token}, "pacote": {"apoiador-supremo"}, "motivo": {"x"},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if len(cart.envios) != 0 || len(log.recorded()) != 0 {
		t.Errorf("um moderador enviou o pacote: %+v", cart.envios)
	}

	// E o cartão nem aparece para ele.
	if body := signedIn(t, h)("/contas/ana?aba=itens").Body.String(); strings.Contains(body, "Enviar pacote de apoiador") {
		t.Error("o cartão do pacote aparece para moderador")
	}
}

// TestPacoteRecusaAPropriaConta: o admin não se dá um pacote, nem chamando a rota
// direto.
func TestPacoteRecusaAPropriaConta(t *testing.T) {
	cart := &fakeCarteira{}
	post, token := signedInPost(t, newTestPanelPacote(t, roleAdmin, cart, newFakeAudit()))

	rec := post("/contas/chefe/pacote", url.Values{
		"csrf": {token}, "pacote": {"apoiador-supremo"}, "motivo": {"x"},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if len(cart.envios) != 0 {
		t.Errorf("enviou para a própria conta: %+v", cart.envios)
	}
}

// TestPacoteRecusasViramAviso: as recusas previstas voltam para a aba Itens com
// uma frase, e nenhuma delas entra na auditoria — nada foi dado.
func TestPacoteRecusasViramAviso(t *testing.T) {
	for _, c := range []struct {
		nome   string
		err    error
		trecho string
	}{
		{"repetido", donate.ErrPacoteRepetido, "menos de um minuto"},
		{"sem motivo", donate.ErrMotivoVazio, "motivo"},
		{"fora de venda", donate.ErrPacoteIndisponivel, "não está disponível"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			cart := &fakeCarteira{erroEnvio: c.err}
			log := newFakeAudit()
			post, token := signedInPost(t, newTestPanelPacote(t, roleAdmin, cart, log))
			rec := post("/contas/ana/pacote", url.Values{
				"csrf": {token}, "pacote": {"apoiador-supremo"}, "motivo": {"x"},
			})
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", rec.Code)
			}
			loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
			if !strings.Contains(loc, c.trecho) {
				t.Errorf("aviso = %q, want algo com %q", loc, c.trecho)
			}
			if len(log.recorded()) != 0 {
				t.Error("uma recusa entrou na auditoria")
			}
		})
	}
}

// TestCartaoDoPacoteMostraOConteudo: o admin vê, na aba Itens, o que cada pacote
// entrega — com o nome do catálogo, a duração e a pilha —, e não vê o cartão nas
// outras abas.
func TestCartaoDoPacoteMostraOConteudo(t *testing.T) {
	get := signedIn(t, newTestPanelPacote(t, roleAdmin, &fakeCarteira{}, newFakeAudit()))

	body := get("/contas/ana?aba=itens").Body.String()
	for _, want := range []string{
		"Enviar pacote de apoiador", `action="/contas/ana/pacote"`,
		`value="apoiador-supremo"`, "Apoiador Supremo", "20000",
		"Dragão Vermelho (15 dias)", "64× Baú do Apoiador",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a aba Itens não tem %q", want)
		}
	}
	if strings.Contains(get("/contas/ana").Body.String(), "Enviar pacote de apoiador") {
		t.Error("o cartão do pacote aparece fora da aba Itens")
	}
}
