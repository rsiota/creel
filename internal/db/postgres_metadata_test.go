package db

import "testing"

func TestParsePostgresIndexDef(t *testing.T) {
	cases := []struct {
		name     string
		def      string
		wantCols []string
		wantPart string
	}{
		{
			name:     "plain single column",
			def:      "CREATE INDEX idx_users_email ON public.users (email)",
			wantCols: []string{"email"},
			wantPart: "",
		},
		{
			name:     "multi column",
			def:      "CREATE INDEX idx ON public.users (last_name, first_name)",
			wantCols: []string{"last_name", "first_name"},
			wantPart: "",
		},
		{
			name:     "partial index",
			def:      "CREATE INDEX idx_active ON public.users (id) WHERE (active = true)",
			wantCols: []string{"id"},
			wantPart: "(active = true)",
		},
		{
			name:     "expression index",
			def:      "CREATE INDEX idx_lower ON public.users (lower(email))",
			wantCols: []string{"lower(email)"},
			wantPart: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cols, partial := parsePostgresIndexDef(c.def)
			if !equalStrings(cols, c.wantCols) {
				t.Errorf("columns = %v, want %v", cols, c.wantCols)
			}
			if partial != c.wantPart {
				t.Errorf("partial = %q, want %q", partial, c.wantPart)
			}
		})
	}
}

func TestParsePostgresTriggerDef(t *testing.T) {
	cases := []struct {
		def        string
		wantTiming string
		wantEvent  string
	}{
		{"TRIGGER trg BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION f()", "BEFORE", "UPDATE"},
		{"TRIGGER trg AFTER INSERT ON public.users FOR EACH ROW EXECUTE FUNCTION f()", "AFTER", "INSERT"},
		{"TRIGGER trg INSTEAD OF DELETE ON public.v FOR EACH ROW EXECUTE FUNCTION f()", "INSTEAD OF", "DELETE"},
		{"TRIGGER trg AFTER TRUNCATE ON public.users FOR EACH STATEMENT EXECUTE FUNCTION f()", "AFTER", "TRUNCATE"},
	}
	for _, c := range cases {
		t.Run(c.wantTiming+"/"+c.wantEvent, func(t *testing.T) {
			timing, event := parsePostgresTriggerDef(c.def)
			if timing != c.wantTiming || event != c.wantEvent {
				t.Errorf("got (%q,%q), want (%q,%q)", timing, event, c.wantTiming, c.wantEvent)
			}
		})
	}
}
