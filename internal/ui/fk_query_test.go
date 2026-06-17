package ui

import "testing"

func TestBuildForeignKeyQuery(t *testing.T) {
	got := buildForeignKeyQuery("users", "id", "42")
	want := "SELECT * FROM users WHERE id = '42'"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	got = buildForeignKeyQuery("users", "id", "O'Brien")
	want = "SELECT * FROM users WHERE id = 'O''Brien'"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
