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
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/jogo"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
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

// --- entrega chegando na hora ---

// newTestPanelEntrega wires the mailbox and the game link together, which is the
// only configuration where an immediate delivery is even possible.
func newTestPanelEntregaComJogo(t *testing.T, ent Deliveries, j Live, log AuditLog) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: log,
		Entregas: ent, Jogo: j, GameData: newFakeGameData(), Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

func formEntrega(token string) url.Values {
	return url.Values{
		"csrf": {token}, "item": {"1415"}, "dias": {"0"},
		"eff0": {"0"}, "effv0": {"0"}, "eff1": {"0"}, "effv1": {"0"},
		"eff2": {"0"}, "effv2": {"0"},
	}
}

func TestEntregaChegaNaHoraParaQuemEstaConectado(t *testing.T) {
	// The reason this exists. Before it, every delivery ended with "log out and
	// back in", which is the moment a player finds out the panel cannot hand
	// them anything while they are standing there.
	j := &fakeJogo{estado: jogo.Estado{Players: []jogo.Player{
		{Conta: "ana", Personagem: "Heroina", Jogando: true},
	}}, entregues: 1}
	ent := &fakeEntregas{}
	h := newTestPanelEntregaComJogo(t, ent, j, newFakeAudit())
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/entregar", formEntrega(token))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(j.entregasAgora) != 1 || j.entregasAgora[0] != "ana" {
		t.Fatalf("entregas imediatas = %v, want [ana]", j.entregasAgora)
	}
	destino, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(destino, "chegou agora") {
		t.Errorf("aviso = %q, want dizendo que chegou agora", destino)
	}
	if strings.Contains(destino, "relogar") && !strings.Contains(destino, "sem precisar relogar") {
		t.Errorf("aviso = %q, ainda manda relogar", destino)
	}
}

func TestEntregaParaQuemNaoEstaConectadoContinuaNaFila(t *testing.T) {
	j := &fakeJogo{estado: jogo.Estado{}} // ninguém conectado
	ent := &fakeEntregas{}
	h := newTestPanelEntregaComJogo(t, ent, j, newFakeAudit())
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/entregar", formEntrega(token))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	destino, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(destino, "próximo login") {
		t.Errorf("aviso = %q, want dizendo que chega no próximo login", destino)
	}
	// And the item is queued either way: the mailbox is the delivery, the drain
	// is only about when.
	if len(ent.enfileirou) != 1 {
		t.Errorf("enfileiradas = %d, want 1", len(ent.enfileirou))
	}
}

func TestBauCheioNaoViraEntregaConfirmada(t *testing.T) {
	// The login drain loses these the same way. Reporting a delivery here would
	// send the player looking for something that is not there.
	j := &fakeJogo{estado: jogo.Estado{Players: []jogo.Player{
		{Conta: "ana", Personagem: "Heroina", Jogando: true},
	}}, perdidos: 1}
	h := newTestPanelEntregaComJogo(t, &fakeEntregas{}, j, newFakeAudit())
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/entregar", formEntrega(token))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	destino, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(destino, "cheio") {
		t.Errorf("aviso = %q, não diz que o baú está cheio", destino)
	}
	if strings.Contains(destino, "chegou agora") {
		t.Error("um item que não coube foi anunciado como entregue")
	}
}

func TestJogoForaDoArNaoDerrubaAEntrega(t *testing.T) {
	// The item is in the mailbox before the drain is attempted, so a game server
	// that is down costs the delivery nothing — it arrives at the next login,
	// exactly as it did before any of this existed.
	j := &fakeJogo{estado: jogo.Estado{}, entregarErr: jogo.ErrForaDoAr}
	ent := &fakeEntregas{}
	log := newFakeAudit()
	h := newTestPanelEntregaComJogo(t, ent, j, log)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/entregar", formEntrega(token))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 — a fila não depende do jogo estar no ar", rec.Code)
	}
	if len(ent.enfileirou) != 1 {
		t.Errorf("enfileiradas = %d, want 1", len(ent.enfileirou))
	}
	if len(log.written) != 1 {
		t.Errorf("auditoria = %d, want 1 — a entrega aconteceu", len(log.written))
	}
	destino, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(destino, "próximo login") {
		t.Errorf("aviso = %q", destino)
	}
}

func TestSemLigacaoComOJogoAEntregaSegueComoAntes(t *testing.T) {
	ent := &fakeEntregas{}
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		Entregas: ent, GameData: newFakeGameData(), Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	post, token := signedInPost(t, h.Routes())
	rec := post("/contas/ana/entregar", formEntrega(token))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	destino, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(destino, "próximo login") {
		t.Errorf("aviso = %q", destino)
	}
}
