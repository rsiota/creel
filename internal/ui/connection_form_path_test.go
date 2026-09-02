package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConnectionFormSQLitePathCompletion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.db"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	f := NewConnectionForm()
	f.SetSize(60, 24)
	f.active = 2 // Database field for sqlite (name, driver, database)
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !f.editing || !f.ActiveIsPathField() {
		t.Fatal("expected insert mode on sqlite database path field")
	}

	for _, ch := range dir + "/a" {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if !f.pathComp.compVisible {
		t.Fatal("expected path completions after typing a directory prefix")
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	want := filepath.Join(dir, "app.db")
	if got := f.fields[fieldDatabase].Value(); got != want {
		t.Fatalf("after tab: value = %q, want %q", got, want)
	}
}

func TestConnectionFormSQLitePathCompletionEnterAccepts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.db"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	f := NewConnectionForm()
	f.SetSize(60, 24)
	f.active = 2 // Database field for sqlite
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	for _, ch := range dir + "/a" {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if !f.pathComp.compVisible {
		t.Fatal("expected path completions after typing a directory prefix")
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	want := filepath.Join(dir, "app.db")
	if got := f.fields[fieldDatabase].Value(); got != want {
		t.Fatalf("after enter: value = %q, want %q", got, want)
	}
	if !f.editing {
		t.Fatal("enter with open completions should accept, not leave insert mode")
	}
}

func TestConnectionFormPathEnterLeavesInsertWithoutCompletions(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(60, 24)
	f.active = 2
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if f.editing {
		t.Fatal("enter without completions should leave insert mode")
	}
}

func TestConnectionFormSSHKeyPathCompletion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql")
	f.fields[fieldSSHHost].SetValue("bastion")
	f.setPage(formPageSSH)
	f.SetSize(60, 30)
	// Jump to SSH Key field.
	for _, fi := range f.visibleFields() {
		if fi == fieldSSHKeyPath {
			break
		}
		f.active++
	}
	if f.activeField() != fieldSSHKeyPath {
		t.Fatalf("active field = %d, want SSH key path", f.activeField())
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	for _, ch := range "~/.ssh/i" {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if !f.pathComp.compVisible {
		t.Fatal("expected SSH key path completions")
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := f.fields[fieldSSHKeyPath].Value(); got != "~/.ssh/id_rsa" {
		t.Fatalf("after tab: value = %q, want ~/.ssh/id_rsa", got)
	}
}

func TestConnectionFormSSHKeyPathCompletionEnterAccepts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql")
	f.fields[fieldSSHHost].SetValue("bastion")
	f.setPage(formPageSSH)
	f.SetSize(60, 30)
	for _, fi := range f.visibleFields() {
		if fi == fieldSSHKeyPath {
			break
		}
		f.active++
	}
	if f.activeField() != fieldSSHKeyPath {
		t.Fatalf("active field = %d, want SSH key path", f.activeField())
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	for _, ch := range "~/.ssh/i" {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if !f.pathComp.compVisible {
		t.Fatal("expected SSH key path completions")
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := f.fields[fieldSSHKeyPath].Value(); got != "~/.ssh/id_rsa" {
		t.Fatalf("after enter: value = %q, want ~/.ssh/id_rsa", got)
	}
	if !f.editing {
		t.Fatal("enter with open completions should accept, not leave insert mode")
	}
}

func TestConnectionFormNetworkDatabaseNotPathField(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql")
	if f.isPathField(fieldDatabase) {
		t.Fatal("mysql database field should not use path completion")
	}
}
