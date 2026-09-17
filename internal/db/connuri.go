package db

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// LooksLikeConnectionURI reports whether s is likely a postgres/mysql/sqlite
// connection URL (scheme://…), suitable for paste detection in the form.
func LooksLikeConnectionURI(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	lower := strings.ToLower(s)
	for _, prefix := range []string{
		"postgres://", "postgresql://",
		"mysql://", "mariadb://",
		"sqlite://", "file:",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// ParseConnectionURI parses a postgres://, mysql://, or sqlite/file: URL into
// a ConnectionConfig. Name is left empty so callers can keep an existing name
// or invent one. Query sslmode (and mysql tls=) map onto SSLMode; a host that
// looks like an absolute path becomes Socket.
func ParseConnectionURI(raw string) (ConnectionConfig, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	if raw == "" {
		return ConnectionConfig{}, fmt.Errorf("empty connection URI")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ConnectionConfig{}, fmt.Errorf("invalid connection URI: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "postgres", "postgresql":
		return parsePostgresURI(u)
	case "mysql", "mariadb":
		return parseMySQLURI(u)
	case "sqlite", "file":
		return parseSQLiteURI(u, raw)
	default:
		return ConnectionConfig{}, fmt.Errorf("unsupported URI scheme %q (want postgres, mysql, or sqlite)", u.Scheme)
	}
}

func parsePostgresURI(u *url.URL) (ConnectionConfig, error) {
	cfg := ConnectionConfig{Driver: DriverPostgres, Port: 5432}
	applyUserInfo(&cfg, u)
	cfg.Database = strings.TrimPrefix(u.Path, "/")
	if q := u.Query(); q.Get("sslmode") != "" {
		cfg.SSLMode = NormalizeSSLMode(q.Get("sslmode"))
	}
	host, port, sock := splitURIHost(u, 5432)
	if sock != "" {
		cfg.Socket = sock
	} else {
		cfg.Host = host
		if port > 0 {
			cfg.Port = port
		}
	}
	// libpq-style ?host=/var/run/postgresql
	if h := u.Query().Get("host"); h != "" && strings.HasPrefix(h, "/") {
		cfg.Socket = h
		cfg.Host = ""
	}
	return cfg, nil
}

func parseMySQLURI(u *url.URL) (ConnectionConfig, error) {
	cfg := ConnectionConfig{Driver: DriverMySQL, Port: 3306}
	applyUserInfo(&cfg, u)
	cfg.Database = strings.TrimPrefix(u.Path, "/")
	q := u.Query()
	if q.Get("sslmode") != "" {
		cfg.SSLMode = NormalizeSSLMode(q.Get("sslmode"))
	} else if tls := q.Get("tls"); tls != "" {
		cfg.SSLMode = mysqlTLSToSSLMode(tls)
	}
	if sock := q.Get("socket"); sock != "" {
		cfg.Socket = sock
	} else if sock := q.Get("unix_socket"); sock != "" {
		cfg.Socket = sock
	}
	host, port, sock := splitURIHost(u, 3306)
	if sock != "" && cfg.Socket == "" {
		cfg.Socket = sock
	} else if cfg.Socket == "" {
		cfg.Host = host
		if port > 0 {
			cfg.Port = port
		}
	}
	return cfg, nil
}

func parseSQLiteURI(u *url.URL, raw string) (ConnectionConfig, error) {
	cfg := ConnectionConfig{Driver: DriverSQLite}
	// file:/abs/path, file:///abs/path, sqlite:///abs/path, sqlite://relative
	switch {
	case u.Scheme == "file":
		// url.Parse("file:///tmp/x.db") → Path=/tmp/x.db; Opaque may hold path
		// for file:relative forms.
		if u.Opaque != "" && u.Path == "" {
			cfg.Database = u.Opaque
		} else {
			cfg.Database = u.Path
			if u.Host != "" && u.Host != "localhost" {
				// file://hostname/path — uncommon; keep host+path
				cfg.Database = "/" + u.Host + u.Path
			}
		}
	default: // sqlite
		if u.Host != "" && u.Path != "" {
			cfg.Database = u.Host + u.Path
		} else if u.Path != "" {
			cfg.Database = u.Path
		} else if u.Opaque != "" {
			cfg.Database = u.Opaque
		}
	}
	cfg.Database = strings.TrimSpace(cfg.Database)
	// sqlite:////abs/path parses as Path="//abs/path"; collapse to one leading slash.
	for strings.HasPrefix(cfg.Database, "//") {
		cfg.Database = cfg.Database[1:]
	}
	if cfg.Database == "" {
		return ConnectionConfig{}, fmt.Errorf("sqlite URI missing path: %s", raw)
	}
	return cfg, nil
}

func applyUserInfo(cfg *ConnectionConfig, u *url.URL) {
	if u.User == nil {
		return
	}
	cfg.Username = u.User.Username()
	if pass, ok := u.User.Password(); ok {
		cfg.Password = pass
	}
}

// splitURIHost extracts host, port, and optional unix-socket path from u.
// Absolute-path hosts (postgres host=/dir) become sockets.
func splitURIHost(u *url.URL, defaultPort int) (host string, port int, socket string) {
	port = defaultPort
	h := u.Host
	if h == "" {
		return "", port, ""
	}
	// url.Parse keeps IPv6 as [addr]:port in Host.
	if strings.HasPrefix(h, "/") {
		return "", 0, h
	}
	hostname, portStr, err := net.SplitHostPort(h)
	if err != nil {
		// No port — Host is bare hostname (or path-like without brackets).
		if strings.HasPrefix(h, "/") {
			return "", 0, h
		}
		return h, port, ""
	}
	if hostname == "" {
		hostname = "localhost"
	}
	if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
		port = p
	}
	if strings.HasPrefix(hostname, "/") {
		return "", 0, hostname
	}
	return hostname, port, ""
}

func mysqlTLSToSSLMode(tls string) string {
	switch strings.ToLower(strings.TrimSpace(tls)) {
	case "false", "0", "skip-verify":
		return "disable"
	case "true", "1":
		return "require"
	case "preferred":
		return "prefer"
	default:
		return NormalizeSSLMode(tls)
	}
}

// SuggestConnectionName builds a short display name from a parsed URI config
// (e.g. "localhost/myapp" or "mydb.sqlite").
func SuggestConnectionName(cfg ConnectionConfig) string {
	switch cfg.Driver {
	case DriverSQLite:
		base := cfg.Database
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		if base == "" {
			return "sqlite"
		}
		return base
	default:
		host := cfg.Host
		if host == "" {
			if cfg.Socket != "" {
				host = "socket"
			} else {
				host = "localhost"
			}
		}
		if cfg.Database != "" {
			return host + "/" + cfg.Database
		}
		return host
	}
}
