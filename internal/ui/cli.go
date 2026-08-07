package ui

import (
	"fmt"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

// This file is the exported surface for the headless CLI (cmd/creel). It wraps
// the existing (unexported) serializer and connection-resolution paths so the
// CLI can reuse them without duplicating logic or rippling through the UI's
// internal call sites.

// Serialize renders result columns and rows in the named format, shared between
// the export UI and the CLI headless path (cmd/creel -format). Supported
// formats: csv, json, jsonl, md, tsv.
func Serialize(format string, cols []string, rows [][]string) (string, error) {
	f, ok := parseExportFormat(format)
	if !ok {
		return "", fmt.Errorf("unsupported format %q (want one of csv, json, jsonl, md, tsv)", format)
	}
	return serializeFormat(f, cols, rows)
}

// ResolveConnection loads a saved connection by name from config, converts it
// to a db.ConnectionConfig, and resolves its secrets (password / SSH
// passphrase) from the keyring — the same path the connection form uses on
// connect. Exported for the CLI headless path (cmd/creel -c <name>). Returns an
// error if no saved connection matches name.
func ResolveConnection(cfg *config.Config, name string, forceReadOnly bool) (*db.ConnectionConfig, error) {
	saved := cfg.GetConnection(name)
	if saved == nil {
		return nil, fmt.Errorf("no saved connection named %q", name)
	}
	dbCfg := connConfigToDB(*saved, forceReadOnly)
	resolved, err := resolveConnSecrets(dbCfg)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}
