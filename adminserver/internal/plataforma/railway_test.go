package plataforma

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// capture stands a server in front of the client and records the one request it
// receives, so a test can assert on how the call was signed.
func capture(t *testing.T, cfg Config, resposta string) *http.Request {
	t.Helper()
	var visto *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resposta))
	}))
	defer srv.Close()

	c := New(cfg)
	c.endpoint = srv.URL
	if err := c.Restart(context.Background(), "dep-1"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if visto == nil {
		t.Fatal("o servidor não recebeu requisição nenhuma")
	}
	return visto
}

const okRestart = `{"data":{"deploymentRestart":true}}`

func TestProjectTokenGoesInItsOwnHeader(t *testing.T) {
	// The platform reads the two kinds of credential from different headers, so
	// sending a project token as a bearer is simply rejected — and the panel
	// would report the game server as unreachable rather than misconfigured.
	r := capture(t, Config{ProjectToken: "pt-abc"}, okRestart)

	if got := r.Header.Get("Project-Access-Token"); got != "pt-abc" {
		t.Errorf("Project-Access-Token = %q, want pt-abc", got)
	}
	if got := r.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want vazio — não é esse o tipo do token", got)
	}
}

func TestAccountTokenIsSentAsBearer(t *testing.T) {
	r := capture(t, Config{Token: "acc-xyz"}, okRestart)

	if got := r.Header.Get("Authorization"); got != "Bearer acc-xyz" {
		t.Errorf("Authorization = %q, want Bearer acc-xyz", got)
	}
	if got := r.Header.Get("Project-Access-Token"); got != "" {
		t.Errorf("Project-Access-Token = %q, want vazio", got)
	}
}

func TestProjectTokenWinsWhenBothAreSet(t *testing.T) {
	// Two headers would leave the choice to the platform, and the wrong choice
	// here is the broad one. Prefer the credential scoped to this project.
	r := capture(t, Config{Token: "acc-xyz", ProjectToken: "pt-abc"}, okRestart)

	if got := r.Header.Get("Project-Access-Token"); got != "pt-abc" {
		t.Errorf("Project-Access-Token = %q, want pt-abc", got)
	}
	if got := r.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want vazio — o token de projeto é o mais estreito", got)
	}
}

func TestReadyNeedsOneOfTheTwoTokens(t *testing.T) {
	base := Config{ProjectID: "p", EnvironmentID: "e", ServiceID: "s"}

	if base.Ready() {
		t.Error("Ready sem token nenhum — o painel tentaria chamar a API sem credencial")
	}
	comConta := base
	comConta.Token = "acc"
	if !comConta.Ready() {
		t.Error("token de conta devia bastar")
	}
	comProjeto := base
	comProjeto.ProjectToken = "pt"
	if !comProjeto.Ready() {
		t.Error("token de projeto devia bastar")
	}
	semServico := comProjeto
	semServico.ServiceID = ""
	if semServico.Ready() {
		t.Error("Ready sem o serviço alvo — não há o que reiniciar")
	}
}

func TestGraphQLErrorsBecomeAnError(t *testing.T) {
	// The API answers 200 with an errors array as readily as it answers a status
	// code. Reading only the status would turn a refusal into a silent success,
	// and the panel would tell the operator the server had restarted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"Not Authorized"}]}`))
	}))
	defer srv.Close()

	c := New(Config{ProjectToken: "pt-abc"})
	c.endpoint = srv.URL

	err := c.Restart(context.Background(), "dep-1")
	if err == nil {
		t.Fatal("um 200 com errors passou como sucesso")
	}
	if !strings.Contains(err.Error(), "Not Authorized") {
		t.Errorf("o erro não repassa o motivo da plataforma: %v", err)
	}
}

func TestLatestReadsTheFirstDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"deployments":{"edges":[{"node":{
			"id":"dep-9","status":"SUCCESS","createdAt":"2026-09-04T23:57:27Z"}}]}}}`))
	}))
	defer srv.Close()

	c := New(Config{ProjectToken: "pt", ProjectID: "p", EnvironmentID: "e", ServiceID: "s"})
	c.endpoint = srv.URL

	d, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if d.ID != "dep-9" || d.Status != "SUCCESS" {
		t.Errorf("deployment = %+v, want dep-9 / SUCCESS", d)
	}
	// The boot time is the whole point: it is what the pending-restart count on
	// the home page is measured against.
	want := time.Date(2026, 9, 4, 23, 57, 27, 0, time.UTC)
	if !d.CreatedAt.Equal(want) {
		t.Errorf("createdAt = %v, want %v", d.CreatedAt, want)
	}
}

func TestLatestSaysSoWhenThereIsNoDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"deployments":{"edges":[]}}}`))
	}))
	defer srv.Close()

	c := New(Config{ProjectToken: "pt", ServiceID: "svc-1"})
	c.endpoint = srv.URL

	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("uma lista vazia passou como sucesso")
	}
}
