package db

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// runMysqlDumpHelp runs `bin --help`; tests replace it.
var runMysqlDumpHelp = func(bin string) ([]byte, error) {
	return exec.Command(bin, "--help").CombinedOutput()
}

var columnStatisticsSupport sync.Map // bin path → bool

// mysqlDumpSupportsColumnStatistics reports whether bin accepts
// --column-statistics (MySQL 8+). MariaDB's mysqldump does not; passing the
// flag there errors. MySQL 8 defaults it on, which breaks dumps against
// MariaDB / older MySQL (Unknown table COLUMN_STATISTICS).
func mysqlDumpSupportsColumnStatistics(bin string) bool {
	if v, ok := columnStatisticsSupport.Load(bin); ok {
		return v.(bool)
	}
	out, err := runMysqlDumpHelp(bin)
	supported := err == nil && strings.Contains(string(out), "column-statistics")
	columnStatisticsSupport.Store(bin, supported)
	return supported
}

// SwapRunMysqlDumpHelp replaces --help probing; the returned func restores it.
func SwapRunMysqlDumpHelp(fn func(string) ([]byte, error)) func() {
	prev := runMysqlDumpHelp
	runMysqlDumpHelp = fn
	columnStatisticsSupport = sync.Map{}
	return func() {
		runMysqlDumpHelp = prev
		columnStatisticsSupport = sync.Map{}
	}
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
// SSH-tunneled connections are allowed: the caller must open a localhost
// forward through the live tunnel before invoking mysqldump.
func MysqlDumpGuard(cfg ConnectionConfig) error {
	if cfg.Driver != DriverMySQL {
		return fmt.Errorf(":backup uses mysqldump (MySQL/MariaDB); use X for a SQL dump")
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("no database selected")
	}
	return nil
}

// MysqlDumpDefaults is a my.cnf snippet with client user/password so the
// password never appears on argv. Caller writes it to a 0600 temp file.
// Do not put net_read_timeout / net_write_timeout here: mysqldump treats them
// as unknown variables in the option file (even though the mysql CLI accepts
// them). Long dumps rely on OpenSSH -L, --compress, and half-close proxying.
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
// throughSSH adds protocol compression to shrink bulk result traffic.
func BuildMysqlDumpArgs(cfg ConnectionConfig, defaultsFile, resultFile string, throughSSH bool) []string {
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
		"--max-allowed-packet=1073741824",
	)
	if throughSSH {
		args = append(args, "--compress")
	}
	args = append(args,
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

// mysqlDumpConfigViaForward rewrites cfg so mysqldump dials the localhost
// proxy. Drop SSH/socket; force ssl-mode=disable because TLS hostname checks
// cannot use 127.0.0.1 and the SSH hop already encrypts the path to the bastion
// (same pattern as most GUI clients over SSH).
func mysqlDumpConfigViaForward(cfg ConnectionConfig, fwd *LocalForward) ConnectionConfig {
	out := cfg
	out.SSHHost = ""
	out.Socket = ""
	out.Host = fwd.Host
	out.Port = fwd.Port
	out.SSLMode = "disable"
	return out
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
//
// SSH + MySQL on the SSH host (localhost/127.0.0.1): run mysqldump on the
// remote machine and stream stdout back — same path as a manual
// `ssh host mysqldump … > dump.sql`, which avoids truncated dumps through a
// localhost forward. Falls back to local mysqldump + port forward when the
// remote binary is missing or MySQL is on a different internal host.
func RunMysqlDump(bin string, cfg ConnectionConfig, resultFile string, conn *Connection) error {
	if err := MysqlDumpGuard(cfg); err != nil {
		return err
	}

	dir := filepath.Dir(resultFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	throughSSH := strings.TrimSpace(cfg.SSHHost) != ""
	if throughSSH && MysqlHostOnSSHTarget(cfg.Host) && conn != nil {
		if err := conn.runRemoteMysqlDump(resultFile); err == nil {
			return nil
		} else if !remoteDumpUnavailable(err) {
			return err
		}
		// Remote has no mysqldump — try local binary + forward below.
	}

	if bin == "" {
		var err error
		bin, err = FindMysqlDump()
		if err != nil {
			return fmt.Errorf("mysqldump is not on PATH")
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
		dumpCfg = mysqlDumpConfigViaForward(cfg, fwd)
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
	if _, err := tmp.WriteString(MysqlDumpDefaults(dumpCfg)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	args := BuildMysqlDumpArgs(dumpCfg, defaultsPath, resultFile, throughSSH)
	if mysqlDumpSupportsColumnStatistics(bin) {
		// Keep defaults-extra-file first; insert compatibility flag next.
		args = append([]string{args[0], "--column-statistics=0"}, args[1:]...)
	}
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
