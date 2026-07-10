package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/ruben/gsql/internal/config"
)

func TestConnectionFormValidation(t *testing.T) {
	f := NewConnectionForm()

	// No name should error
	_, errMsg := f.EnterPressed()
	if errMsg == "" {
		t.Error("expected error for empty name")
	}

	// Set name but invalid driver
	f.fields[fieldName].SetValue("test")
	f.fields[fieldDriver].SetValue("mongodb")
	_, errMsg = f.EnterPressed()
	if errMsg == "" {
		t.Error("expected error for invalid driver")
	}

	// Valid postgres connection
	f.fields[fieldDriver].SetValue("postgres")
	f.fields[fieldDatabase].SetValue("myapp")
	f.fields[fieldHost].SetValue("localhost")
	cfg, errMsg := f.EnterPressed()
	if errMsg != "" {
		t.Fatalf("expected no error for postgres, got: %s", errMsg)
	}
	if cfg.Driver != "postgres" {
		t.Errorf("expected driver 'postgres', got '%s'", cfg.Driver)
	}

	// Valid sqlite connection
	f.fields[fieldDriver].SetValue("sqlite")
	f.fields[fieldDatabase].SetValue("/tmp/test.db")
	cfg, errMsg = f.EnterPressed()
	if errMsg != "" {
		t.Fatalf("expected no error, got: %s", errMsg)
	}
	if cfg.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", cfg.Name)
	}
	if cfg.Driver != "sqlite" {
		t.Errorf("expected driver 'sqlite', got '%s'", cfg.Driver)
	}
}

func TestConnectionFormEditPreFill(t *testing.T) {
	original := config.ConnectionConfig{
		Name:     "staging",
		Driver:   "mysql",
		Database: "myapp",
		Host:     "10.0.0.5",
		Port:     3306,
		Username: "admin",
		Password: "secret",
	}

	f := NewConnectionFormEdit(original)

	if f.mode != formModeEdit {
		t.Error("expected formModeEdit")
	}
	if f.editing != "staging" {
		t.Errorf("expected editing 'staging', got '%s'", f.editing)
	}
	if f.fields[fieldName].Value() != "staging" {
		t.Errorf("expected name 'staging', got '%s'", f.fields[fieldName].Value())
	}
	if f.fields[fieldHost].Value() != "10.0.0.5" {
		t.Errorf("expected host '10.0.0.5', got '%s'", f.fields[fieldHost].Value())
	}
}

func TestConnectionFormPortValidation(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldName].SetValue("test")
	f.fields[fieldDriver].SetValue("mysql")
	f.fields[fieldDatabase].SetValue("mydb")
	f.fields[fieldHost].SetValue("localhost")
	f.fields[fieldPort].SetValue("notanumber")

	_, errMsg := f.EnterPressed()
	if errMsg == "" {
		t.Error("expected error for invalid port")
	}

	f.fields[fieldPort].SetValue("3307")
	cfg, errMsg := f.EnterPressed()
	if errMsg != "" {
		t.Fatalf("expected no error, got: %s", errMsg)
	}
	if cfg.Port != 3307 {
		t.Errorf("expected port 3307, got %d", cfg.Port)
	}
}

func TestConnectionFormMySQLDatabaseOptional(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldName].SetValue("local")
	f.fields[fieldDriver].SetValue("mysql")
	f.fields[fieldDatabase].SetValue("") // empty — should be allowed for MySQL
	f.fields[fieldHost].SetValue("localhost")

	cfg, errMsg := f.EnterPressed()
	if errMsg != "" {
		t.Fatalf("expected no error for MySQL with empty database, got: %s", errMsg)
	}
	if cfg.Database != "" {
		t.Errorf("expected empty database, got '%s'", cfg.Database)
	}
}

func TestConnectionFormSQLiteDatabaseRequired(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldName].SetValue("local")
	f.fields[fieldDriver].SetValue("sqlite")
	f.fields[fieldDatabase].SetValue("") // empty — should error for SQLite

	_, errMsg := f.EnterPressed()
	if errMsg == "" {
		t.Fatal("expected error for SQLite with empty database")
	}
}

// Ensure textinput.Model compiles with our field setup.
func TestConnectionFormFieldCount(t *testing.T) {
	f := NewConnectionForm()
	if len(f.fields) != fieldCount {
		t.Errorf("expected %d fields, got %d", fieldCount, len(f.fields))
	}
	for i, field := range f.fields {
		if field.EchoMode != textinput.EchoNormal && i != fieldPass && i != fieldSSHPassword {
			t.Errorf("field %d should use normal echo", i)
		}
	}
}

func TestSecretsModeNormalization(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "keychain"},          // blank defaults to keychain
		{"keychain", "keychain"},
		{"Keychain", "keychain"}, // case-insensitive
		{"plain", "plain"},
		{"plain text", "plain"},  // leading substring still plain
		{"typo", "plain"},        // unknown falls back to plain (safe)
		{"  keychain  ", "keychain"},
	}
	for _, c := range cases {
		f := NewConnectionForm()
		f.fields[fieldSecrets].SetValue(c.in)
		if got := f.secretsMode(); got != c.want {
			t.Errorf("secretsMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNewFormDefaultsToKeychain(t *testing.T) {
	f := NewConnectionForm()
	if got := f.secretsMode(); got != "keychain" {
		t.Errorf("new form secretsMode = %q, want keychain", got)
	}
}

func TestSecretsModeFromConfig(t *testing.T) {
	// Plaintext config -> plain (do not silently migrate on re-save).
	plain := config.ConnectionConfig{Password: "hunter2"}
	if got := secretsModeFromConfig(plain); got != "plain" {
		t.Errorf("plaintext config mode = %q, want plain", got)
	}

	// A reference anywhere -> keychain.
	ref := config.ConnectionConfig{Password: "secret://prod/password"}
	if got := secretsModeFromConfig(ref); got != "keychain" {
		t.Errorf("reference config mode = %q, want keychain", got)
	}

	ref2 := config.ConnectionConfig{SSHPassword: "secret://x/ssh_password"}
	if got := secretsModeFromConfig(ref2); got != "keychain" {
		t.Errorf("ssh reference config mode = %q, want keychain", got)
	}
}

func TestNewConnectionFormEditSetsSecretsMode(t *testing.T) {
	// Editing a plaintext config preserves the user's plain preference.
	plain := config.ConnectionConfig{
		Name:     "staging",
		Driver:   "mysql",
		Password: "secret",
	}
	f := NewConnectionFormEdit(plain)
	if got := f.secretsMode(); got != "plain" {
		t.Errorf("edit plaintext mode = %q, want plain", got)
	}
	if f.fields[fieldPass].Value() != "secret" {
		t.Errorf("plaintext password not pre-filled: %q", f.fields[fieldPass].Value())
	}
}
