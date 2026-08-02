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
./creel
```

Add a connection from the connection list (`n`), then:

| Field    | Value                                  |
| -------- | -------------------------------------- |
| Driver   | `sqlite`                               |
| Database | absolute path to `demo/creel-demo.db`  |

Connect, then try the showcase features:

| Keys   | What you get                                         |
| ------ | ---------------------------------------------------- |
| `g R`  | **Static ERD** — table cards + FK arrows; `zz` fits  |
| `s`    | `SELECT *` from a table into the results grid        |
| `g r`  | **Relationship explorer** — a row's FK graph         |
| `g c`  | Theme picker (live preview)                          |
| `?`    | Full keybinding overlay                              |

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
