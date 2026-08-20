// Package demo embeds the sample e-commerce schema and materializes a SQLite
// database for first-run exploration. The TUI offers it from an empty
// connection list so brew / go-install users can try creel without cloning.
package demo

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "embed"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var SchemaSQL string

const dbFileName = "creel-demo.db"

// ResolvePath returns a usable path to the sample database.
// Preference order:
//  1. ./demo/creel-demo.db relative to the current working directory (dev clone)
//  2. <configDir>/demo/creel-demo.db, created from the embedded schema if missing
func ResolvePath(configDir string) (string, error) {
	if cwd, err := os.Getwd(); err == nil {
		local := filepath.Join(cwd, "demo", dbFileName)
		if st, err := os.Stat(local); err == nil && !st.IsDir() && st.Size() > 0 {
			return local, nil
		}
	}
	return Ensure(configDir)
}

// Ensure materializes <configDir>/demo/creel-demo.db from the embedded schema
// when the file is missing or empty. Existing non-empty files are reused.
func Ensure(configDir string) (string, error) {
	dir := filepath.Join(configDir, "demo")
	path := filepath.Join(dir, dbFileName)
	if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Size() > 0 {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create demo dir: %w", err)
	}
	// Remove a zero-length leftover so Open can recreate cleanly.
	_ = os.Remove(path)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return "", fmt.Errorf("open demo db: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(SchemaSQL); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("apply demo schema: %w", err)
	}
	return path, nil
}
