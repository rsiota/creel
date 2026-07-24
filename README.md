# gsql

A fast, memory-efficient SQL TUI for **SQLite**, **MySQL**, and **PostgreSQL**, written in Go.

Inspired by [sqlit](https://github.com/Maxteabag/sqlit) (Python/Textual), `gsql` brings a vim-driven, keyboard-first workflow to browsing schemas, running queries, and editing data — all from the terminal.

## Features

- **Three databases, one interface** — connect to SQLite, MySQL, or PostgreSQL (with SSH tunneling for remote MySQL)
- **Secret storage** — passwords and SSH passwords are stored in the OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service) rather than plaintext config; falls back to plaintext on systems without a keychain
- **Vim-mode editor** — normal/insert modes, motions (`h/j/k/l`, `w/b`), operators (`dd`, `dw`, `x`, `D`), yank/paste, and SQL autocompletion
- **Table browser** — expand/collapse columns, inspect schemas, fuzzy-filter tables
- **Results grid** — sort, filter, search, hide/show columns, follow foreign keys (`g d`), mark and bulk-delete rows
- **Relationship explorer** (`g r` / `:explore panel`) — a docked panel that browses a row like a folder: an expandable object-graph tree of the focused row's inbound and outbound foreign keys, with a live per-edge count. Expand an edge (`→`) to see the related rows inline, expand a row to see *its* edges, and so on (depth-capped); `Enter` opens a node in the grid, `←` collapses, `r` retargets. The panel stays open and re-roots as the cursor moves. Counts and child rows fan out concurrently across all three drivers
- **Inline editing** — edit cells directly, insert rows, clone rows, paste from clipboard
- **Record inspector** — side panel with a vertical form view that tracks the results cursor
- **Query history & bookmarks** — per-connection, persisted, searchable
- **Session restore** — reopening a connection brings back your open tabs and editor buffers from the last visit (keyed per connection + database). Buffers are restored but not re-executed; a `gsql -f` startup file still takes precedence on first connect. `:session clear` wipes the saved snapshot, `:session save` snapshots now
- **EXPLAIN plans** — driver-aware rendering (`g e`) of query plans
- **Schema editing** — add columns, rename tables, create/drop/truncate tables, and a grid-based table designer (`N`)
- **Table structure view** (`d`) — a tabbed structure editor: columns (editable grid), foreign keys, indexes, check constraints, and triggers in one view, plus a definition tab for views
- **Read-only mode** — a per-connection `readonly: true` flag or a global `--readonly` CLI flag that rejects writes (INSERT/UPDATE/DELETE/DDL), blocks transactions and imports, and opens the connection read-only at the engine level (SQLite `query_only`, Postgres `default_transaction_read_only`); a `READ-ONLY` indicator shows in the status bar
- **Import / export** — streaming SQL dump importer (`I`) and a pure-Go `mysqldump`-compatible exporter (`X`); CSV export (`x`) for result sets
- **Cross-table search** (`S`) and **column statistics** (`g s`)
- **Command palette** (`Ctrl+P`) and a full **help overlay** (`?`)
- **Pagination** — large result sets are paged (LIMIT/OFFSET) for speed and low memory
- **CLI mode** — run a query and print results without launching the TUI

## Requirements

- Go 1.26+
- A terminal with Unicode support

## Build

```sh
go build -o gsql ./cmd/gsql/
```

## Usage

### Interactive TUI

```sh
./gsql
```

On first run, no config exists — use the connection list (`n` to add) to create connections, which are saved to `~/.config/gsql/config.yaml`.

### CLI mode

Execute a single query and print results as TSV:

```sh
./gsql -e "SELECT * FROM users" -database /tmp/test.db
./gsql -e "SHOW TABLES" -driver mysql -database myapp -host 10.0.0.5 -user admin -password secret
```

Flags:

| Flag        | Description                                        | Default   |
| ----------- | -------------------------------------------------- | --------- |
| `-e`        | SQL query to execute (enables CLI mode)            |           |
| `-driver`   | `sqlite`, `mysql`, or `postgres`                   | `sqlite`  |
| `-database` | Database name (SQLite path or MySQL/PG database)   |           |
| `-host`     | Database host (MySQL/Postgres)                     | `localhost` |
| `-port`     | Database port (MySQL/Postgres)                     | `3306`    |
| `-user`     | Username (MySQL/Postgres)                          | `root`    |
| `-password` | Password (MySQL/Postgres)                          |           |
| `-cli`      | Force CLI mode                                     | `false`   |

