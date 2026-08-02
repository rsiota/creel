package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/ai"
)

// setAIEnv sets the given env vars for the duration of the test, restoring the
// prior values (including unsetting ones that were absent) on cleanup.
func setAIEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		prev, had := os.LookupEnv(k)
		if v == "" {
			_ = os.Unsetenv(k)
		} else {
			_ = os.Setenv(k, v)
		}
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, prev)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
}

func clearAllAIEnv(t *testing.T) {
	for _, k := range []string{"CREEL_AI_API_KEY", "GSQL_AI_API_KEY", "OPENAI_API_KEY", "ZAI_API_KEY", "CREEL_AI_BASE_URL", "GSQL_AI_BASE_URL", "CREEL_AI_MODEL", "GSQL_AI_MODEL"} {
		if prev, had := os.LookupEnv(k); had {
			_ = os.Unsetenv(k)
			t.Cleanup(func() { _ = os.Setenv(k, prev) })
		}
	}
}

func TestAIConfigFromEnv_ZAIAutoDefaults(t *testing.T) {
	clearAllAIEnv(t)
	setAIEnv(t, map[string]string{"ZAI_API_KEY": "zk-123"})

	cfg := aiConfigFromEnv()
	if cfg.APIKey != "zk-123" {
		t.Errorf("APIKey = %q, want zk-123", cfg.APIKey)
	}
	if cfg.BaseURL != "https://api.z.ai/api/coding/paas/v4" {
		t.Errorf("BaseURL = %q, want the z.ai coding endpoint", cfg.BaseURL)
	}
	if cfg.Model != "glm-4.6" {
		t.Errorf("Model = %q, want glm-4.6", cfg.Model)
	}
}

func TestAIConfigFromEnv_ZAIOverridesRespected(t *testing.T) {
	clearAllAIEnv(t)
	setAIEnv(t, map[string]string{
		"ZAI_API_KEY":      "zk-123",
		"CREEL_AI_BASE_URL": "https://my-proxy.example/v1",
		"CREEL_AI_MODEL":    "glm-5.1",
	})

	cfg := aiConfigFromEnv()
	if cfg.BaseURL != "https://my-proxy.example/v1" {
		t.Errorf("explicit base URL should win, got %q", cfg.BaseURL)
	}
	if cfg.Model != "glm-5.1" {
		t.Errorf("explicit model should win, got %q", cfg.Model)
	}
}

func TestAIConfigFromEnv_CREELKeyWins(t *testing.T) {
	clearAllAIEnv(t)
	setAIEnv(t, map[string]string{
		"CREEL_AI_API_KEY": "gk-1",
		"OPENAI_API_KEY":  "ok-2",
		"ZAI_API_KEY":     "zk-3",
	})

	cfg := aiConfigFromEnv()
	if cfg.APIKey != "gk-1" {
		t.Errorf("CREEL_AI_API_KEY should win, got %q", cfg.APIKey)
	}
	// No z.ai key as the source, so the z.ai coding defaults must NOT apply.
	if cfg.BaseURL == "https://api.z.ai/api/coding/paas/v4" {
		t.Errorf("z.ai defaults leaked though key source is CREEL_AI_API_KEY")
	}
	if cfg.Model == "glm-4.6" {
		t.Errorf("z.ai model leaked though key source is CREEL_AI_API_KEY")
	}
}

func TestAIConfigFromEnv_NoKey(t *testing.T) {
	clearAllAIEnv(t)
	cfg := aiConfigFromEnv()
	if cfg.APIKey != "" {
		t.Errorf("expected empty key, got %q", cfg.APIKey)
	}
}

func TestAIConfigFromEnv_DeprecatedGSQLKey(t *testing.T) {
	clearAllAIEnv(t)
	setAIEnv(t, map[string]string{"GSQL_AI_API_KEY": "gk-dep"})

	cfg := aiConfigFromEnv()
	if cfg.APIKey != "gk-dep" {
		t.Errorf("GSQL_AI_API_KEY should be honoured as a deprecated fallback, got %q", cfg.APIKey)
	}
	// The deprecated key is the source, so the z.ai coding defaults must NOT apply.
	if cfg.BaseURL == "https://api.z.ai/api/coding/paas/v4" {
		t.Errorf("z.ai defaults leaked though key source is GSQL_AI_API_KEY")
	}
}

func TestAIConfigFromEnv_CREELKeyBeatsDeprecatedGSQL(t *testing.T) {
	clearAllAIEnv(t)
	setAIEnv(t, map[string]string{
		"CREEL_AI_API_KEY": "gk-new",
		"GSQL_AI_API_KEY":  "gk-old",
	})

	cfg := aiConfigFromEnv()
	if cfg.APIKey != "gk-new" {
		t.Errorf("CREEL_AI_API_KEY should beat the deprecated alias, got %q", cfg.APIKey)
	}
}

func TestAIConfigFromEnv_DeprecatedGSQLBaseURLAndModel(t *testing.T) {
	clearAllAIEnv(t)
	setAIEnv(t, map[string]string{
		"GSQL_AI_API_KEY":  "gk-dep",
		"GSQL_AI_BASE_URL": "https://legacy.example/v1",
		"GSQL_AI_MODEL":    "legacy-model",
	})

	cfg := aiConfigFromEnv()
	if cfg.BaseURL != "https://legacy.example/v1" {
		t.Errorf("GSQL_AI_BASE_URL should fall back, got %q", cfg.BaseURL)
	}
	if cfg.Model != "legacy-model" {
		t.Errorf("GSQL_AI_MODEL should fall back, got %q", cfg.Model)
	}
}

func TestAIAuthHint(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"401", errors.New("ai: provider returned 401: token expired or incorrect"), "CREEL_AI_BASE_URL"},
		{"unauthorized", errors.New("unauthorized"), "CREEL_AI_BASE_URL"},
		{"other", errors.New("network timeout"), ""},
		{"nil", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := aiAuthHint(c.err)
			if c.want == "" {
				if got != "" {
					t.Errorf("expected no hint, got %q", got)
				}
				return
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("hint %q missing %q", got, c.want)
			}
		})
	}
}

// Compile-time check that we reference the ai package (keeps the import honest
// even if the table above were removed).
var _ = ai.DefaultModel
