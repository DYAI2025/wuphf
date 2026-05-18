package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveProviderEndpoint_DefaultsWhenNothingConfigured exercises the
// fallback path: no env, no config file → caller's defaults are returned
// verbatim.
func TestResolveProviderEndpoint_DefaultsWhenNothingConfigured(t *testing.T) {
	withTempConfig(t, func(_ string) {
		t.Setenv("WUPHF_MLX_LM_BASE_URL", "")
		t.Setenv("WUPHF_MLX_LM_MODEL", "")

		baseURL, model := ResolveProviderEndpoint("mlx-lm",
			"http://default/v1", "default-model")
		if baseURL != "http://default/v1" {
			t.Errorf("baseURL = %q, want default", baseURL)
		}
		if model != "default-model" {
			t.Errorf("model = %q, want default", model)
		}
	})
}

// TestResolveProviderEndpoint_ConfigFileOverridesDefault exercises the
// middle layer: a config file with provider_endpoints overrides the
// caller-supplied defaults.
func TestResolveProviderEndpoint_ConfigFileOverridesDefault(t *testing.T) {
	withTempConfig(t, func(dir string) {
		t.Setenv("WUPHF_MLX_LM_BASE_URL", "")
		t.Setenv("WUPHF_MLX_LM_MODEL", "")

		cfg := Config{
			ProviderEndpoints: map[string]ProviderEndpoint{
				"mlx-lm": {BaseURL: "http://configured:9000/v1", Model: "configured-model"},
			},
		}
		writeTestConfig(t, dir, cfg)

		baseURL, model := ResolveProviderEndpoint("mlx-lm",
			"http://default/v1", "default-model")
		if baseURL != "http://configured:9000/v1" {
			t.Errorf("baseURL = %q, want configured override", baseURL)
		}
		if model != "configured-model" {
			t.Errorf("model = %q, want configured override", model)
		}
	})
}

// TestResolveProviderEndpoint_EnvOverridesConfig exercises the top of the
// resolution order: env > config file > default.
func TestResolveProviderEndpoint_EnvOverridesConfig(t *testing.T) {
	withTempConfig(t, func(dir string) {
		cfg := Config{
			ProviderEndpoints: map[string]ProviderEndpoint{
				"ollama": {BaseURL: "http://configured/v1", Model: "configured-model"},
			},
		}
		writeTestConfig(t, dir, cfg)

		t.Setenv("WUPHF_OLLAMA_BASE_URL", "http://env/v1")
		t.Setenv("WUPHF_OLLAMA_MODEL", "env-model")

		baseURL, model := ResolveProviderEndpoint("ollama",
			"http://default/v1", "default-model")
		if baseURL != "http://env/v1" {
			t.Errorf("baseURL = %q, want env override", baseURL)
		}
		if model != "env-model" {
			t.Errorf("model = %q, want env override", model)
		}
	})
}

// TestResolveProviderEndpoint_PartialOverrides verifies a partially-set
// config (only base_url) doesn't blank out the model — each field falls
// through independently.
func TestResolveProviderEndpoint_PartialOverrides(t *testing.T) {
	withTempConfig(t, func(dir string) {
		cfg := Config{
			ProviderEndpoints: map[string]ProviderEndpoint{
				"exo": {BaseURL: "http://configured/v1"}, // model intentionally empty
			},
		}
		writeTestConfig(t, dir, cfg)
		t.Setenv("WUPHF_EXO_BASE_URL", "")
		t.Setenv("WUPHF_EXO_MODEL", "")

		baseURL, model := ResolveProviderEndpoint("exo",
			"http://default/v1", "default-model")
		if baseURL != "http://configured/v1" {
			t.Errorf("baseURL = %q, want configured", baseURL)
		}
		if model != "default-model" {
			t.Errorf("model = %q, want compile-time default (config left blank)", model)
		}
	})
}

// TestResolveProviderEndpoint_KindWithDashesMapsToEnvUnderscore confirms
// that mlx-lm → WUPHF_MLX_LM_BASE_URL (not WUPHF_MLX-LM_BASE_URL, which
// most shells refuse to set).
func TestResolveProviderEndpoint_KindWithDashesMapsToEnvUnderscore(t *testing.T) {
	withTempConfig(t, func(_ string) {
		t.Setenv("WUPHF_MLX_LM_BASE_URL", "http://expected/v1")
		t.Setenv("WUPHF_MLX_LM_MODEL", "expected-model")
		baseURL, model := ResolveProviderEndpoint("mlx-lm",
			"http://default/v1", "default-model")
		if baseURL != "http://expected/v1" || model != "expected-model" {
			t.Errorf("env-via-underscore not honoured: baseURL=%q model=%q", baseURL, model)
		}
	})
}

