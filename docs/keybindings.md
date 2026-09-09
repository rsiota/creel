# Keybindings

creel is keyboard-first and vim-driven. Press `?` inside the TUI for the full
overlay, or `Ctrl+P` for the fuzzy jump-anywhere palette — both are generated from a
single keybinding registry (`internal/ui/registry.go`), and a drift-detection
test keeps the documentation in sync with what's actually wired.

## Global

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
| `alt+h/j/k/l`   | Resize focused pane (also `ctrl+alt+…`) |
| `alt+b` / `alt+e` | Toggle sidebar / query editor         |
| `tab` / `shift+tab` | Cycle focus (skips tab bar) |
| `ctrl+d` / `ctrl+u` | Next / previous page        |
| `ctrl+p`        | Jump-anywhere palette           |
| `:`             | Ex command line (`:q`, `:param`, `:goto`, `:sizes`, `:zen`, …) |
| `g c`           | Theme picker (live preview)     |
| `?`             | Toggle help                     |
| `q` / `ctrl+q`  | Quit (not while editing)        |

## Connections

| Key        | Action                          |
| ---------- | ------------------------------- |
| `enter`    | Connect to selected             |
| `[` / `]`  | Prev / next group tab (when grouped) |
| `n`        | New connection                  |
| `e`        | Edit connection                 |
| `d`        | Delete connection               |
| `/`        | Filter connections              |

The picker remembers the last connections you opened (MRU) and selects the most
recent on reopen. Each row shows the connection name with a muted host or path
trailing. When connections use `group`, a tab strip switches between groups
(same `[` / `]` as the form pages). With no saved connections, a **Try the demo
database** row opens the bundled sample schema (Enter) — using
`./demo/creel-demo.db` when present, otherwise materializing one under the
config dir.

In the add/edit form:

| Key        | Action                          |
| ---------- | ------------------------------- |
| `[` / `]`  | Prev / next form page           |
| `tab`      | Next field                      |
| `enter`    | Save                            |
| `ctrl+t`   | Test connection (no save)       |
| `esc`      | Cancel                          |

The form splits fields across pages so the everyday path stays short. For
mysql/postgres each page has six fields: **Connection** (name, driver, host,
user, password, database), **SSH** (tunnel settings — leave SSH Host blank
for no tunnel), and **Options** (port, socket, SSL, secrets, read-only,
group). SQLite uses Connection + Options only.

## Sidebar (Tables)

| Key        | Action                    |
| ---------- | ------------------------- |
| `j/k`      | Move                      |
| `l`        | Focus results             |
| `g g` / `G`| Top / bottom              |
| `space`    | Expand columns            |
| `enter` / `s` | `SELECT *` from table  |
| `d`        | Structure (columns/indexes/FKs/checks/triggers) |
| `a`        | Add column                |
| `r`        | Rename table              |
| `T`        | Truncate table            |
| `D`        | Drop table                |
| `N`        | New table (grid editor)   |
| `X`        | Export database (portable SQL dump) |
| `:backup`  | Size-aware `mysqldump` / `pg_dump` picker (schema/data per table) → `~/Downloads` |
| `:restore[!] <file>` | `mysql` / `psql` CLI load (MySQL/Postgres; SSH OK). `:restore!` continues past SQL errors and opens an error review overlay |
| `:sizes`   | Table row counts and disk sizes (largest first; Enter opens table) |
| `:locks`   | Show lock waiters → blockers (MySQL/Postgres); Enter opens relation |
| `:who`     | List live sessions (MySQL/Postgres); pair with `:kill <pid>` |
| `:kill <pid>` | Terminate a session (confirm; `:kill!` skips; not in read-only) |
| `:diagnose` | Flag seq/full scans and index hints for the editor statement |
| `:zen`     | Results-only layout (`:zen off` restores) |
| `g e` / `:explain` | Raw query plan overlay |
| `I`        | Import SQL dump           |
| `S`        | Cross-table search        |
| `:grep [q]` | Cross-table search (global; optional query) |
| `/`        | Filter tables             |

