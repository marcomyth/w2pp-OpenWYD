// Package plataforma talks to the hosting platform's API, so the panel can
// report when the game server last started and restart it.
//
// Why the panel needs this at all: mob/NPC template stat overrides are applied
// ONCE at boot — there is no hot reload, matching the legacy EDITAPPMOB, which
// also required a server restart. So a moderator edits a monster and nothing
// happens until somebody restarts, and "somebody restarts" is the step that gets
// forgotten. Knowing the boot time turns that into a visible pending count.
//
// NPC definitions, shops and item prices are NOT in that boat: the tmServer
// polls their version every ~15s and applies them live. Only what is genuinely
// boot-bound should ever be counted as pending, or the warning becomes noise
// people learn to ignore.
//
// Everything here is optional. With no token or service id configured the panel
// hides the restart card and works exactly as before.
package plataforma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// defaultEndpoint is the platform's GraphQL API. A client carries its own copy
// so a test can point one at a local server and assert on what was sent — the
// choice of auth header below is exactly the kind of thing that breaks silently.
const defaultEndpoint = "https://backboard.railway.com/graphql/v2"

// requestTimeout bounds a call. The panel renders a page around this, so a slow
// platform must degrade to "unknown" quickly rather than hang the request.
const requestTimeout = 10 * time.Second

// Config is what the caller must supply. Project and environment ids are
// injected into every service by the platform, so in practice only the token and
// the target service id have to be set by hand.
// The platform issues two kinds of credential and they travel in different
// headers, so which one is set decides how the request is signed.
//
// Prefer ProjectToken. It reaches this project only, while an account token
// reaches every project the owner has — and this one lives in the environment of
// a service that is published on the internet. The narrower credential costs
// nothing here: the panel only ever reads and restarts this project.
type Config struct {
	Token         string // account or workspace token: Authorization: Bearer
	ProjectToken  string // project token: Project-Access-Token
	ProjectID     string
	EnvironmentID string
	ServiceID     string // the service to report on and restart (the game server)
}

// Ready reports whether enough is configured to call the API.
func (c Config) Ready() bool {
	return (c.Token != "" || c.ProjectToken != "") &&
		c.ProjectID != "" && c.EnvironmentID != "" && c.ServiceID != ""
}

// Deployment is the running deployment of the watched service.
type Deployment struct {
	ID        string
	Status    string
	CreatedAt time.Time
}

// Client calls the platform API.
type Client struct {
	cfg      Config
	endpoint string
	http     *http.Client
}

// New builds a client. Callers should check cfg.Ready() first.
func New(cfg Config) *Client {
	return &Client{cfg: cfg, endpoint: defaultEndpoint, http: &http.Client{Timeout: requestTimeout}}
}

