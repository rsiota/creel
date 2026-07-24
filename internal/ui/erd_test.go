package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/db"
)

// erdFixture is a tiny schema: orders.user_id → users.id.
func erdFixture() (tables []string, schemas map[string][]db.Column, pks map[string][]string, fks map[string][]db.ForeignKey) {
	tables = []string{"users", "orders"}
	schemas = map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}, {Name: "total", Type: "real"}},
	}
	pks = map[string][]string{"users": {"id"}, "orders": {"id"}}
	fks = map[string][]db.ForeignKey{"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}}}
	return
}

func TestBuildMermaidERD(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	out := strings.Join(buildMermaidERD(tables, schemas, pks, fks), "\n")

	if !strings.HasPrefix(out, "erDiagram") {
		t.Errorf("missing erDiagram header:\n%s", out)
	}
	for _, want := range []string{
		"orders {",       // entity block
		"int id PK",      // PK marker
		"int user_id FK", // FK marker
		"users {",
		"users ||--o{ orders : user_id", // relationship
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A column that is both PK and FK gets the combined PK,FK marker.
func TestBuildMermaidERDPKAndFK(t *testing.T) {
	schemas := map[string][]db.Column{
		"memberships": {{Name: "user_id", Type: "int"}},
	}
	fks := map[string][]db.ForeignKey{
		"memberships": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	pks := map[string][]string{"memberships": {"user_id"}}
	out := strings.Join(buildMermaidERD([]string{"memberships"}, schemas, pks, fks), "\n")
	if !strings.Contains(out, "int user_id PK,FK") {
		t.Errorf("expected PK,FK marker:\n%s", out)
	}
}

// Fks whose parent is outside the drawn entity set are omitted, so a focused
// neighbourhood stays self-contained.
func TestBuildMermaidERDOmitsCrossSetFKs(t *testing.T) {
	_, schemas, pks, fks := erdFixture()
	// Draw only "orders"; its user_id → users FK must be dropped (users absent).
	out := strings.Join(buildMermaidERD([]string{"orders"}, schemas, pks, fks), "\n")
	if strings.Contains(out, "||--o{") {
		t.Errorf("cross-set FK should be omitted:\n%s", out)
	}
}

func TestBuildMermaidERDNoTables(t *testing.T) {
	out := buildMermaidERD(nil, nil, nil, nil)
	if len(out) != 1 || !strings.Contains(out[0], "no tables") {
		t.Errorf("expected a no-tables message, got %v", out)
	}
}

func TestERDFocusSet(t *testing.T) {
	tables, _, _, fks := erdFixture()
	// Focusing "users" pulls in orders (inbound FK) — and users itself.
	got := erdFocusSet("users", tables, fks)
	want := map[string]bool{"users": true, "orders": true}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected focus member %q", name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("focus set = %v, want %v", got, want)
	}
}

func TestERDType(t *testing.T) {
	cases := map[string]string{
		"int":                         "int",
		"varchar(255)":                "varchar",
		"timestamp without time zone": "timestamp",
		"":                            "text",
		"weird/type!":                 "weird_type_",
	}
	for in, want := range cases {
		if got := erdType(in); got != want {
			t.Errorf("erdType(%q) = %q, want %q", in, got, want)
		}
	}
}
