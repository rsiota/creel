package db

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
)

const sshKeepAliveInterval = 30 * time.Second

// mysqlRegisterDialContext registers a custom dialer for the MySQL driver
// that routes connections through the SSH tunnel.
func mysqlRegisterDialContext(name string, tunnel *SSHTunnel) {
	mysql.RegisterDialContext(name, func(ctx context.Context, addr string) (net.Conn, error) {
		return tunnel.DialContext(ctx, "tcp", addr)
	})
}

// SSHTunnel manages an SSH connection used to tunnel database traffic.
type SSHTunnel struct {
	client *ssh.Client
	stop   chan struct{}
	once   sync.Once
}

// NewSSHTunnel establishes an SSH connection to the bastion host.
// Returns (nil, nil) if no SSH host is configured.
func NewSSHTunnel(cfg ConnectionConfig) (*SSHTunnel, error) {
	if cfg.SSHHost == "" {
		return nil, nil
	}

	port := cfg.SSHPort
	if port == 0 {
		port = 22
	}

	authMethods, err := sshAuthMethods(cfg)
	if err != nil {
		return nil, err
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.SSHHost, port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	t := &SSHTunnel{client: client, stop: make(chan struct{})}
	go t.keepAlive()
	return t, nil
}

// DialContext opens a connection to the target through the SSH tunnel.
func (t *SSHTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.client.Dial(network, addr)
}

// NewSession opens an SSH session on the bastion (e.g. to run remote mysqldump).
func (t *SSHTunnel) NewSession() (*ssh.Session, error) {
	if t == nil || t.client == nil {
		return nil, fmt.Errorf("no active SSH tunnel")
	}
	return t.client.NewSession()
}

// Close closes the underlying SSH client connection and stops keepalives.
func (t *SSHTunnel) Close() error {
	if t == nil {
		return nil
	}
	t.once.Do(func() {
		close(t.stop)
	})
	if t.client == nil {
		return nil
	}
	return t.client.Close()
}

// keepAlive sends OpenSSH keepalive requests so idle bastion sessions (and
// intervening NATs) do not drop the tunnel. On failure the client is closed
// so the next DB Ping/Dial fails fast and the UI can reconnect.
func (t *SSHTunnel) keepAlive() {
	ticker := time.NewTicker(sshKeepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			_, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				_ = t.client.Close()
				return
			}
		}
	}
}

func sshAuthMethods(cfg ConnectionConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if cfg.SSHKeyPath != "" {
		signer, err := loadKeySigner(cfg.SSHKeyPath, cfg.SSHPassphrase)
		if err != nil {
			return nil, fmt.Errorf("ssh key %s: %w", cfg.SSHKeyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if cfg.SSHPassword != "" {
		methods = append(methods, ssh.Password(cfg.SSHPassword))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no ssh auth method: provide ssh_key_path or ssh_password")
	}

	return methods, nil
}

func loadKeySigner(keyPath, passphrase string) (ssh.Signer, error) {
	expanded, err := expandHomePath(keyPath)
	if err != nil {
		return nil, err
	}
	keyBytes, err := os.ReadFile(expanded)
	if err != nil {
		return nil, err
	}

	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
		if err == nil {
			return signer, nil
		}
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}
	return signer, nil
}

// expandHomePath resolves a leading ~ (~ or ~/rest) to the user's home
// directory before opening the SSH private key. Other paths are cleaned and
// returned unchanged.
func expandHomePath(raw string) (string, error) {
	raw = filepath.Clean(raw)
	if raw == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if len(raw) >= 2 && raw[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, raw[2:]), nil
	}
	return raw, nil
}
