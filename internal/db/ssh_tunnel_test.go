package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHomePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/abs/id_rsa", "/abs/id_rsa"},
		{"foo/bar", "foo/bar"},
		{"foo/../bar", "bar"},
	}
	for _, c := range cases {
		got, err := expandHomePath(c.in)
		if err != nil {
			t.Errorf("expandHomePath(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("expandHomePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err := expandHomePath("~/.ssh/id_rsa")
	if err != nil {
		t.Fatalf("expandHomePath(~/.ssh/id_rsa) error: %v", err)
	}
	want := filepath.Join(home, ".ssh", "id_rsa")
	if got != want {
		t.Fatalf("expandHomePath(~/.ssh/id_rsa) = %q, want %q", got, want)
	}
}
