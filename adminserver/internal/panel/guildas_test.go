package panel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

type fakeGuildas struct {
	mu        sync.Mutex
	guildas   []domain.Guild
	membros   map[uint16][]domain.GuildMember
	contagem  map[uint16]int
	relacoes  []domain.GuildRelation
	zonas     []domain.GuildZone
	listaErr  error
	membroErr error
	contaErr  error
	relErr    error
	zonaErr   error
}

func (f *fakeGuildas) ListGuilds(context.Context) ([]domain.Guild, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.guildas, f.listaErr
}

func (f *fakeGuildas) ListGuildMembers(_ context.Context, id uint16) ([]domain.GuildMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.membros[id], f.membroErr
}

func (f *fakeGuildas) CountGuildMembers(context.Context) (map[uint16]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.contagem, f.contaErr
}

func (f *fakeGuildas) ListGuildRelations(context.Context) ([]domain.GuildRelation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.relacoes, f.relErr
}

func (f *fakeGuildas) LoadGuildZones(context.Context) ([]domain.GuildZone, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.zonas, f.zonaErr
}

func newTestPanelGuildas(t *testing.T, g Guildas) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		Guildas: g, Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

func mundoDeGuildas() *fakeGuildas {
	return &fakeGuildas{
		guildas: []domain.Guild{
			{ID: 1, Name: "Pequena", Fame: 10},
			{ID: 2, Name: "Grande", Fame: 900},
		},
		contagem: map[uint16]int{1: 2, 2: 30},
		relacoes: []domain.GuildRelation{
			{GuildID: 2, TargetGuildID: 1, Kind: domain.GuildRelationWar},
		},
		zonas: []domain.GuildZone{
			{Zone: 0, ChargeGuild: 2, CityTax: 12, TaxVault: 5000},
			{Zone: 1},
		},
		membros: map[uint16][]domain.GuildMember{
			2: {
				{GuildID: 2, Name: "Chefia", AccountName: "lokitoo", Level: 9},
				{GuildID: 2, Name: "Peao", AccountName: "outro", Level: 1},
			},
		},
	}
}

func TestAListaDeGuildasComecaPelaMaior(t *testing.T) {
	// A list of guilds is read to find the ones that matter, and what makes a
	// guild matter is how many people are in it.
	body := getSignedIn(t, newTestPanelGuildas(t, mundoDeGuildas()), "/guildas").Body.String()
	if strings.Index(body, "Grande") > strings.Index(body, "Pequena") {
		t.Error("a guilda pequena veio antes da grande")
	}
	if !strings.Contains(body, ">30<") {
		t.Error("a contagem de membros não apareceu")
	}
}

func TestOIdDaCidadeViraNome(t *testing.T) {
	// guild_zone is keyed 0..4. Printing "zona 0" would make the moderator go
	// look up which city that is.
	body := getSignedIn(t, newTestPanelGuildas(t, mundoDeGuildas()), "/guildas").Body.String()
	if !strings.Contains(body, "Armia") {
		t.Error("a cidade apareceu como número em vez de nome")
	}
	// And the city says who holds it, by guild name rather than by id.
	if !strings.Contains(body, "12%") {
		t.Error("o imposto da cidade não apareceu")
	}
}

func TestAGuerraApareceComoNomeDaOutraGuilda(t *testing.T) {
	body := getSignedIn(t, newTestPanelGuildas(t, mundoDeGuildas()), "/guildas").Body.String()
	if !strings.Contains(body, "chip-danger") {
		t.Error("a guerra não foi destacada")
	}
}

func TestAPaginaDaGuildaListaOsMembrosComAConta(t *testing.T) {
	// The row names a CHARACTER, and every panel action works in accounts — so
	// the account is the part that lets a moderator do anything about it.
	body := getSignedIn(t, newTestPanelGuildas(t, mundoDeGuildas()), "/guildas/2").Body.String()
	if !strings.Contains(body, "Chefia") || !strings.Contains(body, "Peao") {
		t.Error("os membros não apareceram")
	}
	if !strings.Contains(body, `href="/contas/lokitoo"`) {
		t.Error("o membro não leva para a conta dele")
	}
	if !strings.Contains(body, "líder") {
		t.Error("o líder não foi marcado")
	}
}

func TestGuildaInexistenteDa404(t *testing.T) {
	rec := getSignedIn(t, newTestPanelGuildas(t, mundoDeGuildas()), "/guildas/999")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestATelaDizQueSoMostra(t *testing.T) {
	// The absence of writes is a decision, not an unfinished half: the game owns
	// this state in its loop and rewrites it. Saying so is what stops somebody
	// asking for an edit button that could only lie.
	body := getSignedIn(t, newTestPanelGuildas(t, mundoDeGuildas()), "/guildas").Body.String()
	if !strings.Contains(body, "só mostra") {
		t.Error("a página não avisa que é somente leitura")
	}
	if !strings.Contains(body, "gm guildname") {
		t.Error("a página não aponta o caminho que o jogo respeita")
	}
}

func TestUmaLeituraQuebradaNaoDerrubaAPagina(t *testing.T) {
	// The roster of guilds is the page; the counts, relations and cities are what
	// it says about each one. Losing an extra must not blank the list.
	g := mundoDeGuildas()
	g.contaErr = errors.New("falha de leitura")
	g.relErr = errors.New("falha de leitura")
	g.zonaErr = errors.New("falha de leitura")

	rec := getSignedIn(t, newTestPanelGuildas(t, g), "/guildas")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Grande") {
		t.Error("a lista sumiu porque um extra falhou")
	}
}

func TestSemGuildasConfiguradasONavNaoOferece(t *testing.T) {
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if body := getSignedIn(t, h.Routes(), "/contas").Body.String(); strings.Contains(body, ">Guildas<") {
		t.Error("o menu oferece a página sem ela existir")
	}
}
