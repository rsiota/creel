# gsql — Project Memory

## Overview
A fast, memory-efficient SQL TUI inspired by [sqlit](https://github.com/Maxteabag/sqlit) (which is Python/Textual). Written in Go for speed. Currently supports **SQLite** and **MySQL**.

## Tech Stack
- **Language**: Go 1.26+
- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-style architecture) + [Lipgloss](https://github.com/charmbracelet/lipgloss) (styling) + [Bubbles](https://github.com/charmbracelet/bubbles) (components)
- **SQLite**: `modernc.org/sqlite` (pure Go, no CGO required)
- **MySQL**: `github.com/go-sql-driver/mysql`
- **Config**: YAML (`gopkg.in/yaml.v3`), stored at `~/.config/gsql/config.yaml`

## Build & Run Commands
- **Build**: `go build -o gsql ./cmd/gsql/`
- **Build all packages**: `go build ./...`
- **Run TUI**: `./gsql`
- **CLI mode**: `./gsql -e "SELECT * FROM users" -database /tmp/test.db`
- **Vet**: `go vet ./...`
- **Tidy deps**: `go mod tidy`

## Architecture
```
cmd/gsql/main.go          — Entry point (TUI + CLI modes)
internal/db/              — Database abstraction layer
  db.go                   — DB interface, Connection wrapper
  sqlite.go               — SQLite implementation
  mysql.go                — MySQL implementation
  ssh_tunnel.go           — SSH tunnel (golang.org/x/crypto/ssh)
internal/config/          — Config loading/saving (YAML)
internal/history/         — Query history (per-connection JSON, searchable)
internal/ui/              — All Bubble Tea UI components
  app.go                  — Top-level Model (state machine)
  styles.go               — Shared color palette + lipgloss styles
  connection_list.go      — Connection selection screen
  connection_form.go      — Add/edit connection form
  query_editor.go         — SQL editor with vim mode (bubbles/textarea)
  results_table.go        — Query results table (custom renderer)
  help.go                 — Help overlay panel (toggled with `?`)
  history_panel.go        — Query history overlay panel
  connection_form_test.go — Tests for form validation
  table_scroll_test.go    — Tests for sidebar table/schema scrolling
```

## Key Design Decisions
- **DB interface** in `internal/db/db.go` — all drivers implement this, making it trivial to add Postgres etc. later
- **Elm-style state machine** in app.go: `stateConnections` → `stateWorkspace`, with `Focus` cycling between panels
- **Pure Go SQLite** (`modernc.org/sqlite`) — no CGO, simpler cross-compilation
- **lipgloss v1.1.0** — colors must use `lipgloss.Color()` not raw strings
- **Bubbles v1.0.0** — list delegate `Render` signature is `Render(w io.Writer, m Model, index int, item Item)`

## Vertical Slices Progress
- [x] Slice 1: Project scaffold + DB abstraction + CLI mode
- [x] Slice 1b: TUI skeleton (connection list, editor, results, workspace layout)
- [x] Slice 2: Connection manager (add/edit/delete connections from TUI, saved to config)
- [x] Slice 3: Vim mode editing (normal/insert modes, h/j/k/l, i/a/o/A/O, dd/dw/x/D, y/p)
- [x] Slice 4: Query history (per-connection, persisted, searchable, overlay panel)
- [x] Slice 5: Full table browser (expand/collapse columns, schema view, quick actions)
- [x] Slice 6: Row pagination for large result sets (LIMIT/OFFSET wrapping, 200/page)
- [x] Fuzzy table search in sidebar (press `/`, type to filter, `enter` to select, `esc` to cancel)
- [x] SSH tunnel support (MySQL via bastion host; key-based auth with passphrase + password auth)
- [x] Record inspector (Ctrl+O toggles right-side panel; vertical form editor that tracks results cursor)
- [x] Help overlay (`?` toggles a full keybindings popup; status bar shows only context info — connection, table, dimensions, messages)
- [x] Column hide/visibility (`H` hides cursor column, `g H` shows all, `v` opens a column-visibility overlay; hidden cols are display-only and survive same-table re-queries)
- [x] Drop table (`D` in sidebar with typed-name confirmation)
- [ ] DB export (pure-Go dumper: SQL done in `internal/db/dump.go`; CSV/JSON + UI picker pending)

## Config Format (~/.config/gsql/config.yaml)
```yaml
connections:
  - name: local-dev
    driver: sqlite
    database: /path/to/db.sqlite
  - name: staging
    driver: mysql
    database: myapp
    host: 10.0.0.5
    port: 3306
    username: admin
    password: secret
```

## Conventions
- All colors/styles centralized in `internal/ui/styles.go`
- Tokyo Night color palette
- Value receivers in Bubble Tea Model methods (per Elm architecture — model is immutable)
- Database queries use `sql.NullString` for safe scanning
