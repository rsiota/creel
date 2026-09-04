package db

import (
	"strings"
	"testing"
)

func TestStartOpenSSHLocalForwardRequiresKey(t *testing.T) {
	_, err := startOpenSSHLocalForward(ConnectionConfig{
		SSHHost: "bastion", SSHUser: "u",
	}, "127.0.0.1", 3306)
	if err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("got %v", err)
	}
}

func TestStartOpenSSHLocalForwardSkipsPassphrase(t *testing.T) {
	_, err := startOpenSSHLocalForward(ConnectionConfig{
		SSHHost: "bastion", SSHUser: "u",
		SSHKeyPath: "~/.ssh/id_ed25519", SSHPassphrase: "secret",
	}, "127.0.0.1", 3306)
	if err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("got %v", err)
	}
}
