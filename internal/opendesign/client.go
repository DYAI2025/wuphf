// Package opendesign is a thin HTTP client for the open-design daemon
// (https://github.com/DYAI2025/open-design). open-design is a local
// design-app daemon that exposes a REST surface for projects, skills,
// design templates, and live artifacts. wuphf agents reach it through
// internal/teammcp/open_design_tools.go so the Designer / CMO / CEO
// roles can delegate hard design work (decks, posters, full pages) to
// the open-design stack while wuphf's own runtime stays local-only.
//
// The daemon listens on 127.0.0.1 by default; we never reach out over
// the public network. If the daemon isn't running, every tool call
// returns a clean, single-sentence error message instead of a 60s
// timeout — the agent then either retries after the human starts the
// daemon, or falls back to its local model.
package opendesign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultBaseURL matches open-design's documented local listener.
	// Override with WUPHF_OPEN_DESIGN_URL when running the daemon on
	// a non-default port or remote loopback proxy.
	DefaultBaseURL = "http://127.0.0.1:7878"

	defaultTimeout = 30 * time.Second
)

// ErrDaemonUnreachable is returned when the daemon's /api/health endpoint
// can't be reached. Callers (MCP handlers) translate this to a one-line
// error message the LLM can act on ("start the daemon") rather than
// surfacing a raw net/http error.
var ErrDaemonUnreachable = errors.New("open-design daemon unreachable")

// Client is a small synchronous HTTP wrapper. It's safe for concurrent
// use because *http.Client is.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New constructs a Client using env-or-default base URL. Pass an
// explicit baseURL to override per-call.
func New(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = strings.TrimSpace(os.Getenv("WUPHF_OPEN_DESIGN_URL"))
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: defaultTimeout},
	}
}

// Skill is the minimal projection of an open-design skill record. The
// daemon returns more fields; we expose only the ones an LLM needs to
// pick a skill and explain its choice to the user.
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Scenario    string `json:"scenario,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

// DesignTemplate is the minimal projection of a design-template record
// returned by /api/design-templates. Same rationale as Skill: minimal
// surface, no leaking daemon internals into the MCP schema.
type DesignTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// LiveArtifact is the projection of a created/listed live-artifact.
type LiveArtifact struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId,omitempty"`
	Title      string `json:"title,omitempty"`
	Kind       string `json:"kind,omitempty"`
	PreviewURL string `json:"previewUrl,omitempty"`
	Status     string `json:"status,omitempty"`
}

// CreateArtifactInput is what wuphf sends to /api/tools/live-artifacts/create.
// The shape mirrors the daemon's request body; we keep optional fields
// pointer-free so JSON omitempty hides them when the agent leaves them
// blank.
type CreateArtifactInput struct {
	ProjectID  string `json:"projectId,omitempty"`
	SkillID    string `json:"skillId,omitempty"`
	TemplateID string `json:"templateId,omitempty"`
	Title      string `json:"title,omitempty"`
	Prompt     string `json:"prompt"`
}

// Health probes the daemon. Returns ErrDaemonUnreachable wrapped with
// the underlying transport error when the daemon isn't reachable.
func (c *Client) Health(ctx context.Context) error {
	u := c.BaseURL + "/api/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrDaemonUnreachable, resp.StatusCode)
	}
	return nil
}

// ListSkills calls /api/skills. Optional scenario filter is forwarded
// as a query parameter so the daemon can do the filtering server-side.
func (c *Client) ListSkills(ctx context.Context, scenario string) ([]Skill, error) {
	q := url.Values{}
	if strings.TrimSpace(scenario) != "" {
		q.Set("scenario", scenario)
	}
	var body struct {
		Skills []Skill `json:"skills"`
	}
	if err := c.getJSON(ctx, "/api/skills", q, &body); err != nil {
		return nil, err
	}
	return body.Skills, nil
}

// ListTemplates calls /api/design-templates.
func (c *Client) ListTemplates(ctx context.Context) ([]DesignTemplate, error) {
	var body struct {
		Templates []DesignTemplate `json:"templates"`
	}
	if err := c.getJSON(ctx, "/api/design-templates", nil, &body); err != nil {
		return nil, err
	}
	return body.Templates, nil
}

// CreateArtifact calls /api/tools/live-artifacts/create.
func (c *Client) CreateArtifact(ctx context.Context, in CreateArtifactInput) (LiveArtifact, error) {
	var out LiveArtifact
	if err := c.postJSON(ctx, "/api/tools/live-artifacts/create", in, &out); err != nil {
		return LiveArtifact{}, err
	}
	return out, nil
}

// ListArtifacts calls /api/tools/live-artifacts/list.
func (c *Client) ListArtifacts(ctx context.Context) ([]LiveArtifact, error) {
	var body struct {
		Artifacts []LiveArtifact `json:"artifacts"`
	}
	if err := c.getJSON(ctx, "/api/tools/live-artifacts/list", nil, &body); err != nil {
		return nil, err
	}
	return body.Artifacts, nil
}

// RefreshArtifact calls /api/tools/live-artifacts/refresh. Refresh is
// the daemon's term for "re-run the skill on the same prompt"; useful
// when the human asks for a variation.
func (c *Client) RefreshArtifact(ctx context.Context, id string) (LiveArtifact, error) {
	var out LiveArtifact
	body := map[string]string{"artifactId": id}
	if err := c.postJSON(ctx, "/api/tools/live-artifacts/refresh", body, &out); err != nil {
		return LiveArtifact{}, err
	}
	return out, nil
}

// PreviewURL composes the daemon-rooted preview URL for an artifact so
// the agent can drop it into a markdown link. No request is made — the
// URL is the daemon's documented preview route.
func (c *Client) PreviewURL(id string) string {
	return c.BaseURL + "/api/live-artifacts/" + url.PathEscape(id) + "/preview"
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) postJSON(ctx context.Context, path string, in, out any) error {
	buf, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("open-design %s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
