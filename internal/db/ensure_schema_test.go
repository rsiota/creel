package db

import "testing"

func TestEnsureActiveSchemaPostgres(t *testing.T) {
	conn, err := New(ConnectionConfig{
		Driver: DriverPostgres, Host: "127.0.0.1", Port: 5432,
		Username: "postgres", Database: "postgres",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Skip(err)
	}
	defer conn.Close()

	if err := conn.UseDatabase("demo"); err != nil {
		t.Skip(err)
	}
	if got := conn.Config().Schema; got != "" {
		t.Fatalf("after UseDatabase Schema = %q, want empty", got)
	}
	if err := conn.EnsureActiveSchema(); err != nil {
		t.Fatal(err)
	}
	if got := conn.Config().Schema; got != "public" {
		t.Fatalf("EnsureActiveSchema Schema = %q, want public", got)
	}

	// Already set: leave alone (do not re-query / overwrite).
	conn.config.Schema = "bookings"
	if err := conn.EnsureActiveSchema(); err != nil {
		t.Fatal(err)
	}
	if got := conn.Config().Schema; got != "bookings" {
		t.Fatalf("should keep explicit Schema, got %q", got)
	}
}

func TestEnsureActiveSchemaNoopWithoutDB(t *testing.T) {
	c := ConnectionFromConfig(ConnectionConfig{Driver: DriverPostgres, Database: "x"})
	if err := c.EnsureActiveSchema(); err != nil {
		t.Fatal(err)
	}
	if c.Config().Schema != "" {
		t.Fatalf("stub connection should stay empty, got %q", c.Config().Schema)
	}
}