## Editor (Vim)

| Key          | Action                          |
| ------------ | ------------------------------- |
| `i/a/o/A/O`  | Enter insert mode               |
| `esc`        | Normal mode                     |
| `h/j/k/l`, `w/b` | Move                       |
| `x` / `dd` / `dw` / `D` | Delete              |
| `y` / `p`    | Yank / paste (shared with cell popup)         |
| `yy` / `yw` / `y$` | Yank line / word / to EOL               |
| `u` / `U`    | Undo / redo                                     |
| `/`          | Search in buffer                                |
| `n` / `N`    | Next / previous search match                    |
| `V`          | Visual line (yank / delete)                     |
| `ctrl+n`     | Autocomplete                                    |
| `==`         | Format SQL                                      |

Status bar shows `NORMAL` / `INSERT` / `SEARCH` / `V-LINE` while the editor is focused.

## Results

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
| `g r`      | Relationship explorer (row's FK graph)       |
| `g R`      | Static ERD (table cards + arrows)            |
| `*`        | Keep rows equal to cursor cell               |
| `!`        | Hide rows equal to cursor cell               |
| `g f`      | Filter column values                         |
| `space`    | Toggle row mark                              |
| `M`        | Toggle column mark (for `:bar` / `:line` / `:scatter` / `:hist` / `:freq` / `:pie`) |
| `F`        | Filter by marked rows                        |
| `C`        | Clear marks (rows and columns)               |
| `u`        | Undo last filter                             |
| `c`        | Clear filters                                |
| `V`        | Visual mode (select range; `p` fills column) |
| `dd`       | Delete marked or cursor row                  |
| `e`        | Edit cell                                    |
| `E`        | Expand/view cell (multi-line, vim editing)  |
| `ctrl+s`   | Save edits                                   |
| `A`        | Insert new row                               |
| `D`        | Discard edits                                |
| `H`        | Hide column                                  |
| `g H`      | Show all columns                             |
| `<` / `>`  | Narrow / widen column (or drag header `│`)   |
| `=`        | Reset column width to auto-fit               |
| `v`        | Column visibility overlay                    |
| `:`        | Ex command line (`:q`, `:sort`, `:goto`, …; column jump in results) |
| `y y`      | Copy cell                                    |
| `y r`      | Copy rows as TSV (Sheets/Slack; same as `:copyrow`) |
| `p`        | Paste clipboard; fill marked or visual column |
| `Y`        | Copy rows as INSERT statements               |
| `P`        | Clone marked/cursor row                      |
| `x`        | Export current page to CSV                 |
| `g X`      | Export dialog (format · columns · scope)   |
| `:diff`    | Diff result pages of two tabs (`:diff [a] [b]`) |

## Record inspector (`ctrl+o`)

| Key | Action |
| --- | --- |
| `j/k` | Move field (or tree row when JSON is open) |
| `g` / `G` | Top / bottom (field or JSON tree) |
| `/` | Filter fields |
| `o` / `enter` | Open JSON fold / toggle node under cursor |
| `h` / `l` | Collapse / expand JSON node (←/→ same) |
| `esc` | Collapse JSON tree (or cancel insert) |
| `e` / `i` | Edit field (`E` popup for JSON) |
| `E` | Expand/view field (multi-line) |
| `g d` | Follow foreign key |
| `u` / `g b` | Go back |
| `A` | Insert row |
| `D` | Discard edits |
| `ctrl+s` | Save |

## Chart panel (`:bar` / `:line` / `:pie` / …)

| Key | Action |
| --- | --- |
| `j/k/h/l` | Move |
| `o` | Unfold / fold `(other)` |
| `enter` | Keep rows for this bar / slice |
| `x` | Export Unicode snapshot → `~/Downloads` |
| `X` | Export SVG → `~/Downloads` |
| `:chartexport [txt\|svg]` | Same export with an explicit format |
| `esc` / `q` | Close chart |

## Cell editor (`E`)

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| `i/a/o`    | Insert mode                                     |
| `esc`      | Normal mode / close                             |
| `h/j/k/l`  | Move (normal)                                   |
| `x` / `dd` | Delete (normal)                                 |
| `y` / `p`  | Yank / paste (normal; shared with SQL editor)   |
| `yy` / `yw` / `y$` | Yank line / word / to EOL (normal)      |
| `/`        | Search in buffer (normal and read-only)       |
| `n` / `N`  | Next / prev search match (normal)             |
| `ctrl+s`   | Stage edit & close                              |
| `q`        | Close (normal mode)                             |

## Diff panel (`:diff`)

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| `j/k`      | Scroll                                          |
| `g` / `G`  | Top / bottom                                    |
| `a`        | Toggle changes-only / all rows                  |
| `esc` / `q`| Close                                           |

## Relationship explorer (`g r`)

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| `j/k`      | Move                                            |
| `h/l`      | Collapse / expand                               |
| `enter`    | Re-root this tab on the node                    |
| `t`        | Open the node in a new tab                      |
| `A`        | Insert related row (inbound edge; stays on parent) |
| `u` / `g b`| Go back                                         |
| `r`        | Retarget / refresh                              |
| `esc` / `q`| Close                                           |

## Static ERD (`g R`)

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| `j/k/h/l`  | Move focus between cards                        |
| `H/J/K/L`  | Nudge the focused card                          |
| `space`    | Highlight focused card's relations              |
| `enter`    | Browse the focused table (`SELECT *`)           |
| `f`        | Re-focus on the focused card's neighbourhood    |
| `zz`       | Fit all cards to the viewport                   |
| `zc/zo/za` | Collapse / expand / toggle the focused card     |
| `/`        | Jump to a table by name                         |
| `p`        | Trace the FK path between two tables            |
| `m`        | Toggle Mermaid source                           |
| `y` / `s`  | Copy / save Mermaid source                      |
| `esc` / `q`| Close                                           |

## Backup picker (`:backup`)

Opens before a native `mysqldump` / `pg_dump`. Tables are sorted largest-first
(same size data as `:sizes`). Leaving every table on schema+data keeps a
full-database dump.

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| `j/k`      | Move                                            |
| `g` / `G`  | Top / bottom                                    |
| `space`    | Include / omit table (schema+data)              |
| `s` / `d`  | Toggle schema / data on the cursor row          |
| `a` / `n`  | All schema+data / none                          |
| `o`        | Schema only for every table                     |
| `enter`    | Run backup                                      |
| `esc`      | Cancel                                          |

## Lookup panel (`:sizes`, `:locks`, `:who`, `:diagnose`, …)

Shared overlay for catalog and ops lookups. Enter jumps when a table target
is available.

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| `j/k`      | Move                                            |
| `g` / `G`  | Top / bottom                                    |
| `ctrl+d` / `ctrl+u` | Page down / up                           |
| `enter`    | Open table (when jumpable)                      |
| `esc`      | Close                                           |

## Filter picker (`g f`)

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| type       | Fuzzy-filter values                             |
| `j/k`      | Move                                            |
| `space`    | Toggle value                                    |
| `a` / `n`  | Select all / none                               |
| `enter`    | Apply filter                                    |
| `esc`      | Cancel                                          |

## Column visibility (`v`)

| Key        | Action                                          |
| ---------- | ----------------------------------------------- |
| type       | Fuzzy-filter columns                            |
| `j/k`      | Move                                            |
| `space`    | Toggle visibility                               |
| `a` / `n`  | Show all / hide all (keeps one)                 |
| `enter`    | Apply                                           |
| `esc`      | Cancel                                          |
