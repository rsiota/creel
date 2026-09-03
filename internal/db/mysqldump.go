package db

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// lookPathMysqlDump is exec.LookPath for "mysqldump"; tests replace it.
var lookPathMysqlDump = exec.LookPath

// runMysqlDumpCmd runs a prepared mysqldump command; tests replace it.
var runMysqlDumpCmd = func(cmd *exec.Cmd) error { return cmd.Run() }

// SwapLookPathMysqlDump replaces PATH lookup; the returned func restores it.
func SwapLookPathMysqlDump(fn func(string) (string, error)) func() {
	prev := lookPathMysqlDump
	lookPathMysqlDump = fn
	return func() { lookPathMysqlDump = prev }
}

// SwapRunMysqlDumpCmd replaces command execution; the returned func restores it.
func SwapRunMysqlDumpCmd(fn func(*exec.Cmd) error) func() {
	prev := runMysqlDumpCmd
	runMysqlDumpCmd = fn
	return func() { runMysqlDumpCmd = prev }
}

// FindMysqlDump returns the mysqldump binary on PATH, or an error if missing.
func FindMysqlDump() (string, error) {
	p, err := lookPathMysqlDump("mysqldump")
	if err != nil {
		return "", err
	}
	return p, nil
}

// MysqlDumpGuard reports why cfg cannot be backed up with mysqldump, or nil.
func MysqlDumpGuard(cfg ConnectionConfig) error {
	if cfg.Driver != DriverMySQL {
		return fmt.Errorf(":backup uses mysqldump (MySQL/MariaDB); use X for a SQL dump")
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("no database selected")
	}
	if strings.TrimSpace(cfg.SSHHost) != "" {
		return fmt.Errorf(":backup cannot use Creel's SSH tunnel; dump from a host that can reach MySQL, or use X")
	}
	return nil
}

// MysqlDumpDefaults is a my.cnf snippet with client user/password so the
// password never appears on argv. Caller writes it to a 0600 temp file.
func MysqlDumpDefaults(cfg ConnectionConfig) string {
	var b strings.Builder
	b.WriteString("[client]\n")
	if cfg.Username != "" {
		fmt.Fprintf(&b, "user=%s\n", optionFileValue(cfg.Username))
	}
	if cfg.Password != "" {
		fmt.Fprintf(&b, "password=%s\n", optionFileValue(cfg.Password))
	}
	return b.String()
}

// BuildMysqlDumpArgs is the mysqldump argv after the binary. defaultsFile must
// be the first option (--defaults-extra-file). Password is never included.
func BuildMysqlDumpArgs(cfg ConnectionConfig, defaultsFile, resultFile string) []string {
	args := []string{"--defaults-extra-file=" + defaultsFile}
	if sock := cfg.socketPath(); sock != "" {
		args = append(args, "--socket="+sock)
	} else {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 3306
		}
		args = append(args,
			"--protocol=TCP",
			"--host="+host,
			fmt.Sprintf("--port=%d", port),
		)
	}
	if ssl := mysqlDumpSSLMode(cfg.SSLMode); ssl != "" {
		args = append(args, "--ssl-mode="+ssl)
	}
	args = append(args,
		"--single-transaction",
		"--routines",
		"--events",
		"--result-file="+resultFile,
		cfg.Database,
	)
	return args
}

func mysqlDumpSSLMode(sslmode string) string {
	switch NormalizeSSLMode(sslmode) {
	case "disable":
		return "DISABLED"
	case "require":
		return "REQUIRED"
	case "verify-ca":
		return "VERIFY_CA"
	case "verify-full":
		return "VERIFY_IDENTITY"
	default:
		return "" // prefer/allow: client default
	}
}

func optionFileValue(s string) string {
	if !strings.ContainsAny(s, " \t#\"'=;\n") {
		return s
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// RunMysqlDump writes cfg's database to resultFile using mysqldump.
func RunMysqlDump(bin string, cfg ConnectionConfig, resultFile string) error {
	if err := MysqlDumpGuard(cfg); err != nil {
		return err
	}
	if bin == "" {
		var err error
		bin, err = FindMysqlDump()
		if err != nil {
			return fmt.Errorf("mysqldump is not on PATH")
		}
	}
	dir := filepath.Dir(resultFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "creel-mysqldump-*.cnf")
	if err != nil {
		return err
	}
	defaultsPath := tmp.Name()
	defer os.Remove(defaultsPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(MysqlDumpDefaults(cfg)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	args := BuildMysqlDumpArgs(cfg, defaultsPath, resultFile)
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := runMysqlDumpCmd(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("mysqldump: %w", err)
		}
		return fmt.Errorf("mysqldump: %s", msg)
	}
	return nil
}