## Configuration

Connections are stored at `~/.config/gsql/config.yaml`. Secret fields (passwords)
default to the OS keychain; in that case the config holds an opaque reference
instead of the real password:

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
    password: secret://staging/password   # looked up in the OS keychain
    group: Work                            # optional folder in the connection list
  - name: prod-pg
    driver: postgres
    database: analytics
    host: db.internal
    port: 5432
    username: readonly
    password: secret://prod-pg/password
  # MySQL behind a bastion host
  - name: tunneled
    driver: mysql
    database: reports
    host: 10.0.0.20
    port: 3306
    username: admin
    password: secret://tunneled/password
    ssh_host: bastion.example.com
    ssh_port: 22
    ssh_user: deploy
    ssh_key_path: ~/.ssh/id_ed25519
```

A `password:` value that is **not** a `secret://` reference is treated as
plaintext and used directly, so existing configs keep working. Set the
connection form's **Secrets** field to `plain` to opt out of the keychain for a
specific connection.

### Read-only mode

Point gsql at production safely by marking a connection `readonly: true` (see
the `prod-pg` example below) or launching with `gsql --readonly` to force every
connection read-only. In either case writes are refused — `Exec`/`Execute`
return an error for INSERT/UPDATE/DELETE/DDL, `Begin`/`Session` (imports) are
blocked — and the connection is opened read-only at the engine level where the
driver supports it. A `READ-ONLY` badge appears in the status bar.

```yaml
connections:
  - name: prod-pg
    driver: postgres
    database: analytics
    host: db.internal
    port: 5432
    username: readonly
    password: secret://prod-pg/password
    readonly: true
```

### Connection groups

Give a connection a `group` to organize the connection list into collapsible
folders (see the `staging` example above):

```yaml
connections:
  - name: staging
    driver: mysql
    # ...
    group: Work
  - name: personal-db
    driver: sqlite
    database: ~/notes.db
    group: Personal
```

In the connection list, grouped connections render indented under `▾ Group`
headers (with a connection count), ungrouped ones lead under "Ungrouped", then
named groups alphabetically. Press `space` (or `tab`) to fold/unfold the group
under the cursor, or `enter` on a header to toggle it. Filtering (`/`) flattens
the list to ranked matches regardless of groups. Connections with no `group`
render exactly as before when none of your connections use groups.

### AI assistant

The assistant panel (and the `:ai` command) turn a natural-language question
into SQL using any OpenAI-compatible endpoint (OpenAI, z.ai, OpenRouter,
Ollama, LM Studio, …). Configure it **in-app** from the assistant panel:

- `M` opens the provider picker. From there `n` adds a provider, `e` edits the
  one under the cursor, `d` deletes it (a `y/n` prompt confirms first, like
  deleting a connection), and `enter` makes one the active default.
- The add/edit form collects a **Name**, **API Key**, **Base URL**, and a
  **Secrets** toggle (`keychain` / `plain`). In `keychain` mode (the default)
  the API key is stored in the OS keychain as a `secret://` reference — never
  written to the config file in plaintext — exactly like a connection password.
- `ctrl+t` in the form probes the provider's `/models` endpoint and reports
  `✓ reachable` or the real error (a valid key pointed at the wrong endpoint
  is the usual cause of a confusing "unauthorized").
- `m` (from the panel) browses the models the active provider exposes and
  pins one as that provider's `model:`.

The same data lives under an `ai:` block if you prefer to hand-edit:

```yaml
ai:
  default: openai
  providers:
    - name: openai
      api_key: secret://ai/openai/api_key   # keychain ref (set by the form)
      base_url: https://api.openai.com/v1   # optional; defaults to OpenAI
      model: gpt-4o-mini                     # optional; set via `m`
```

With no providers configured, gsql falls back to environment variables
(`GSQL_AI_API_KEY` / `OPENAI_API_KEY` / `ZAI_API_KEY`, and optionally
`GSQL_AI_BASE_URL` / `GSQL_AI_MODEL`), so the panel works with zero config.

