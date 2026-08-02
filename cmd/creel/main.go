package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/ui"
)

func main() {
	var (
		queryFlag    string
		fileFlag     string
		driverFlag   string
		databaseFlag string
		hostFlag     string
		portFlag     int
		userFlag     string
		passFlag     string
		cliMode      bool
		readOnlyFlag bool
	)

	flag.StringVar(&queryFlag, "e", "", "Execute SQL query and exit (CLI mode)")
	flag.StringVar(&fileFlag, "f", "", "Load a .sql file into the editor at startup")
	flag.StringVar(&driverFlag, "driver", "sqlite", "Database driver: sqlite, mysql, or postgres")
	flag.StringVar(&databaseFlag, "database", "", "Database name (SQLite path or MySQL database)")
	flag.StringVar(&hostFlag, "host", "localhost", "Database host (MySQL only)")
	flag.IntVar(&portFlag, "port", 3306, "Database port (MySQL or Postgres only)")
	flag.StringVar(&userFlag, "user", "root", "Database username (MySQL only)")
	flag.StringVar(&passFlag, "password", "", "Database password (MySQL only)")
	flag.BoolVar(&cliMode, "cli", false, "Run in CLI mode (non-interactive)")
	flag.BoolVar(&readOnlyFlag, "readonly", false, "Force read-only mode for all connections (reject writes)")
	flag.Parse()

	// CLI mode: execute query and print results
	if queryFlag != "" || cliMode {
		if err := runCLI(queryFlag, driverFlag, databaseFlag, hostFlag, portFlag, userFlag, passFlag, readOnlyFlag); err != nil {
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

	if err := ui.Run(cfg, readOnlyFlag, fileFlag); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(query, driver, database, host string, port int, user, pass string, readOnly bool) error {
	if database == "" {
		return fmt.Errorf("database is required (use -database flag)")
	}
	if query == "" {
		return fmt.Errorf("query is required (use -e flag)")
	}

	connCfg := db.ConnectionConfig{
		Driver:   db.Driver(driver),
		Database: database,
		Host:     host,
		Port:     port,
		Username: user,
		Password: pass,
		ReadOnly: readOnly,
	}

	conn, err := db.New(connCfg)
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

	for _, col := range result.Columns {
		fmt.Printf("%s\t", col.Name)
	}
	fmt.Println()

	for _, row := range result.Rows {
		for _, val := range row {
			fmt.Printf("%s\t", val)
		}
		fmt.Println()
	}

	fmt.Fprintf(os.Stderr, "%s\n", result.Message)
	return nil
}
