package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/jogo"
)

// --- desatolar pelo painel ---

func TestDesatolarMoveEAudita(t *testing.T) {
	j := &fakeJogo{estado: estadoDeTeste()}
	log := newFakeAudit()
	h := newTestPanelJogo(t, log, j)
	post, token := signedInPost(t, h)

	rec := post("/servidor/desatolar", url.Values{"csrf": {token}, "conta": {"ana"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(j.desatolados) != 1 || j.desatolados[0] != "ana" {
		t.Fatalf("desatolados = %v, want [ana]", j.desatolados)
	}
	// Moving somebody's character is an action on a player, so it is on the
	// record with where they came from and where they went.
	if len(log.written) != 1 || log.written[0].Action != audit.ActionUnstuck {
		t.Fatalf("auditoria = %+v, want um UNSTUCK", log.written)
	}
	campos, ok := log.written[0].New.(map[string]any)
	if !ok || campos["cidade"] != "Armia" {
		t.Errorf("auditoria não guardou para onde foi: %+v", log.written[0].New)
	}
	// And the operator is told what to say to the player.
	if destino := rec.Header().Get("Location"); !strings.Contains(destino, "Armia") {
		t.Errorf("aviso = %q, não diz para onde o personagem foi", destino)
	}
}

func TestDesatolarDeQuemNaoEstaNoMundoNaoAudita(t *testing.T) {
	// The player may have logged off between the report and the click. Nothing
	// happened to anybody, so nothing goes on the record — an audit line saying
	// a character was moved when none was is worse than no line.
	j := &fakeJogo{estado: estadoDeTeste()}
	log := newFakeAudit()
	h := newTestPanelJogo(t, log, j)
	post, token := signedInPost(t, h)

	rec := post("/servidor/desatolar", url.Values{"csrf": {token}, "conta": {"bruno"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if len(log.written) != 0 {
		t.Errorf("auditoria = %+v, want vazia", log.written)
	}
	if destino := rec.Header().Get("Location"); !strings.Contains(destino, "n%C3%A3o+est%C3%A1") {
		t.Errorf("aviso = %q, não explica que a conta não está no mundo", destino)
	}
}

func TestDesatolarExigeUmaConta(t *testing.T) {
	h := newTestPanelJogo(t, newFakeAudit(), &fakeJogo{estado: estadoDeTeste()})
	post, token := signedInPost(t, h)
	if rec := post("/servidor/desatolar", url.Values{"csrf": {token}}); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDesatolarComJogoForaDoArAvisa(t *testing.T) {
	j := &fakeJogo{estado: estadoDeTeste(), desatolarErr: jogo.ErrForaDoAr}
	h := newTestPanelJogo(t, newFakeAudit(), j)
	post, token := signedInPost(t, h)
	if rec := post("/servidor/desatolar", url.Values{"csrf": {token}, "conta": {"ana"}}); rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestOBotaoDeDesatolarSoApareceParaQuemEstaNoMundo(t *testing.T) {
	// A session still on the character screen has no position to fix. Offering
	// the button there is offering something that cannot work.
	j := &fakeJogo{estado: estadoDeTeste()} // ana está jogando, bruno escolhendo
	body := getSignedIn(t, newTestPanelJogo(t, newFakeAudit(), j), "/servidor").Body.String()
	if strings.Count(body, "/servidor/desatolar") != 1 {
		t.Errorf("botões de desatolar = %d, want 1 (só a ana)",
			strings.Count(body, "/servidor/desatolar"))
	}
	if strings.Count(body, "/servidor/derrubar") != 2 {
		t.Errorf("botões de derrubar = %d, want 2 — derrubar vale para os dois",
			strings.Count(body, "/servidor/derrubar"))
	}
}
