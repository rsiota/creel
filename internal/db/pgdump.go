package db

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// lookPathPgDump is exec.LookPath for "pg_dump"; tests replace it.
var lookPathPgDump = exec.LookPath

// runPgDumpCmd runs a prepared pg_dump command; tests replace it.
var runPgDumpCmd = func(cmd *exec.Cmd) error { return cmd.Run() }

// SwapLookPathPgDump replaces PATH lookup; the returned func restores it.
func SwapLookPathPgDump(fn func(string) (string, error)) func() {
	prev := lookPathPgDump
	lookPathPgDump = fn
	return func() { lookPathPgDump = prev }
}

// SwapRunPgDumpCmd replaces command execution; the returned func restores it.
func SwapRunPgDumpCmd(fn func(*exec.Cmd) error) func() {
	prev := runPgDumpCmd
	runPgDumpCmd = fn
	return func() { runPgDumpCmd = prev }
}

// FindPgDump returns the pg_dump binary on PATH, or an error if missing.
func FindPgDump() (string, error) {
	p, err := lookPathPgDump("pg_dump")
	if err != nil {
		return "", err
	}
	return p, nil
}

// PgDumpGuard reports why cfg cannot be backed up with pg_dump, or nil.
func PgDumpGuard(cfg ConnectionConfig) error {
	if cfg.Driver != DriverPostgres {
		return fmt.Errorf(":backup uses pg_dump (PostgreSQL); use X for a SQL dump")
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("no database selected")
	}
	return nil
}

// escapePgPassField escapes \ and : for a .pgpass field.
func escapePgPassField(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `:`, `\:`)
	return s
}

// PgPassLine builds one .pgpass entry for cfg (hostname:port:db:user:password).
func PgPassLine(cfg ConnectionConfig) string {
	host := cfg.Host
	if sock := cfg.socketPath(); sock != "" {
		host = sock
	} else if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	dbName := cfg.Database
	if dbName == "" {
		dbName = "*"
	}
	user := cfg.Username
	if user == "" {
		user = "*"
	}
	return fmt.Sprintf("%s:%d:%s:%s:%s",
		escapePgPassField(host),
		port,
		escapePgPassField(dbName),
		escapePgPassField(user),
		escapePgPassField(cfg.Password),
	)
}

// writePgPassFile writes a 0600 temp .pgpass for cfg and returns its path.
func writePgPassFile(cfg ConnectionConfig) (string, error) {
	tmp, err := os.CreateTemp("", "creel-pgpass-*")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(path)
		return "", err
	}
	if _, err := tmp.WriteString(PgPassLine(cfg) + "\n"); err != nil {
		tmp.Close()
		os.Remove(path)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// pgClientEnv sets PGPASSFILE / PGSSLMODE for libpq tools (never puts the
// password on argv).
func pgClientEnv(cfg ConnectionConfig, passFile string) []string {
	var out []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "PGPASSWORD=") ||
			strings.HasPrefix(e, "PGPASSFILE=") ||
			strings.HasPrefix(e, "PGSSLMODE=") {
			continue
		}
		out = append(out, e)
	}
	out = append(out, "PGPASSFILE="+passFile)
	if mode := cfg.sslMode(); mode != "" {
		out = append(out, "PGSSLMODE="+mode)
	}
	return out
}

// PgDumpOptions controls selective pg_dump invocation.
type PgDumpOptions struct {
	Tables       []string // empty = whole database; each becomes --table=
	SchemaOnly   bool
	DataOnly     bool
}

// BuildPgDumpArgs is the pg_dump argv after the binary. Password is never
// included (use PGPASSFILE). Output goes to stdout for progress counting.
func BuildPgDumpArgs(cfg ConnectionConfig, opt PgDumpOptions) []string {
	args := []string{"--no-password", "--format=plain"}
	if sock := cfg.socketPath(); sock != "" {
		args = append(args, "--host="+sock)
	} else {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 5432
		}
		args = append(args, "--host="+host, fmt.Sprintf("--port=%d", port))
	}
	if cfg.Username != "" {
		args = append(args, "--username="+cfg.Username)
	}
	if opt.SchemaOnly {
		args = append(args, "--schema-only")
	}
	if opt.DataOnly {
		args = append(args, "--data-only")
	}
	for _, t := range opt.Tables {
		args = append(args, "--table="+t)
	}
	args = append(args, "--dbname="+cfg.Database)
	return args
}

