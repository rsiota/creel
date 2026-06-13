package db

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
)

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

	return &SSHTunnel{client: client}, nil
}

// DialContext opens a connection to the target through the SSH tunnel.
func (t *SSHTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.client.Dial(network, addr)
}

// Close closes the underlying SSH client connection.
func (t *SSHTunnel) Close() error {
	if t == nil || t.client == nil {
		return nil
	}
	return t.client.Close()
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
	keyBytes, err := os.ReadFile(keyPath)
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