### Settings

Top-level app preferences live under a `settings:` block. All fields are
optional and fall back to defaults when omitted:

| Key              | Default      | Description                                                              |
| ---------------- | ------------ | ------------------------------------------------------------------------ |
| `page_size`      | 200          | Rows fetched per page in results                                         |
| `query_timeout`  | 30s          | Per-query deadline (friendly form: `2m`, `1h30m`, or a bare seconds int) |
| `default_driver` | sqlite       | Driver pre-filled in the add-connection form                             |
| `theme`          | tokyo-night  | Palette: `tokyo-night`, `gruvbox`, `nord`, `catppuccin`, `light` + ~565 auto-derived from iTerm2-Color-Schemes (dracula, solarized, …). Unknown → default |
| `icons`          | unicode      | Glyph set for tree expand/collapse markers (sidebar, connection groups, relationship explorer). `unicode` uses portable triangles (▾/▸); `nerdfont` uses Nerd Font angle chevrons (U+F107/U+F105) — open, rotationally-symmetric like treemacs, but only renders correctly in a terminal running a Nerd Font. Unknown → default |
| `transparent_background` | false | By default gsql fills the app background with the theme's bg colour (required for light themes to be readable). Set `true` to leave it unpainted so the terminal's own background / transparency shows through — at the cost of light themes looking wrong. |
| `confirm_destructive` | true | Destructive actions (drop table/database, truncate, delete rows, discard edits, drop column, delete provider/connection, clear history/bookmarks) prompt for confirmation. Set `false` to skip the prompts and run each action immediately. |

```yaml
settings:
  page_size: 500
  query_timeout: 1m
  default_driver: postgres
  theme: gruvbox
  icons: nerdfont
  confirm_destructive: false
```

`query_timeout` accepts values like `30s`, `2m`, `1h30m`, or a bare number of
seconds (`45`). An invalid value makes config load fail loudly rather than
silently falling back. An unknown `theme` silently falls back to the default
rather than blocking startup. (`cursor_style` is reserved for upcoming work.)

To experiment with themes live, press `g c` in the workspace to open the
theme picker (a scrollable, filterable overlay — type to filter by name,
`↑`/`↓` to preview); moving the selection re-themes the UI immediately,
`enter` saves the choice to config, and `esc` reverts. The theme's background
is painted too, so light themes preview correctly. See `THIRDPARTY.md`
for theme attribution.

## Keybindings

Press `?` inside the TUI for the full overlay, or `Ctrl+P` for the fuzzy command palette.

### Global

