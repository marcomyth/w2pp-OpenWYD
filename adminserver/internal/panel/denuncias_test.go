package panel

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

type fakeDenuncias struct {
	mu          sync.Mutex
	fila        []domain.PlayerReport
	consulta    []store.ReportQuery
	tratadas    []int64
	porQuem     []int64
	listErr     error
	contagemErr error
	marcaErr    error
}

func (f *fakeDenuncias) ListReports(_ context.Context, q store.ReportQuery) ([]domain.PlayerReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consulta = append(f.consulta, q)
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []domain.PlayerReport
	for _, r := range f.fila {
		if q.SoAbertos && !r.Aberto() {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeDenuncias) CountReports(context.Context) (store.ReportCounts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.contagemErr != nil {
		return store.ReportCounts{}, f.contagemErr
	}
	c := store.ReportCounts{Total: len(f.fila)}
	for _, r := range f.fila {
		if r.Aberto() {
			c.Abertos++
			if c.MaisAntigo.IsZero() || r.At.Before(c.MaisAntigo) {
				c.MaisAntigo = r.At
			}
		}
	}
	return c, nil
}

func (f *fakeDenuncias) MarkReportHandled(_ context.Context, id, staffID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.marcaErr != nil {
		return f.marcaErr
	}
	f.tratadas = append(f.tratadas, id)
	f.porQuem = append(f.porQuem, staffID)
	return nil
}

func newTestPanelDenuncias(t *testing.T, cargo string, d Denuncias, log AuditLog) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(cargo), Writer: newFakeWriter(), Audit: log, Denuncias: d,
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

func denunciaAberta() domain.PlayerReport {
	return domain.PlayerReport{
		ID: 1, At: time.Now().Add(-3 * time.Hour),
		Account: "lokitoo", Character: "Vandalyzz", Level: 220,
		Text: "o cara ali esta usando bot", X: 2800, Y: 2600,
		Nearby: []string{"Xdfgh", "Xdfgi"},
	}
}

func denunciaTratada() domain.PlayerReport {
	tratada := time.Now().Add(-time.Hour)
	r := denunciaAberta()
	r.ID = 2
	r.Character = "JaResolvida"
	r.Text = "isso ja foi visto"
	r.HandledAt = &tratada
	return r
}

func TestAPaginaMostraORelatoEOContexto(t *testing.T) {
	// The report and the snapshot together are the point. Either alone sends the
	// moderator back to asking the player to reproduce it.
	d := &fakeDenuncias{fila: []domain.PlayerReport{denunciaAberta()}}
	body := getSignedIn(t, newTestPanelDenuncias(t, roleAdmin, d, newFakeAudit()), "/denuncias").Body.String()

	if !strings.Contains(body, "o cara ali esta usando bot") {
		t.Error("o relato não apareceu")
	}
	if !strings.Contains(body, "2800") || !strings.Contains(body, "2600") {
		t.Error("a posição não apareceu")
	}
	// The coordinates alone are two numbers; the region is what a person reads.
	if !strings.Contains(body, "Campo") && !strings.Contains(body, "Armia") {
		t.Error("a página não diz que lugar é aquele")
	}
	if !strings.Contains(body, "Xdfgh") {
		t.Error("quem estava por perto não apareceu")
	}
	// The account is a link, because the next move is always to open it.
	if !strings.Contains(body, `href="/contas/lokitoo"`) {
		t.Error("a denúncia não leva para a conta de quem reportou")
	}
}

func TestAFilaAbreNoQueFaltaTratar(t *testing.T) {
	// The page exists to answer "what needs me now". Opening on everything ever
	// reported would bury that under months of closed rows.
	d := &fakeDenuncias{fila: []domain.PlayerReport{denunciaAberta(), denunciaTratada()}}
	h := newTestPanelDenuncias(t, roleAdmin, d, newFakeAudit())

	body := getSignedIn(t, h, "/denuncias").Body.String()
	if strings.Contains(body, "JaResolvida") {
		t.Error("a fila abriu com denúncia já tratada")
	}
	if len(d.consulta) != 1 || !d.consulta[0].SoAbertos {
		t.Fatalf("consulta = %+v, want só abertas", d.consulta)
	}

	// And everything is one click away.
	todas := getSignedIn(t, h, "/denuncias?todas=1").Body.String()
	if !strings.Contains(todas, "JaResolvida") {
		t.Error("o filtro Todas não mostra as tratadas")
	}
}

func TestAPaginaDizOPrazo(t *testing.T) {
	// The row holds a person's words and the names of people who were merely
	// standing nearby. Whoever reads it should know it goes away.
	d := &fakeDenuncias{fila: []domain.PlayerReport{denunciaAberta()}}
	body := getSignedIn(t, newTestPanelDenuncias(t, roleAdmin, d, newFakeAudit()), "/denuncias").Body.String()
	if !strings.Contains(body, "dias") {
		t.Error("a página não diz que a denúncia expira")
	}
	// And that being nearby is not an accusation.
	if !strings.Contains(body, "indício, não acusação") {
		t.Error("a página não separa estar por perto de ser culpado")
	}
}

func TestModeradorTrataDenuncia(t *testing.T) {
	// Answering reports is the job. A queue only the admin can clear is a queue
	// nobody clears.
	d := &fakeDenuncias{fila: []domain.PlayerReport{denunciaAberta()}}
	log := newFakeAudit()
	h := newTestPanelDenuncias(t, roleModerator, d, log)
	post, token := signedInPost(t, h)

	rec := post("/denuncias/1/tratar", url.Values{"csrf": {token}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(d.tratadas) != 1 || d.tratadas[0] != 1 {
		t.Fatalf("tratadas = %v, want [1]", d.tratadas)
	}
	// The actor comes from the session, never from the form.
	if len(d.porQuem) != 1 || d.porQuem[0] == 0 {
		t.Errorf("quem tratou = %v, want a conta da sessão", d.porQuem)
	}
	if len(log.written) != 1 || log.written[0].Action != audit.ActionHandleReport {
		t.Fatalf("auditoria = %+v", log.written)
	}
}

func TestTratarPrecisaDoCSRF(t *testing.T) {
	d := &fakeDenuncias{fila: []domain.PlayerReport{denunciaAberta()}}
	h := newTestPanelDenuncias(t, roleAdmin, d, newFakeAudit())
	post, _ := signedInPost(t, h)

	rec := post("/denuncias/1/tratar", url.Values{})
	if rec.Code == http.StatusSeeOther {
		t.Fatal("tratou sem o token")
	}
	if len(d.tratadas) != 0 {
		t.Error("marcou sem o token")
	}
}

func TestFilaVaziaDizQueEstaVazia(t *testing.T) {
	d := &fakeDenuncias{}
	body := getSignedIn(t, newTestPanelDenuncias(t, roleAdmin, d, newFakeAudit()), "/denuncias").Body.String()
	if !strings.Contains(body, "Nenhuma denúncia aberta") {
		t.Error("a fila vazia não diz que está vazia")
	}
}

func TestIdade(t *testing.T) {
	// The panel prints an age, not a timestamp: "há 3 h" is what says whether a
	// queue is being worked; a date makes the reader do the subtraction. And it
	// is ONE formatter — there used to be two, disagreeing on adjacent screens.
	agora := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		nome  string
		desde time.Duration
		want  string
	}{
		{"segundos", 30 * time.Second, "menos de um minuto"},
		{"minutos", 42 * time.Minute, "42 min"},
		{"horas", 5 * time.Hour, "5 h"},
		{"quase um dia", 23*time.Hour + 59*time.Minute, "23 h"},
		{"um dia", 25 * time.Hour, "1 dia"},
		{"dias", 50 * time.Hour, "2 dias"},
		{"um mês", 61 * 24 * time.Hour, "2 meses"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := idade(agora.Add(-c.desde), agora); got != c.want {
				t.Errorf("idade = %q, want %q", got, c.want)
			}
		})
	}
	// Sem data não é data zero: a diferença entre "não sei quando" e "há
	// cinquenta e seis anos".
	if got := idade(time.Time{}, agora); got != "" {
		t.Errorf("sem data = %q, want vazio", got)
	}
	// Relógio adiantado é relógio errado, não idade negativa. Dizer "menos de um
	// minuto" esconderia o problema.
	if got := idade(agora.Add(time.Hour), agora); got != "data no futuro" {
		t.Errorf("data futura = %q", got)
	}
}

// A frase tem de encaixar depois de "há" e depois de "No ar há", que são os dois
// lugares onde ela aparece. Uma que já começasse com "há" leria "há há".
func TestIdadeEncaixaDepoisDeHa(t *testing.T) {
	agora := time.Now()
	for _, d := range []time.Duration{30 * time.Second, 5 * time.Minute, 5 * time.Hour, 50 * time.Hour} {
		got := idade(agora.Add(-d), agora)
		if strings.HasPrefix(got, "há") || strings.HasSuffix(got, "atrás") {
			t.Errorf("idade(%v) = %q; a frase não pode trazer o \"há\" dentro", d, got)
		}
	}
}

func TestSemDenunciasConfiguradasONavNaoOferece(t *testing.T) {
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	body := getSignedIn(t, h.Routes(), "/contas").Body.String()
	if strings.Contains(body, ">Denúncias<") {
		t.Error("o menu oferece a página sem ela existir")
	}
}