// pgDumpConfigViaForward rewrites cfg so pg_dump dials the localhost proxy.
// Drop SSH/socket; force sslmode=disable because TLS hostname checks cannot
// use 127.0.0.1 and the SSH hop already encrypts the path to the bastion.
func pgDumpConfigViaForward(cfg ConnectionConfig, fwd *LocalForward) ConnectionConfig {
	out := cfg
	out.SSHHost = ""
	out.Socket = ""
	out.Host = fwd.Host
	out.Port = fwd.Port
	out.SSLMode = "disable"
	return out
}

// RunPgDump writes cfg's database to resultFile using pg_dump.
//
// plan selects tables and schema/data. A full plan dumps the whole database.
// A selective plan may run pg_dump twice (schema pass, then data pass).
//
// SSH + Postgres on the SSH host (localhost/127.0.0.1): run pg_dump on the
// remote machine and stream stdout back. Falls back to local pg_dump + port
// forward when the remote binary is missing or Postgres is on another host.
//
// onBytes is called with the cumulative byte count as the dump streams (may
// be nil).
func RunPgDump(bin string, cfg ConnectionConfig, resultFile string, conn *Connection, plan DumpPlan, onBytes func(int64)) error {
	if err := PgDumpGuard(cfg); err != nil {
		return err
	}
	if !plan.HasContent() {
		return fmt.Errorf("nothing selected to backup")
	}

	dir := filepath.Dir(resultFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	throughSSH := strings.TrimSpace(cfg.SSHHost) != ""
	if throughSSH && MysqlHostOnSSHTarget(cfg.Host) && conn != nil {
		if err := conn.runRemotePgDump(resultFile, plan, onBytes); err == nil {
			return nil
		} else if !remotePgDumpUnavailable(err) {
			return err
		}
	}

	if bin == "" {
		var err error
		bin, err = FindPgDump()
		if err != nil {
			return fmt.Errorf("pg_dump is not on PATH")
		}
	}

	dumpCfg := cfg
	if throughSSH {
		if conn == nil {
			return fmt.Errorf(":backup needs an active SSH connection")
		}
		fwd, err := conn.startMysqlDumpForward()
		if err != nil {
			return err
		}
		defer fwd.Close()
		dumpCfg = pgDumpConfigViaForward(cfg, fwd)
	}

	passFile, err := writePgPassFile(dumpCfg)
	if err != nil {
		return err
	}
	defer os.Remove(passFile)

	out, err := os.Create(resultFile)
	if err != nil {
		return err
	}
	defer out.Close()

	cw := &countingWriter{w: out, onBytes: onBytes}
	for _, opt := range pgDumpPasses(plan) {
		if err := runPgDumpOnce(bin, dumpCfg, passFile, opt, cw); err != nil {
			return err
		}
	}
	return nil
}

// pgDumpPasses builds one or two pg_dump invocations for plan.
func pgDumpPasses(plan DumpPlan) []PgDumpOptions {
	if plan.IsFull() {
		return []PgDumpOptions{{}}
	}
	schema := plan.SchemaTables()
	data := plan.DataTables()
	var passes []PgDumpOptions
	if len(schema) > 0 {
		passes = append(passes, PgDumpOptions{Tables: schema, SchemaOnly: true})
	}
	if len(data) > 0 {
		passes = append(passes, PgDumpOptions{Tables: data, DataOnly: true})
	}
	return passes
}

func runPgDumpOnce(bin string, cfg ConnectionConfig, passFile string, opt PgDumpOptions, stdout io.Writer) error {
	args := BuildPgDumpArgs(cfg, opt)
	cmd := exec.Command(bin, args...)
	cmd.Env = pgClientEnv(cfg, passFile)
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := runPgDumpCmd(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("pg_dump: %w", err)
		}
		return fmt.Errorf("pg_dump: %s", msg)
	}
	return nil
}