| Key             | Action                          |
| --------------- | ------------------------------- |
| `ctrl+e` / `\`  | Run statement under cursor      |
| `ctrl+r`        | Refresh schema & re-run query   |
| `esc` / `ctrl+c`| Cancel running query            |
| `ctrl+w`        | Maximize / restore editor       |
| `ctrl+t`        | Switch connection               |
| `ctrl+b`        | Browse databases (MySQL)        |
| `ctrl+y`        | Query history                   |
| `ctrl+g`        | Bookmarks                       |
| `B`             | Bookmark current query          |
| `ctrl+o`        | Toggle record inspector         |
| `ctrl+h/j/k/l`  | Move focus between panels       |
| `tab` / `shift+tab` | Cycle focus                |
| `ctrl+d` / `ctrl+u` | Next / previous page        |
| `ctrl+p`        | Command palette                 |
| `:`             | Ex command line (`:q`, `:sort`, `:goto`, …) |
| `g c`           | Theme picker (live preview)     |
| `?`             | Toggle help                     |
| `q` / `ctrl+q`  | Quit (not while editing)        |

### Connections

| Key        | Action                          |
| ---------- | ------------------------------- |
| `enter`    | Connect to selected             |
| `n`        | New connection                  |
| `e`        | Edit connection                 |
| `d`        | Delete connection               |
| `/`        | Filter connections              |

In the add/edit form:

| Key        | Action                          |
| ---------- | ------------------------------- |
| `tab`      | Next field                      |
| `enter`    | Save                            |
| `ctrl+t`   | Test connection (no save)       |
| `esc`      | Cancel                          |

### Sidebar (Tables)

| Key        | Action                    |
| ---------- | ------------------------- |
| `j/k`      | Move                      |
| `g g` / `G`| Top / bottom              |
| `space`    | Expand columns            |
| `enter` / `s` | `SELECT *` from table  |
| `d`        | Structure (columns/indexes/FKs/checks/triggers) |
| `a`        | Add column                |
| `r`        | Rename table              |
| `T`        | Truncate table            |
| `D`        | Drop table                |
| `N`        | New table (grid editor)   |
| `X`        | Export database            |
| `I`        | Import SQL dump           |
| `S`        | Cross-table search        |
| `/`        | Filter tables             |

### Editor (Vim)

| Key          | Action                          |
| ------------ | ------------------------------- |
| `i/a/o/A/O`  | Enter insert mode               |
| `esc`        | Normal mode                     |
| `h/j/k/l`, `w/b` | Move                       |
| `x` / `dd` / `dw` / `D` | Delete              |
| `y` / `p`    | Yank / paste                    |
| `ctrl+n`     | Autocomplete                    |
| `==`         | Format SQL                      |

### Results

| Key        | Action                                       |
| ---------- | -------------------------------------------- |
| `h/j/k/l`  | Move cursor                                  |
| `0` / `$`  | First / last column                          |
| `g g` / `G`| Top / bottom                                 |
| `/`        | Search all columns                           |
| `g /`      | Regex search                                 |
| `n` / `N`   | Next / previous match                       |
| `o`        | Sort column                                  |
| `g s`      | Column statistics                            |
| `g e`      | Explain query plan                           |
| `g d`      | Follow foreign key                           |
| `g b`      | Go back                                      |
| `*`        | Keep rows equal to cursor cell               |
| `!`        | Hide rows equal to cursor cell               |
| `g f`      | Filter column values                         |
| `space`    | Toggle row mark                              |
| `F`        | Filter by marked rows                        |
| `C`        | Clear marks                                  |
| `u`        | Undo last filter                             |
| `c`        | Clear filters                                |
| `V`        | Visual mode (select range)                   |
| `dd`       | Delete marked or cursor row                  |
| `e`        | Edit cell                                    |
| `E`        | Expand cell (large values)                   |
| `ctrl+s`   | Save edits                                   |
| `A`        | Insert new row                               |
| `D`        | Discard edits                                |
| `H`        | Hide column                                  |
| `g H`      | Show all columns                             |
| `v`        | Column visibility overlay                    |
| `:`        | Ex command line (`:q`, `:sort`, `:goto`, …; column jump in results) |
| `y` / `p`  | Copy cell / paste to cell                    |
| `Y`        | Copy rows as INSERT statements               |
| `P`        | Clone marked/cursor row                      |
| `x`        | Export results to CSV                        |

## Architecture

```
cmd/gsql/main.go        Entry point (TUI + CLI modes)
internal/db/            Database abstraction layer
  db.go                 DB interface + Connection wrapper
  sqlite.go             SQLite driver (modernc.org/sqlite, pure Go)
  mysql.go              MySQL driver
  postgres.go           PostgreSQL driver (pgx/v5)
  ssh_tunnel.go         SSH tunnel support
  dump.go / import.go   Export & streaming import
internal/config/        YAML config load/save
internal/secrets/       OS keychain secret store (secret:// refs + plaintext fallback)
internal/history/       Per-connection query history (JSON)
internal/session/      Per-connection workspace state — open tabs, restored on reconnect (JSON)
internal/bookmarks/     Saved queries
internal/ui/            Bubble Tea components
  app.go                Top-level Model (Elm-style state machine)
  query_editor.go       SQL editor with vim mode
  results_table.go      Results grid renderer
  sidebar / inspector / editor overlays
  registry.go           Single source of truth for keybindings
  palette.go            Fuzzy command palette (Ctrl+P)
```

### Design notes

- **Driver interface** — `internal/db/db.go` defines the `DB` interface; every driver implements it, so adding a new database is self-contained.
- **Elm architecture** — the UI is an immutable state machine in `app.go` (`stateConnections` → `stateWorkspace`), with focus cycling between panels.
- **Pure-Go SQLite** — no CGO, which simplifies cross-compilation.
- **Keybinding registry** — `registry.go` is the single source of truth for both the help overlay and the command palette, with a drift-detection test ensuring documented keys are actually wired.

## License

MIT