// Latest returns the service's current successful deployment.
func (c *Client) Latest(ctx context.Context) (Deployment, error) {
	const q = `query($input: DeploymentListInput!) {
	  deployments(input: $input, first: 1) {
	    edges { node { id status createdAt } }
	  }
	}`
	vars := map[string]any{
		"input": map[string]any{
			"projectId":     c.cfg.ProjectID,
			"environmentId": c.cfg.EnvironmentID,
			"serviceId":     c.cfg.ServiceID,
			"status":        map[string]any{"successfulOnly": true},
		},
	}

	var out struct {
		Deployments struct {
			Edges []struct {
				Node struct {
					ID        string    `json:"id"`
					Status    string    `json:"status"`
					CreatedAt time.Time `json:"createdAt"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"deployments"`
	}
	if err := c.do(ctx, q, vars, &out); err != nil {
		return Deployment{}, err
	}
	if len(out.Deployments.Edges) == 0 {
		return Deployment{}, fmt.Errorf("plataforma: no successful deployment for service %s", c.cfg.ServiceID)
	}
	n := out.Deployments.Edges[0].Node
	return Deployment{ID: n.ID, Status: n.Status, CreatedAt: n.CreatedAt}, nil
}

// Restart restarts a deployment in place.
//
// Restart rather than redeploy on purpose: the image is already built and
// nothing in the repository changed, so rebuilding would take minutes and could
// pick up a commit the operator did not mean to ship. Restart reuses exactly the
// artifact that is running.
func (c *Client) Restart(ctx context.Context, deploymentID string) error {
	const m = `mutation($id: String!) { deploymentRestart(id: $id) }`
	return c.do(ctx, m, map[string]any{"id": deploymentID}, &struct{}{})
}

// do executes one GraphQL request and unmarshals data into out.
func (c *Client) do(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("plataforma: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plataforma: build request: %w", err)
	}
	// A project token wins when both are set: it is the narrower of the two, and
	// sending both headers would leave which one the platform honours up to it.
	if c.cfg.ProjectToken != "" {
		req.Header.Set("Project-Access-Token", c.cfg.ProjectToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("plataforma: call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// GraphQL answers 200 with an errors array as often as it answers a status
	// code, so both have to be checked or a failure reads as an empty success.
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("plataforma: decode response (http %d): %w", resp.StatusCode, err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("plataforma: api error: %s", env.Errors[0].Message)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plataforma: http %d", resp.StatusCode)
	}
	if len(env.Data) == 0 {
		return fmt.Errorf("plataforma: empty response (http %d)", resp.StatusCode)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("plataforma: decode data: %w", err)
	}
	return nil
}

// LatestAny returns the service's most recent deployment whatever its state.
//
// Latest filters to successful ones, which is right for "how long has it been
// up" and wrong for turning the server back on: a stopped deployment is the one
// that has to be found, and filtering by success could hide it and leave the
// panel with no id to redeploy — the server off and no button to fix it.
func (c *Client) LatestAny(ctx context.Context) (Deployment, error) {
	const q = `query($input: DeploymentListInput!) {
	  deployments(input: $input, first: 1) {
	    edges { node { id status createdAt } }
	  }
	}`
	vars := map[string]any{
		"input": map[string]any{
			"projectId":     c.cfg.ProjectID,
			"environmentId": c.cfg.EnvironmentID,
			"serviceId":     c.cfg.ServiceID,
		},
	}

	var out struct {
		Deployments struct {
			Edges []struct {
				Node struct {
					ID        string    `json:"id"`
					Status    string    `json:"status"`
					CreatedAt time.Time `json:"createdAt"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"deployments"`
	}
	if err := c.do(ctx, q, vars, &out); err != nil {
		return Deployment{}, err
	}
	if len(out.Deployments.Edges) == 0 {
		return Deployment{}, fmt.Errorf("plataforma: no deployment at all for service %s", c.cfg.ServiceID)
	}
	n := out.Deployments.Edges[0].Node
	return Deployment{ID: n.ID, Status: n.Status, CreatedAt: n.CreatedAt}, nil
}

// NoAr reports whether a deployment status means the service is running.
//
// The platform has more states than "up" and "down" — building, deploying,
// crashed, removed — and only one of them means players can connect. Anything
// else is offered to the operator as "turn it on", which is safe to press when
// it is already on.
func NoAr(status string) bool { return status == "SUCCESS" }

// Stop takes the deployment down without deleting the service.
//
// The service, its variables and its volume all survive: this is the "off"
// switch the dashboard does not offer, where the only button there is Delete.
// Redeploy brings the same deployment back.
func (c *Client) Stop(ctx context.Context, deploymentID string) error {
	const m = `mutation($id: String!) { deploymentStop(id: $id) }`
	return c.do(ctx, m, map[string]any{"id": deploymentID}, &struct{}{})
}

// Redeploy brings a stopped deployment back up.
//
// usePreviousImageTag reuses the image that was already built: turning the
// server back on must not become a rebuild, which would take minutes and could
// pick up a commit nobody meant to ship.
func (c *Client) Redeploy(ctx context.Context, deploymentID string) error {
	const m = `mutation($id: String!) {
	  deploymentRedeploy(id: $id, usePreviousImageTag: true) { id }
	}`
	return c.do(ctx, m, map[string]any{"id": deploymentID}, &struct{}{})
}
