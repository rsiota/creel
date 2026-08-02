package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruben/creel/internal/config"
	"github.com/ruben/creel/internal/secrets"
)

// TestSecretConfigRoundTrip verifies the full save/load cycle: plaintext
// passwords are migrated to keychain references on save, the YAML file never
// contains plaintext, and Resolve recovers the plaintext at "connect" time.
func TestSecretConfigRoundTrip(t *testing.T) {
	if !secrets.Available() {
		t.Skip("OS keychain not available")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	const connName = "creel-roundtrip"
	t.Cleanup(func() { _ = secrets.DeleteAll(connName) })

	// Build a config as the form would (plaintext passwords).
	cfg, _ := config.Load()
	cc := config.ConnectionConfig{
		Name: connName, Driver: "postgres", Database: "analytics",
		Host: "db.internal", Port: 5432, Username: "readonly",
		Password: "supersecret", SSHPassword: "sshpw",
	}
	stored, err := storeConnSecrets(cc, "keychain")
	if err != nil {
		t.Fatalf("storeConnSecrets: %v", err)
	}
	cfg.AddConnection(stored)
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The on-disk YAML must not leak plaintext.
	data, err := os.ReadFile(filepath.Join(dir, "creel", "config.yaml"))
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	if strings.Contains(string(data), "supersecret") {
		t.Errorf("plaintext password leaked into YAML:\n%s", data)
	}
	if strings.Contains(string(data), "sshpw") {
		t.Errorf("plaintext ssh password leaked into YAML:\n%s", data)
	}
	if !strings.Contains(string(data), "secret://") {
		t.Errorf("expected a secret:// reference in YAML:\n%s", data)
	}

	// Reload from disk (as at startup) and resolve references (as connectToDB
	// does). The plaintext must round-trip exactly.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := loaded.GetConnection(connName)
	if got == nil {
		t.Fatalf("connection %q missing after reload", connName)
	}
	if got.Password != stored.Password {
		t.Errorf("password not persisted as reference: got %q", got.Password)
	}
	pw, err := secrets.Resolve(got.Password)
	if err != nil {
		t.Fatalf("Resolve password: %v", err)
	}
	if pw != "supersecret" {
		t.Errorf("resolved password = %q, want supersecret", pw)
	}
}
