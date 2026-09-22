# creel demo database

A small, self-contained **SQLite** schema (an e-commerce domain) with a rich
foreign-key graph — purpose-built for trying creel and for taking screenshots.
No server, no credentials, no network.

## Build

```sh
sqlite3 demo/creel-demo.db < demo/schema.sql
```

(`demo/creel-demo.db` is gitignored; rebuild it any time with the line above.)

## Explore

```sh
creel                                  # empty list → Enter on "Try the demo database"
# or, from a clone with a built demo file:
creel -database demo/creel-demo.db
```

From the connection list with no saved connections, select **Try the demo
database** and press Enter — creel uses `./demo/creel-demo.db` when present,
otherwise writes the sample under `~/.config/creel/demo/`. Or add a connection
(`n`):

| Field    | Value                                  |
| -------- | -------------------------------------- |
| Driver   | `sqlite`                               |
| Database | absolute path to `demo/creel-demo.db`  |

### Graph tour (try this)

The demo schema is built for the FK graph. After connect:

1. `j/k` to **users**, `s` (or `:goto users`) — load the grid
2. `l` to focus results if needed, then `g r` — relationship explorer on the row
3. `j` to an inbound edge (e.g. **orders**), `A` — insert related (FK prefilled); `esc` cancels
4. `:explore` (or `esc` with explorer focused) to close, then `g R` / `:erd` — static ERD
5. On the ERD: `p` to anchor a path, move to another card, `p` again to trace, `i` to drop JOIN SQL into the editor

A recorded version of that loop lives at
[`docs/images/demo-graph.gif`](../docs/images/demo-graph.gif) (regenerate with
`./scripts/record-graph-tour.sh` — VHS, GitHub Light, 1400×880).

| Keys   | What you get                                         |
| ------ | ---------------------------------------------------- |
| `g R`  | **Static ERD** — table cards + FK arrows; `zz` fits, mini-map pans |
| `s`    | `SELECT *` from a table into the results grid        |
| `g r`  | **Relationship explorer** — a row's FK graph         |
| `g c`  | Theme picker (live preview)                      |
| `?`    | Full keybinding overlay (Start tab covers this tour) |

### Schema

8 tables, ~85 rows total. `users` is a hub (addresses / orders / reviews
point at it); `products` and `orders` each fan out to two children;
`categories` is self-referential — so the ERD and relationship explorer have
real structure to show.

```
users ─┬─ addresses
       ├─ orders ─┬─ order_items ── products ── categories (self)
       │          └─ payments                  │
       └─ reviews ─────────────────────────────┘
```
