# creel

<p align="center">
  <a href="https://github.com/rsiota/creel/actions/workflows/ci.yml"><img src="https://github.com/rsiota/creel/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/rsiota/creel/releases/latest"><img src="https://img.shields.io/github/v/release/rsiota/creel?logo=github" alt="GitHub Release"></a>
  <a href="https://pkg.go.dev/github.com/rsiota/creel"><img src="https://pkg.go.dev/badge/github.com/rsiota/creel.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/rsiota/creel"><img src="https://goreportcard.com/badge/github.com/rsiota/creel" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/rsiota/creel?color=blue" alt="License"></a>
</p>
<p align="center">
  <a href="docs/images/demo.gif">
    <img src="docs/images/demo.gif" alt="creel — recorded demo" width="760">
  </a>
</p>
<p align="center"><em>A quick tour of creel, recorded with <a href="https://asciinema.org">asciinema</a>. Play it live and interactive: <code>asciinema play demo.cast</code></em></p>

A fast, memory-efficient SQL TUI for **SQLite**, **MySQL**, and **PostgreSQL**, written in Go.

Inspired by [sqlit](https://github.com/Maxteabag/sqlit) (Python/Textual), `creel` brings a vim-driven, keyboard-first workflow to browsing schemas, running queries, and editing data — all from the terminal.

## Quickstart

```sh
brew install rsiota/creel/creel        # or: go install github.com/rsiota/creel/cmd/creel@latest
creel                                  # press n to add a connection, or point at any SQLite file
```

To explore the bundled sample database (shown in the demo and screenshots):

```sh
git clone https://github.com/rsiota/creel.git && cd creel
creel -database demo/creel-demo.db     # then press g R for the ERD, g r on a row for its relationships
```

## Features

**Connect**

- Three databases, one interface — SQLite, MySQL, and PostgreSQL, with SSH tunneling for remote MySQL.
- Secret storage — passwords live in the OS keychain (macOS/Windows/Linux), never plaintext config.
- Read-only mode — point creel at production safely; per-connection flag or global `--readonly`.

**Browse**

- Table browser — expand columns, inspect schemas, fuzzy-filter.
- Results grid — sort, filter, search, hide columns, follow foreign keys (`g d`), chart columns (`M` + `:bar`).
- Relationship explorer (`g r`) — browse a row's inbound/outbound FK graph like a folder.
- Static ERD (`g R`) — graphical entity-relationship diagram; exportable as Mermaid.
- EXPLAIN plans (`g e`), cross-table search (`S`), column statistics (`g s`).

**Edit**

- Inline editing — edit cells, insert/clone rows, paste from clipboard.
- Schema editing — add columns, rename/drop/truncate tables, table designer (`N`), structure view (`d`).
- Import / export — streaming SQL dump importer (`I`), `mysqldump`-compatible exporter (`X`), CSV/result export.

**Workflow**

- Vim-mode editor — normal/insert modes, motions, operators, SQL autocompletion.
- Query history & bookmarks — per-connection, persisted, searchable.
- Session restore — reopen a connection to find your tabs and buffers as you left them.
- Record inspector — side panel form view tracking the results cursor.
- Command palette (`Ctrl+P`) and help overlay (`?`).

**Run anywhere** — large result sets are paged for speed and low memory, and a CLI mode runs a query and prints results without the TUI.

See [Features](docs/features.md) for the full detail on the relationship explorer and ERD.

## How creel compares

creel isn't trying to replace a database GUI — it's a fast, keyboard-first terminal tool. A few reference points:

| | creel | dbcli (pgcli / mycli / litecli) | sqlit | usq |
| --- | --- | --- | --- | --- |
| Language | Go — one static binary, no runtime deps | Python | Python | Go |
| Databases | SQLite, MySQL, PostgreSQL | one tool per database | SQLite, MySQL, PostgreSQL, SQL Server, Turso, … | many drivers |
| Interaction | vim modal editor + editable results grid | REPL prompt (optional vi mode) + pager | TUI | prompt-style (psql-like) |
| Schema view | ERD (`g R`) + relationship explorer (`g r`) | — | — | — |

In short: the **dbcli** tools are mature, battle-tested REPLs with excellent autocompletion — reach for them if you want a smart prompt. **sqlit** (which inspired creel) is a polished multi-database TUI in Python/Textual. **usq** is a Go universal CLI spanning many drivers. creel's angle is a **single, dependency-free Go binary** with a **vim-native editor** and **graphical schema exploration** built for browsing and editing relational data quickly.

## Documentation

| Topic | Description |
| --- | --- |
| [Features](docs/features.md) | What creel can do, in depth. |
| [Keybindings](docs/keybindings.md) | Every key in every panel (also `?` in-app). |
| [Configuration](docs/configuration.md) | `config.yaml`, secrets, read-only mode, groups, AI assistant, settings, themes. |
| [CLI mode](docs/cli.md) | Running queries headlessly with flags. |
| [Demo database](demo/README.md) | The bundled e-commerce schema used in the screenshots. |
| [Architecture](docs/architecture.md) | Subsystem map and design decisions. |

## Install

**Homebrew** (tap):

```sh
brew install rsiota/creel/creel
```

**go install**:

```sh
go install github.com/rsiota/creel/cmd/creel@latest
```

**Build from source** (requires Go 1.26+):

```sh
git clone https://github.com/rsiota/creel.git
cd creel
go build -o creel ./cmd/creel/
```

The config lives at `~/.config/creel/config.yaml` and is created on first run.

## Architecture

```
cmd/creel/main.go        Entry point (TUI + CLI modes)
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

For the full subsystem map and design decisions, see [docs/architecture.md](docs/architecture.md).

## Upgrading from gsql

On first launch creel automatically migrates `~/.config/gsql/` to `~/.config/creel/` (connections, history, bookmarks, sessions) and keeps reading keychain secrets stored under the old `gsql` service. See [Configuration](docs/configuration.md) for details.

## Roadmap

See [ROADMAP.md](ROADMAP.md).

## Contributing

Issues and pull requests are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md)
for how to build, test, and add keybindings or database drivers, and
[ROADMAP.md](ROADMAP.md) for the direction of the project. Bug reports and
feature requests use the [issue templates](.github/ISSUE_TEMPLATE). This
project follows the [Contributor Covenant](CODE_OF_CONDUCT.md) code of conduct.

## License

[MIT](LICENSE)
