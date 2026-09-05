package ui

import "testing"

// TestExRegistryConsistency mirrors the keybinding registry's consistency
// test: every spec is well-formed and no verb is claimed by two commands.
func TestExRegistryConsistency(t *testing.T) {
	specs := exCommands()
	if len(specs) == 0 {
		t.Fatal("exCommands() returned no commands")
	}
	seen := map[string]int{}
	for i, s := range specs {
		if len(s.verbs) == 0 {
			t.Errorf("spec #%d has no verbs", i)
			continue
		}
		canonical := s.verbs[0]
		if canonical == "" {
			t.Errorf("spec #%d canonical verb is empty", i)
		}
		if s.desc == "" {
			t.Errorf("command %q has empty Desc", canonical)
		}
		if s.usage == "" {
			t.Errorf("command %q has empty Usage", canonical)
		}
		if s.run == nil {
			t.Errorf("command %q has nil Run", canonical)
		}
		for _, v := range s.verbs {
			if v == "" {
				t.Errorf("command %q has an empty alias", canonical)
				continue
			}
			if prev, ok := seen[v]; ok {
				t.Errorf("verb %q claimed by both %q and %q",
					v, specs[prev].verbs[0], canonical)
			}
			seen[v] = i
		}
	}
}

// TestExLookupResolvesKnownVerbs pins the verb/alias set so a refactor can't
// silently drop a command name. The old switch could lose a case without any
// test noticing; the registry dispatch can't, but the names themselves can
// still drift, so this guards them explicitly.
func TestExLookupResolvesKnownVerbs(t *testing.T) {
	known := []string{
		"e", "edit", "w", "write", "q", "quit", "qa", "wq", "x", "sort", "goto", "gt",
		"export", "copy", "connect", "c", "reconnect", "connections", "db", "use", "schema",
		"refs", "references", "uses", "begin", "transaction",
		"commit", "rollback", "help", "h", "run", "r",
		"explain", "plan", "new", "version", "recent",
		"truncate", "drop", "rename", "create",
		"createdb", "dropdb", "addcolumn", "discard", "clone",
		"follow", "back", "keep", "hide", "undo", "unfilter",
		"copyinsert", "copyrow", "regex", "hidecolumn", "showcolumns",
		"refresh", "reload", "history", "bookmarks", "bm",
		"describe", "desc", "d", "columns", "indexes", "constraints", "fk",
		"tables", "dt", "views", "dv", "schemas", "search", "find",
		"stats", "bar", "line", "scatter", "hist", "freq", "pie", "count", "sample", "head", "format",
		"bookmark", "import", "backup", "mysqldump", "pgdump", "pg_dump", "restore", "mysqlload", "psqlload", "pgload", "rerun", "watch", "tail", "theme", "icons",
		"limit", "timing", "peek", "filter", "open", "o", "save",
		"sizes", "locks", "blocked", "who", "sessions", "kill",
		"explain", "plan", "diagnose", "diag",
		"tabnew", "tabclose", "tabnext", "tabn", "tabprev", "tabp", "tabs", "diff",
		"ai", "aifix", "fixsql", "aiexplain", "why",
	}
	for _, v := range known {
		if exLookup(v) == nil {
			t.Errorf("exLookup(%q) = nil, want a command", v)
		}
	}
	if exLookup("definitely-not-a-command") != nil {
		t.Error("exLookup should return nil for an unknown verb")
	}
}
