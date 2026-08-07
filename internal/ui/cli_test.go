package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestSerialize(t *testing.T) {
	cols := []string{"id", "name"}
	rows := [][]string{{"1", "alice"}, {"2", "NULL"}} // NULL exercises cell handling

	tests := []struct {
		format string
		check  func(string) error
	}{
		{"tsv", func(s string) error {
			want := "id\tname\n1\talice\n2\t\n" // NULL renders as an empty field
			if s != want {
				return fmt.Errorf("got %q, want %q", s, want)
			}
			return nil
		}},
		{"csv", func(s string) error {
			if !strings.HasPrefix(s, "id,name\n") || !strings.Contains(s, "\n1,alice\n") || !strings.Contains(s, "\n2,\n") {
				return fmt.Errorf("got %q, want header + 1,alice + empty NULL field", s)
			}
			return nil
		}},
		{"json", func(s string) error {
			// NULL becomes JSON null (MarshalIndent adds a space after the colon).
			if !strings.Contains(s, `"name": null`) || !strings.Contains(s, `"name": "alice"`) {
				return fmt.Errorf("got %q, want name: null and name: \"alice\"", s)
			}
			return nil
		}},
		{"jsonl", func(s string) error {
			lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
			if len(lines) != 2 || !strings.Contains(lines[1], `"name":null`) {
				return fmt.Errorf("got %q, want two lines, second with name:null", s)
			}
			return nil
		}},
		{"md", func(s string) error {
			if !strings.Contains(s, "| id |") || !strings.Contains(s, "| --- |") || !strings.Contains(s, "| 2 |") {
				return fmt.Errorf("got %q, want markdown table header/separator/rows", s)
			}
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			out, err := Serialize(tt.format, cols, rows)
			if err != nil {
				t.Fatalf("Serialize(%q): %v", tt.format, err)
			}
			if err := tt.check(out); err != nil {
				t.Errorf("Serialize(%q): %v", tt.format, err)
			}
		})
	}

	// Unknown format is rejected with a clear error.
	if _, err := Serialize("yaml", cols, rows); err == nil {
		t.Error("Serialize(unknown) should return an error")
	}
}

func TestResolveConnection(t *testing.T) {
	cfg := &config.Config{
		Connections: []config.ConnectionConfig{
			{
				Name:     "prod",
				Driver:   string(db.DriverSQLite),
				Database: "/tmp/prod.db",
				Password: "hunter2", // plaintext (not a keyring ref) → resolves without keychain
			},
			{Name: "ro", Driver: string(db.DriverSQLite), Database: "/tmp/ro.db", ReadOnly: true},
		},
	}

	t.Run("not found", func(t *testing.T) {
		if _, err := ResolveConnection(cfg, "missing", false); err == nil {
			t.Error("ResolveConnection should error for an unknown name")
		}
	})

	t.Run("found", func(t *testing.T) {
		got, err := ResolveConnection(cfg, "prod", false)
		if err != nil {
			t.Fatalf("ResolveConnection: %v", err)
		}
		if got.Name != "prod" || got.Database != "/tmp/prod.db" || got.Password != "hunter2" {
			t.Errorf("resolved config = %+v", got)
		}
		if got.ReadOnly {
			t.Error("non-readonly connection should not be forced read-only")
		}
	})

	t.Run("forceReadOnly", func(t *testing.T) {
		// A connection that isn't readonly on its own becomes readonly via the flag.
		got, err := ResolveConnection(cfg, "prod", true)
		if err != nil {
			t.Fatalf("ResolveConnection: %v", err)
		}
		if !got.ReadOnly {
			t.Error("forceReadOnly should make the resolved connection read-only")
		}
	})

	t.Run("inherits connection readonly", func(t *testing.T) {
		got, err := ResolveConnection(cfg, "ro", false)
		if err != nil {
			t.Fatalf("ResolveConnection: %v", err)
		}
		if !got.ReadOnly {
			t.Error("a readonly connection should stay read-only without the flag")
		}
	})
}
