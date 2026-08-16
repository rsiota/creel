# CLI mode

As well as the interactive TUI, creel can run a single query and print the
results without launching the interface — handy for scripting, piping, or
quick one-offs. Output goes to **stdout** (so it pipes cleanly); the row-count
summary (`N rows in Xs`) goes to **stderr** so it never contaminates the data.

By default output is **TSV**; use `-format` to pick `csv`, `json`, `jsonl`,
`md`, or `tsv`.

## Usage

Open a SQLite file directly in the TUI (skips the connection picker):

```sh
creel -database /tmp/test.db
creel -database demo/blob-demo.db
creel -c staging          # saved connection by name
```

Ad-hoc connection from flat flags (CLI / one-shot query):

```sh
creel -e "SELECT * FROM users" -database /tmp/test.db
creel -e "SHOW TABLES" -driver mysql -database myapp -host 10.0.0.5 -user admin -password secret
creel -e "SELECT id, name FROM users" -database /tmp/test.db -format csv > users.csv
```

A saved connection by name (`-c`) — the recommended scripting form, since it
resolves the password / SSH passphrase from the keychain exactly as the TUI
does on connect, so connections saved interactively (including SSH-tunneled
ones) work headlessly:

```sh
creel -c prod -e "SELECT count(*) FROM orders" -format json
creel -c staging -e "SELECT * FROM users WHERE created_at > now() - interval '1 day'" -readonly
creel -c localhost -database local_turniq -e "SELECT * FROM users"   # -database fills an empty/default one
```

## Flags

| Flag          | Description                                                          | Default     |
| ------------- | -------------------------------------------------------------------- | ----------- |
| `-e`          | SQL query to execute (enables CLI mode)                              |             |
| `-f`          | Load a `.sql` file into the editor at startup (TUI)                  |             |
| `-c`          | Saved connection name; opens it in the TUI, or uses it with `-e`     |             |
| `-format`     | CLI output format: `csv`, `json`, `jsonl`, `md`, or `tsv`            | `tsv`       |
| `-driver`     | `sqlite`, `mysql`, or `postgres`                                     | `sqlite`    |
| `-database`   | SQLite path or MySQL/Postgres database; opens the TUI workspace when used without `-e` |             |
| `-host`       | Database host (MySQL/Postgres)                                       | `localhost` |
| `-port`       | Database port (MySQL/Postgres)                                       | `3306`      |
| `-user`       | Username (MySQL/Postgres)                                            | `root`      |
| `-password`   | Password (MySQL/Postgres)                                            |             |
| `-sslmode`    | TLS policy: `disable`, `prefer`, `require`, `verify-ca`, `verify-full` | `prefer` (when empty) |
| `-socket`     | Unix socket path (MySQL/Postgres); overrides `-host`                 |             |
| `-cli`        | Force CLI mode                                                       | `false`     |
| `-readonly`   | Force read-only mode for all connections (reject writes)             | `false`     |
| `-version`    | Print version information and exit                                   |             |

`-port` defaults to MySQL's `3306`; pass `-port 5432` for PostgreSQL. With
`-c`, the saved connection is loaded and its secrets resolved, then any
**explicitly-set** `-driver`/`-database`/`-host`/`-port`/`-user`/`-password`/
`-sslmode`/`-socket` flags override the matching fields — handy when a saved
connection has no default `database`, or you want to point at a different DB
on the same server.
Combine `-c` with `-readonly` to force a saved connection read-only for a
one-off. See [Configuration](configuration.md#read-only-mode) for the
per-connection `readonly: true` alternative to `--readonly`.

### Format notes

- **`csv`** quotes fields as needed (RFC 4180), so multi-line text values stay
  within their quoted field.
- **`json`** is a pretty-printed array of objects keyed by column name.
- **`jsonl`** is one compact JSON object per line (handy for streaming).
- **`md`** is a GitHub-flavoured Markdown table.
- **`tsv`** (default) is tab-separated with a header row; SQL `NULL` renders as
  an empty field (CSV/TSV) or JSON `null` (json/jsonl).
