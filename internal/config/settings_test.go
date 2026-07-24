package config

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Effective fills zero values with the defaults.
func TestSettingsEffectiveDefaults(t *testing.T) {
	got := Settings{}.Effective()
	if got.PageSize != DefaultPageSize {
		t.Errorf("PageSize=%d, want %d", got.PageSize, DefaultPageSize)
	}
	if got.QueryTimeout.Std() != DefaultQueryTimeout {
		t.Errorf("QueryTimeout=%v, want %v", got.QueryTimeout.Std(), DefaultQueryTimeout)
	}
	if got.DefaultDriver != DefaultDriver {
		t.Errorf("DefaultDriver=%q, want %q", got.DefaultDriver, DefaultDriver)
	}
}

// Explicit values are preserved by Effective.
func TestSettingsEffectivePreservesExplicit(t *testing.T) {
	in := Settings{PageSize: 50, QueryTimeout: Duration(2 * time.Minute), DefaultDriver: "mysql"}
	got := in.Effective()
	if got.PageSize != 50 || got.QueryTimeout.Std() != 2*time.Minute || got.DefaultDriver != "mysql" {
		t.Errorf("Effective changed explicit values: %+v", got)
	}
}

// A config with no settings block loads as zero and Effective defaults apply.
func TestLoadNoSettingsBlock(t *testing.T) {
	yml := "connections:\n  - name: local\n    driver: sqlite\n    database: /tmp/a.db\n"
	var cfg Config
	if err := yamlUnmarshal(t, yml, &cfg); err != nil {
		t.Fatal(err)
	}
	eff := cfg.Settings.Effective()
	if eff.PageSize != DefaultPageSize || eff.DefaultDriver != DefaultDriver {
		t.Errorf("defaults not applied: %+v", eff)
	}
}

// The friendly "30s" duration form parses correctly from YAML.
func TestDurationUnmarshalFriendly(t *testing.T) {
	cases := map[string]time.Duration{
		"30s":   30 * time.Second,
		"2m":    2 * time.Minute,
		"1h":    time.Hour,
		"1h30m": 90 * time.Minute,
	}
	for in, want := range cases {
		var cfg Config
		yml := "settings:\n  query_timeout: " + in + "\n"
		if err := yamlUnmarshal(t, yml, &cfg); err != nil {
			t.Errorf("parse %q: %v", in, err)
			continue
		}
		if got := cfg.Settings.QueryTimeout.Std(); got != want {
			t.Errorf("parse %q: got %v, want %v", in, got, want)
		}
	}
}

// A bare number is read as seconds.
func TestDurationUnmarshalBareNumber(t *testing.T) {
	for _, in := range []string{"45", " 45 "} {
		var cfg Config
		yml := "settings:\n  query_timeout: " + strings.TrimSpace(in) + "\n"
		if err := yamlUnmarshal(t, yml, &cfg); err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		if got := cfg.Settings.QueryTimeout.Std(); got != 45*time.Second {
			t.Errorf("got %v, want 45s", got)
		}
	}
}

// An invalid duration string errors rather than silently falling back.
func TestDurationUnmarshalInvalid(t *testing.T) {
	var cfg Config
	yml := "settings:\n  query_timeout: abc\n"
	if err := yamlUnmarshal(t, yml, &cfg); err == nil {
		t.Error("expected an error for invalid duration")
	}
}

// The "off"/"none" sentinels (and a negative number) disable the query
// timeout: they parse to a negative Duration that Effective leaves intact so
// the runner applies no deadline.
func TestQueryTimeoutDisabledSentinel(t *testing.T) {
	for _, in := range []string{"off", "none", "disable", "OFF", "-1", "-5s"} {
		var cfg Config
		yml := "settings:\n  query_timeout: " + in + "\n"
		if err := yamlUnmarshal(t, yml, &cfg); err != nil {
			t.Errorf("parse %q: %v", in, err)
			continue
		}
		eff := cfg.Settings.Effective()
		if eff.QueryTimeout >= 0 {
			t.Errorf("%q: effective timeout = %v, want negative (disabled)", in, eff.QueryTimeout)
		}
	}
}

// An unset (zero) query_timeout still falls back to the default, distinct from
// the explicit disable sentinel.
func TestQueryTimeoutZeroIsDefault(t *testing.T) {
	var cfg Config
	yml := "settings:\n  query_timeout: 0\n"
	if err := yamlUnmarshal(t, yml, &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Settings.Effective().QueryTimeout.Std(); got != DefaultQueryTimeout {
		t.Errorf("zero should fall back to default, got %v", got)
	}
}

// The disabled sentinel round-trips through marshal as "off".
func TestQueryTimeoutDisabledRoundTrip(t *testing.T) {
	out, err := yamlMarshal(t, Config{Settings: Settings{QueryTimeout: Duration(-1)}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "off") || strings.Contains(out, "-1") {
		t.Errorf("disabled should marshal as 'off', got:\n%s", out)
	}
	var back Config
	if err := yamlUnmarshal(t, out, &back); err != nil {
		t.Fatal(err)
	}
	if back.Settings.QueryTimeout >= 0 {
		t.Errorf("round-trip lost disabled sentinel: %v", back.Settings.QueryTimeout)
	}
}

// Duration round-trips through marshal/unmarshal in the friendly form.
func TestDurationRoundTrip(t *testing.T) {
	cfg := Config{Settings: Settings{QueryTimeout: Duration(30 * time.Second)}}
	out, err := yamlMarshal(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "30s") {
		t.Errorf("marshaled output should contain 30s:\n%s", out)
	}
	var back Config
	if err := yamlUnmarshal(t, out, &back); err != nil {
		t.Fatal(err)
	}
	if back.Settings.QueryTimeout.Std() != 30*time.Second {
		t.Errorf("round-trip lost value: %v", back.Settings.QueryTimeout.Std())
	}
}

// A config with explicit settings round-trips through YAML.
func TestSettingsRoundTrip(t *testing.T) {
	cfg := Config{
		Settings: Settings{PageSize: 50, QueryTimeout: Duration(time.Minute), DefaultDriver: "postgres"},
	}
	out, err := yamlMarshal(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back Config
	if err := yamlUnmarshal(t, out, &back); err != nil {
		t.Fatal(err)
	}
	eff := back.Settings.Effective()
	if eff.PageSize != 50 || eff.QueryTimeout.Std() != time.Minute || eff.DefaultDriver != "postgres" {
		t.Errorf("round-trip mismatch: %+v", eff)
	}
}

// A zero Settings struct marshals without emitting a settings block, so adding
// a connection doesn't sprout a settings: block the user never wrote.
func TestZeroSettingsOmittedOnMarshal(t *testing.T) {
	out, err := yamlMarshal(t, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "settings:") {
		t.Errorf("zero Settings should be omitted, got:\n%s", out)
	}
}

// --- yaml helpers (kept local so the test file is self-contained) ----------

func yamlUnmarshal(t *testing.T, yml string, out interface{}) error {
	t.Helper()
	return yaml.Unmarshal([]byte(yml), out)
}

func yamlMarshal(t *testing.T, in interface{}) (string, error) {
	t.Helper()
	b, err := yaml.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
