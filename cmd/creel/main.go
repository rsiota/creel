package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/ui"
	"github.com/rsiota/creel/internal/version"
)

func main() {
	var (
		queryFlag    string
		fileFlag     string
		formatFlag   string
		connFlag     string
		driverFlag   string
		databaseFlag string
		hostFlag     string
		portFlag     int
		userFlag     string
		passFlag     string
		cliMode      bool
		readOnlyFlag bool
		versionFlag  bool
	)

	flag.StringVar(&queryFlag, "e", "", "Execute SQL query and exit (CLI mode)")
	flag.StringVar(&fileFlag, "f", "", "Load a .sql file into the editor at startup")
	flag.StringVar(&formatFlag, "format", "tsv", "CLI output format: csv, json, jsonl, md, or tsv")
	flag.StringVar(&connFlag, "c", "", "Saved connection name; opens it in the TUI, or uses it in CLI mode with -e")
	flag.StringVar(&driverFlag, "driver", "sqlite", "Database driver: sqlite, mysql, or postgres")
	flag.StringVar(&databaseFlag, "database", "", "Database (SQLite path or MySQL/Postgres name); opens it in the TUI, or required for CLI -e")
	flag.StringVar(&hostFlag, "host", "localhost", "Database host (MySQL only)")
	flag.IntVar(&portFlag, "port", 3306, "Database port (MySQL or Postgres only)")
	flag.StringVar(&userFlag, "user", "root", "Database username (MySQL only)")
	flag.StringVar(&passFlag, "password", "", "Database password (MySQL only)")
	flag.BoolVar(&cliMode, "cli", false, "Run in CLI mode (non-interactive)")
	flag.BoolVar(&readOnlyFlag, "readonly", false, "Force read-only mode for all connections (reject writes)")
	flag.BoolVar(&versionFlag, "version", false, "Print version information and exit")
	flag.Parse()

	if versionFlag {
		fmt.Println(version.String())
		fmt.Printf("  go:       %s\n", runtime.Version())
		fmt.Printf("  platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return
	}

	// CLI mode: execute query and print results.
	if queryFlag != "" || cliMode {
		setFlags := make(map[string]bool)
		flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
		connCfg, err := buildConnConfig(setFlags, connFlag, driverFlag, databaseFlag, hostFlag, portFlag, userFlag, passFlag, readOnlyFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := runCLI(connCfg, queryFlag, formatFlag); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// TUI mode
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// -database / -c open the workspace directly instead of the connection
	// picker (matches README quickstart: `creel -database demo/….db`).
	var startupConn *db.ConnectionConfig
	if databaseFlag != "" || connFlag != "" {
		setFlags := make(map[string]bool)
		flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
		startupConn, err = buildConnConfig(setFlags, connFlag, driverFlag, databaseFlag, hostFlag, portFlag, userFlag, passFlag, readOnlyFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if err := ui.Run(cfg, readOnlyFlag, fileFlag, startupConn); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// buildConnConfig resolves the connection for CLI mode. A saved connection by
// name (-c) wins and resolves its secrets from the keyring (so connections
// saved in the TUI — including SSH-tunneled ones — work headlessly);
// otherwise the flat driver/host/... flags build an ad-hoc connection. When
// -c is used, any explicitly-set flat flags (tracked in setFlags) override the
// matching field of the saved connection, so e.g. `-c localhost -database x`
// fills in a different database instead of discarding the flag.
func buildConnConfig(setFlags map[string]bool, name, driver, database, host string, port int, user, pass string, readOnly bool) (*db.ConnectionConfig, error) {
	if name != "" {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
		conn, err := ui.ResolveConnection(cfg, name, readOnly)
		if err != nil {
			return nil, err
		}
		applyOverrides(conn, setFlags, driver, database, host, port, user, pass)
		return conn, nil
	}
	if database == "" {
		return nil, fmt.Errorf("database is required (use -database or -c <name>)")
	}
	return &db.ConnectionConfig{
		Driver:   db.Driver(driver),
		Database: database,
		Host:     host,
		Port:     port,
		Username: user,
		Password: pass,
		ReadOnly: readOnly,
	}, nil
}

// applyOverrides fills fields of conn from the flat flags that were explicitly
// set on the command line (present in setFlags). Used so -c <name> composes
// with individual flags instead of replacing them wholesale. ReadOnly is
// handled separately via forceReadOnly in ResolveConnection.
func applyOverrides(conn *db.ConnectionConfig, setFlags map[string]bool, driver, database, host string, port int, user, pass string) {
	if setFlags["driver"] {
		conn.Driver = db.Driver(driver)
	}
	if setFlags["database"] {
		conn.Database = database
	}
	if setFlags["host"] {
		conn.Host = host
	}
	if setFlags["port"] {
		conn.Port = port
	}
	if setFlags["user"] {
		conn.Username = user
	}
	if setFlags["password"] {
		conn.Password = pass
	}
}

func runCLI(connCfg *db.ConnectionConfig, query, format string) error {
	if query == "" {
		return fmt.Errorf("query is required (use -e flag)")
	}

	conn, err := db.New(*connCfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.Connect(); err != nil {
		return err
	}

	result, err := conn.DB().Execute(query)
	if err != nil {
		return err
	}

	cols := make([]string, len(result.Columns))
	for i, c := range result.Columns {
		cols[i] = c.Name
	}

	out, err := ui.Serialize(format, cols, result.Rows)
	if err != nil {
		return err
	}
	fmt.Print(out)

	fmt.Fprintf(os.Stderr, "%s\n", result.Message)
	return nil
}
