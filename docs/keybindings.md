# Keybindings

creel is keyboard-first and vim-driven. Press `?` inside the TUI for the full
overlay, or `Ctrl+P` for the fuzzy command palette — both are generated from a
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
| `tab` / `shift+tab` | Cycle focus (skips tab bar) |
| `ctrl+d` / `ctrl+u` | Next / previous page        |
| `ctrl+p`        | Command palette                 |
| `:`             | Ex command line (`:q`, `:sort`, `:goto`, …) |
| `g c`           | Theme picker (live preview)     |
| `?`             | Toggle help                     |
| `q` / `ctrl+q`  | Quit (not while editing)        |

## Connections

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
| `X`        | Export database            |
| `I`        | Import SQL dump           |
| `S`        | Cross-table search        |
| `/`        | Filter tables             |

## Editor (Vim)

| Key          | Action                          |
| ------------ | ------------------------------- |
| `i/a/o/A/O`  | Enter insert mode               |
| `esc`        | Normal mode                     |
| `h/j/k/l`, `w/b` | Move                       |
| `x` / `dd` / `dw` / `D` | Delete              |
| `y` / `p`    | Yank / paste                    |
| `ctrl+n`     | Autocomplete                    |
| `==`         | Format SQL                      |

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
| `M`        | Toggle column mark (for `:bar` / `:line` / `:hist` / `:freq`) |
| `F`        | Filter by marked rows                        |
| `C`        | Clear marks (rows and columns)               |
| `u`        | Undo last filter                             |
| `c`        | Clear filters                                |
| `V`        | Visual mode (select range)                   |
| `dd`       | Delete marked or cursor row                  |
| `e`        | Edit cell                                    |
| `E`        | Expand/view cell (multi-line)               |
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
| `x`        | Export current page to CSV                 |
| `g X`      | Export dialog (format · columns · scope)   |