// TestResolveProviderModelForAgent_EmptySlug verifies the empty-slug guard:
// callers without an agent context get "" and fall through to the install-
// wide model rather than picking up a stray WUPHF_<KIND>_MODEL_ variable.
func TestResolveProviderModelForAgent_EmptySlug(t *testing.T) {
	withTempConfig(t, func(_ string) {
		t.Setenv("WUPHF_OLLAMA_MODEL_", "should-not-leak")
		got := ResolveProviderModelForAgent("ollama", "")
		if got != "" {
			t.Errorf("got %q, want empty (no slug context)", got)
		}
	})
}

// TestResolveProviderModelForAgent_EnvOverridesConfig confirms the env layer
// (WUPHF_OLLAMA_MODEL_FE) wins over the config-file ModelsByAgent map for the
// same slug — matching ResolveProviderEndpoint's env>config>default order.
func TestResolveProviderModelForAgent_EnvOverridesConfig(t *testing.T) {
	withTempConfig(t, func(dir string) {
		cfg := Config{
			ProviderEndpoints: map[string]ProviderEndpoint{
				"ollama": {ModelsByAgent: map[string]string{"fe": "configured-model"}},
			},
		}
		writeTestConfig(t, dir, cfg)
		t.Setenv("WUPHF_OLLAMA_MODEL_FE", "env-model")

		got := ResolveProviderModelForAgent("ollama", "fe")
		if got != "env-model" {
			t.Errorf("got %q, want env-model", got)
		}
	})
}

// TestResolveProviderModelForAgent_ConfigUsedWhenEnvAbsent confirms the
// config-file path: env unset → ModelsByAgent[slug] is returned.
func TestResolveProviderModelForAgent_ConfigUsedWhenEnvAbsent(t *testing.T) {
	withTempConfig(t, func(dir string) {
		cfg := Config{
			ProviderEndpoints: map[string]ProviderEndpoint{
				"ollama": {ModelsByAgent: map[string]string{
					"eng":      "qwen2.5-coder:14b",
					"designer": "llama3.1:8b",
				}},
			},
		}
		writeTestConfig(t, dir, cfg)
		t.Setenv("WUPHF_OLLAMA_MODEL_ENG", "")
		t.Setenv("WUPHF_OLLAMA_MODEL_DESIGNER", "")

		if got := ResolveProviderModelForAgent("ollama", "eng"); got != "qwen2.5-coder:14b" {
			t.Errorf("eng: got %q, want qwen2.5-coder:14b", got)
		}
		if got := ResolveProviderModelForAgent("ollama", "designer"); got != "llama3.1:8b" {
			t.Errorf("designer: got %q, want llama3.1:8b", got)
		}
	})
}

// TestResolveProviderModelForAgent_UnknownSlugReturnsEmpty confirms an agent
// without an explicit binding falls through (caller uses install-wide model).
func TestResolveProviderModelForAgent_UnknownSlugReturnsEmpty(t *testing.T) {
	withTempConfig(t, func(dir string) {
		cfg := Config{
			ProviderEndpoints: map[string]ProviderEndpoint{
				"ollama": {ModelsByAgent: map[string]string{"eng": "qwen2.5-coder:14b"}},
			},
		}
		writeTestConfig(t, dir, cfg)
		t.Setenv("WUPHF_OLLAMA_MODEL_QA", "")

		if got := ResolveProviderModelForAgent("ollama", "qa"); got != "" {
			t.Errorf("qa (unbound): got %q, want empty", got)
		}
	})
}

// TestResolveProviderModelForAgent_SlugCaseNormalized confirms slug lookup is
// case-insensitive on the env-var name and on the config map key — config
// writers using "FE" or "Fe" should still hit the "fe" map entry.
func TestResolveProviderModelForAgent_SlugCaseNormalized(t *testing.T) {
	withTempConfig(t, func(dir string) {
		cfg := Config{
			ProviderEndpoints: map[string]ProviderEndpoint{
				"ollama": {ModelsByAgent: map[string]string{"fe": "qwen2.5-coder:7b"}},
			},
		}
		writeTestConfig(t, dir, cfg)
		t.Setenv("WUPHF_OLLAMA_MODEL_FE", "")

		if got := ResolveProviderModelForAgent("ollama", "FE"); got != "qwen2.5-coder:7b" {
			t.Errorf("uppercase slug: got %q, want match against lowercase config key", got)
		}
	})
}

func writeTestConfig(t *testing.T, dir string, cfg Config) {
	t.Helper()
	path := filepath.Join(dir, ".wuphf", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
