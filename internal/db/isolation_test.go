package db

import "testing"

func TestParseIsolation(t *testing.T) {
	cases := []struct {
		in   string
		want IsolationLevel
		ok   bool
	}{
		{"", IsolationDefault, true},
		{"default", IsolationDefault, true},
		{"serializable", IsolationSerializable, true},
		{"S", IsolationSerializable, true},
		{"serial", IsolationSerializable, true},
		{"repeatable read", IsolationRepeatableRead, true},
		{"repeatable-read", IsolationRepeatableRead, true},
		{"rr", IsolationRepeatableRead, true},
		{"read committed", IsolationReadCommitted, true},
		{"read-committed", IsolationReadCommitted, true},
		{"RC", IsolationReadCommitted, true},
		{"read uncommitted", IsolationReadUncommitted, true},
		{"ru", IsolationReadUncommitted, true},
		{"bogus", IsolationDefault, false},
	}
	for _, tc := range cases {
		got, err := ParseIsolation(tc.in)
		if tc.ok {
			if err != nil {
				t.Errorf("ParseIsolation(%q): %v", tc.in, err)
				continue
			}
			if got != tc.want {
				t.Errorf("ParseIsolation(%q) = %v, want %v", tc.in, got, tc.want)
			}
		} else if err == nil {
			t.Errorf("ParseIsolation(%q) = %v, want error", tc.in, got)
		}
	}
}

func TestIsolationLabels(t *testing.T) {
	if IsolationSerializable.String() != "serializable" {
		t.Errorf("String = %q", IsolationSerializable.String())
	}
	if IsolationSerializable.Short() != "S" {
		t.Errorf("Short = %q", IsolationSerializable.Short())
	}
	if IsolationDefault.Short() != "" {
		t.Errorf("default Short should be empty, got %q", IsolationDefault.Short())
	}
}
