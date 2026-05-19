package team

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nex-crm/wuphf/internal/config"
)

// TestConfigEndpointAndHealth is a smoke test for ISSUE-004: the wizard's
// POST /config must persist llm_provider and /health must reflect it.
//
// Local-only contract: the wizard can switch between local kinds (ollama,
// mlx-lm, exo). Hosted-LLM kinds (claude-code, codex, opencode) are
// refused at the persist boundary and return 400.
func TestConfigEndpointAndHealth(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("WUPHF_RUNTIME_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".wuphf"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Seed config with ollama (the install-wide local default).
	initial := `{"llm_provider":"ollama"}`
	if err := os.WriteFile(filepath.Join(tmp, ".wuphf", "config.json"), []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	b := newTestBroker(t)
	b.runtimeProvider = "ollama"
	b.token = "test-token"
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("start broker: %v", err)
	}
	defer func() {
		if b.server != nil {
			_ = b.server.Shutdown(context.Background())
		}
	}()

	// /health before — should be ollama (the launcher-seeded default)
	healthURL := "http://" + b.addr + "/health"
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	var h1 map[string]any
	raw1, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(raw1, &h1)
	t.Logf("GET /health (initial) -> %s", string(raw1))
	if p, _ := h1["provider"].(string); p != "ollama" {
		t.Fatalf("expected provider=ollama before POST, got %q", p)
	}
	if backend, _ := h1["memory_backend"].(string); backend != config.MemoryBackendMarkdown {
		t.Fatalf("expected memory_backend=%q before POST, got %q", config.MemoryBackendMarkdown, backend)
	}

	// POST /config with mlx-lm — simulates the wizard switching to
	// another local kind. Must succeed and persist.
	body := bytes.NewBufferString(`{"llm_provider":"mlx-lm"}`)
	req, _ := http.NewRequest(http.MethodPost, "http://"+b.addr+"/config", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /config: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("POST /config {llm_provider:mlx-lm} -> %d %s", resp.StatusCode, string(raw))
	if resp.StatusCode != 200 {
		t.Fatalf("POST /config status=%d body=%s", resp.StatusCode, string(raw))
	}

	// /health after — should reflect the switch
	resp, err = http.Get(healthURL)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	var h2 map[string]any
	raw2, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(raw2, &h2)
	t.Logf("GET /health (after POST) -> %s", string(raw2))
	if p, _ := h2["provider"].(string); p != "mlx-lm" {
		t.Fatalf("expected provider=mlx-lm after POST, got %q", p)
	}

	req, _ = http.NewRequest(http.MethodGet, "http://"+b.addr+"/config", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /config: %v", err)
	}
	rawConfig, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var cfgResp map[string]any
	_ = json.Unmarshal(rawConfig, &cfgResp)
	if p, _ := cfgResp["llm_provider"].(string); p != "mlx-lm" {
		t.Fatalf("expected /config llm_provider=mlx-lm after POST, got %q (body=%s)", p, string(rawConfig))
	}
	if backend, _ := cfgResp["memory_backend"].(string); backend != config.MemoryBackendMarkdown {
		t.Fatalf("expected /config memory_backend=%q, got %q", config.MemoryBackendMarkdown, backend)
	}

	// Verify persisted to disk
	disk, _ := os.ReadFile(filepath.Join(tmp, ".wuphf", "config.json"))
	if !strings.Contains(string(disk), `"llm_provider": "mlx-lm"`) {
		t.Fatalf("config.json missing mlx-lm: %s", string(disk))
	}

	// Cloud kinds (claude-code, codex, opencode) must be rejected with
	// 400 — the wizard tile UI is the user-facing surface for the
	// local-only contract, so a saved cloud kind would mean the build
	// fell through to a hosted LLM.
	for _, blocked := range []string{"claude-code", "codex", "opencode"} {
		body = bytes.NewBufferString(`{"llm_provider":"` + blocked + `"}`)
		req, _ = http.NewRequest(http.MethodPost, "http://"+b.addr+"/config", body)
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /config {%s}: %v", blocked, err)
		}
		raw, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST /config {llm_provider:%s} expected 400, got %d body=%s", blocked, resp.StatusCode, string(raw))
		}
	}

	// Reject unsupported provider (anthropic was never a runnable Kind).
	body = bytes.NewBufferString(`{"llm_provider":"anthropic"}`)
	req, _ = http.NewRequest(http.MethodPost, "http://"+b.addr+"/config", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /config reject: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported provider, got %d", resp.StatusCode)
	}
}

// TestConfigEndpointAcceptsActionProviders verifies the web UI POST /config
// validator accepts every action_provider string the registry supports.
// Regression test for the case where "one" was rejected with 400 even though
// the registry and CLI (`/config set action_provider one`) both accepted it.
func TestConfigEndpointAcceptsActionProviders(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".wuphf"), 0o700); err != nil {
		t.Fatal(err)
	}

	b := newTestBroker(t)
	b.runtimeProvider = "claude-code"
	b.token = "test-token"
	if err := b.StartOnPort(0); err != nil {
		t.Fatalf("start broker: %v", err)
	}
	defer func() {
		if b.server != nil {
			_ = b.server.Shutdown(context.Background())
		}
	}()

	cases := []struct {
		name     string
		value    string
		wantCode int
	}{
		{"auto", "auto", http.StatusOK},
		{"one", "one", http.StatusOK},
		{"composio", "composio", http.StatusOK},
		{"empty", "", http.StatusOK},
		{"rejects unknown", "perplexity", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"action_provider":"` + tc.value + `"}`
			req, _ := http.NewRequest(http.MethodPost, "http://"+b.addr+"/config", bytes.NewBufferString(payload))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST /config: %v", err)
			}
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != tc.wantCode {
				t.Fatalf("POST %s: got status=%d body=%s, want %d", payload, resp.StatusCode, string(raw), tc.wantCode)
			}
			if tc.wantCode == http.StatusOK && tc.value != "" {
				// Verify persisted and echoed back on GET.
				getReq, _ := http.NewRequest(http.MethodGet, "http://"+b.addr+"/config", nil)
				getReq.Header.Set("Authorization", "Bearer test-token")
				getResp, err := http.DefaultClient.Do(getReq)
				if err != nil {
					t.Fatalf("GET /config: %v", err)
				}
				rawGet, _ := io.ReadAll(getResp.Body)
				getResp.Body.Close()
				var cfgResp map[string]any
				_ = json.Unmarshal(rawGet, &cfgResp)
				if ap, _ := cfgResp["action_provider"].(string); ap != tc.value {
					t.Fatalf("expected action_provider=%q on GET, got %q (body=%s)", tc.value, ap, string(rawGet))
				}
			}
		})
	}
}
