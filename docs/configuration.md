# Configuration

creel is configured through a single YAML file at
`~/.config/creel/config.yaml`. On first launch with no config, the connection
list offers **Try the demo database** (Enter) so you can explore immediately;
press `n` to add a saved connection and creel writes the config for you.

The connection picker also keeps an MRU list in `~/.config/creel/recent.json`
(most recently opened connection names) so reopening creel lands on the last
used entry.

> **Upgrading from gsql?** On first launch creel automatically moves
> `~/.config/gsql/` to `~/.config/creel/` (connections, history, bookmarks,
> sessions) and keeps reading any keychain secrets stored under the old
> `gsql` service, so nothing needs to be re-entered. New secrets are written
> under the `creel` service going forward.

## Connections

Secret fields (passwords) default to the OS keychain; in that case the config
holds an opaque reference instead of the real password:

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
    sslmode: prefer                       # disable | prefer | require | verify-full
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

### TLS and unix sockets

MySQL and PostgreSQL connections accept `sslmode` (libpq names: `disable`,
`prefer`, `require`, `verify-ca`, `verify-full`) and `socket` (a unix-domain
socket path). Empty `sslmode` means `prefer`, so existing configs reach
TLS-required cloud hosts without a rewrite. A `host` that starts with `/` is
also treated as a socket. SSH tunnels always use TCP, so `socket` is ignored
when a tunnel is set.

```yaml
  - name: local-pg
    driver: postgres
    database: app
    socket: /var/run/postgresql
    sslmode: disable
```

### Secret storage

Passwords and SSH passwords are stored in the OS keychain (macOS Keychain,
Windows Credential Manager, Linux Secret Service) rather than plaintext config,
falling back to plaintext on systems without a keychain.

### Read-only mode

Point creel at production safely by marking a connection `readonly: true` or
launching with `creel --readonly` to force every connection read-only. In
either case writes are refused — `Exec`/`Execute` return an error for
INSERT/UPDATE/DELETE/DDL, `Begin`/`Session` (imports) are blocked — and the
connection is opened read-only at the engine level where the driver supports it
(SQLite `query_only`, Postgres `default_transaction_read_only`). A `READ-ONLY`
badge appears in the status bar.

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
folders:

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

## AI assistant

The assistant panel (and the `:ai` command) turn a natural-language question
into SQL using any OpenAI-compatible endpoint (OpenAI, z.ai, OpenRouter,
Ollama, LM Studio, …). The schema sent to the model is the current results
(or sidebar) table plus its foreign-key neighbours, and any other tables
named in the question — not the first 100 tables of the database. After a
query fails, `:aifix` (alias `:fixsql`) sends the failed statement and driver
error to the same provider and drops a corrected candidate into the editor
for review — it never auto-runs. `:aiexplain` (alias `:why`) explains the
statement under the cursor (or the last explained SQL), attaching the
EXPLAIN / EXPLAIN QUERY PLAN output, and streams a prose reply into the
assistant panel — also never auto-run. Optional focus text narrows the
question (`:aiexplain why is the join slow`).

Configure it **in-app** from the assistant panel:

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

With no providers configured, creel falls back to environment variables
(`CREEL_AI_API_KEY` / `OPENAI_API_KEY` / `ZAI_API_KEY`, and optionally
`CREEL_AI_BASE_URL` / `CREEL_AI_MODEL`), so the panel works with zero config.
The deprecated `GSQL_AI_*` equivalents are still honoured (in lower priority)
for users upgrading from gsql.

## Settings

Top-level app preferences live under a `settings:` block. All fields are
optional and fall back to defaults when omitted:

| Key              | Default      | Description                                                              |
| ---------------- | ------------ | ------------------------------------------------------------------------ |
| `page_size`      | 200          | Rows fetched per page in results                                         |
| `query_timeout`  | 30s          | Per-query deadline (friendly form: `2m`, `1h30m`, or a bare seconds int). `off` / `none` (or a negative value) disables the deadline entirely — `esc` still cancels |
| `default_driver` | sqlite       | Driver pre-filled in the add-connection form                             |
| `theme`          | tokyo-night  | Palette: `tokyo-night`, `gruvbox`, `nord`, `catppuccin`, `light` + ~565 auto-derived from iTerm2-Color-Schemes (dracula, solarized, …). Unknown → default |
| `icons`          | unicode      | Glyph set for tree expand/collapse markers (sidebar, connection groups, relationship explorer). `unicode` uses portable triangles (▾/▸); `nerdfont` uses Nerd Font angle chevrons (U+F107/U+F105) — open, rotationally-symmetric like treemacs, but only renders correctly in a terminal running a Nerd Font. Unknown → default |
| `transparent_background` | false | By default creel fills the app background with the theme's bg colour (required for light themes to be readable). Set `true` to leave it unpainted so the terminal's own background / transparency shows through — at the cost of light themes looking wrong. |
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

### Themes

To experiment with themes live, press `g c` in the workspace to open the
theme picker (a scrollable, filterable overlay — type to filter by name,
`↑`/`↓` to preview); moving the selection re-themes the UI immediately,
`enter` saves the choice to config, and `esc` reverts. The theme's background
is painted too, so light themes preview correctly. See [`THIRDPARTY.md`](../THIRDPARTY.md)
for theme attribution.

You can also change most settings from inside the app with `:set`:

```
:set transparent_background on
:set page_size 500
:set query_timeout 2m
:set confirm_destructive off
```

Bare `:set` lists current values; `:set <option>` shows one. Changes apply
immediately and are written back to `config.yaml` (same persistence as `:theme`
and `:icons`).
