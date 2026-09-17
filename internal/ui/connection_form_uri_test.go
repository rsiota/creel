package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestConnectionFormApplyURI(t *testing.T) {
	f := NewConnectionForm()
	if err := f.ApplyURI("postgres://alice:secret@db.example:5433/orders?sslmode=require"); err != nil {
		t.Fatal(err)
	}
	if f.driver() != "postgres" {
		t.Fatalf("driver = %q", f.driver())
	}
	if got := f.fields[fieldName].Value(); got != "db.example/orders" {
		t.Fatalf("name = %q", got)
	}
	if got := f.fields[fieldHost].Value(); got != "db.example" {
		t.Fatalf("host = %q", got)
	}
	if got := f.fields[fieldPort].Value(); got != "5433" {
		t.Fatalf("port = %q", got)
	}
	if got := f.fields[fieldUser].Value(); got != "alice" {
		t.Fatalf("user = %q", got)
	}
	if got := f.fields[fieldPass].Value(); got != "secret" {
		t.Fatalf("pass = %q", got)
	}
	if got := f.fields[fieldDatabase].Value(); got != "orders" {
		t.Fatalf("database = %q", got)
	}
	if got := f.fields[fieldSSLMode].Value(); got != "require" {
		t.Fatalf("ssl = %q", got)
	}
	cfg, errMsg := f.EnterPressed()
	if errMsg != "" {
		t.Fatalf("EnterPressed: %s", errMsg)
	}
	if cfg.Driver != "postgres" || cfg.Port != 5433 {
		t.Fatalf("saved cfg = %+v", cfg)
	}
}

func TestConnectionFormApplyURIKeepsName(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldName].SetValue("prod")
	if err := f.ApplyURI("mysql://root@localhost/app"); err != nil {
		t.Fatal(err)
	}
	if got := f.fields[fieldName].Value(); got != "prod" {
		t.Fatalf("name overwritten: %q", got)
	}
	if f.driver() != "mysql" {
		t.Fatalf("driver = %q", f.driver())
	}
}

func TestConnectionFormApplyURIInvalid(t *testing.T) {
	f := NewConnectionForm()
	if err := f.ApplyURI("not-a-uri"); err == nil {
		t.Fatal("expected error")
	}
	_ = db.DriverPostgres
}
