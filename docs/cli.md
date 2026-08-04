# CLI mode

As well as the interactive TUI, creel can run a single query and print the
results without launching the interface — handy for scripting, piping, or
quick one-offs. Output is TSV.

## Usage

```sh
creel -e "SELECT * FROM users" -database /tmp/test.db
creel -e "SHOW TABLES" -driver mysql -database myapp -host 10.0.0.5 -user admin -password secret
```

## Flags

| Flag          | Description                                              | Default     |
| ------------- | -------------------------------------------------------- | ----------- |
| `-e`          | SQL query to execute (enables CLI mode)                  |             |
| `-f`          | Load a `.sql` file into the editor at startup (TUI)      |             |
| `-driver`     | `sqlite`, `mysql`, or `postgres`                         | `sqlite`    |
| `-database`   | Database name (SQLite path or MySQL/Postgres database)   |             |
| `-host`       | Database host (MySQL/Postgres)                           | `localhost` |
| `-port`       | Database port (MySQL/Postgres)                           | `3306`      |
| `-user`       | Username (MySQL/Postgres)                                | `root`      |
| `-password`   | Password (MySQL/Postgres)                                |             |
| `-cli`        | Force CLI mode                                           | `false`     |
| `-readonly`   | Force read-only mode for all connections (reject writes) | `false`     |
| `-version`    | Print version information and exit                       |             |

`-port` defaults to MySQL's `3306`; pass `-port 5432` for PostgreSQL. See
[Configuration](configuration.md#read-only-mode) for the per-connection
`readonly: true` alternative to `--readonly`.
