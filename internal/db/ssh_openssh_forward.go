package db

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// lookPathSSH is exec.LookPath for "ssh"; tests replace it.
var lookPathSSH = exec.LookPath

// startOpenSSHCmd starts an ssh process; tests replace it.
var startOpenSSHCmd = func(cmd *exec.Cmd) error { return cmd.Start() }

// startOpenSSHLocalForward runs `ssh -N -L 127.0.0.1:port:mysqlHost:mysqlPort`
// for bulk tools like mysqldump. OpenSSH handles large streams more reliably
// than an in-process crypto/ssh byte proxy. Requires a key file and no
// interactive passphrase (BatchMode); callers fall back to the in-process
// tunnel when this returns an error.
func startOpenSSHLocalForward(cfg ConnectionConfig, mysqlHost string, mysqlPort int) (*LocalForward, error) {
	if strings.TrimSpace(cfg.SSHHost) == "" {
		return nil, fmt.Errorf("no SSH host")
	}
	if strings.TrimSpace(cfg.SSHKeyPath) == "" {
		return nil, fmt.Errorf("no SSH key path")
	}
	if strings.TrimSpace(cfg.SSHPassphrase) != "" {
		// BatchMode cannot unlock a passphrase-protected key without an agent.
		return nil, fmt.Errorf("SSH key passphrase set; use in-process forward")
	}
	bin, err := lookPathSSH("ssh")
	if err != nil {
		return nil, err
	}
	keyPath, err := expandHomePath(cfg.SSHKeyPath)
	if err != nil {
		return nil, err
	}
	if mysqlHost == "" {
		mysqlHost = "127.0.0.1"
	}
	if mysqlPort == 0 {
		mysqlPort = 3306
	}
	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	// Bind a free port, then release it for ssh -L to claim.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	localPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	args := []string{
		"-N",
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=8",
		// Match Creel's in-process tunnel: do not block on host-key prompts.
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-i", keyPath,
		"-p", strconv.Itoa(sshPort),
		"-L", fmt.Sprintf("127.0.0.1:%d:%s:%d", localPort, mysqlHost, mysqlPort),
		fmt.Sprintf("%s@%s", cfg.SSHUser, cfg.SSHHost),
	}
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := startOpenSSHCmd(cmd); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return nil, fmt.Errorf("ssh -L exited: %s", strings.TrimSpace(stderr.String()))
		default:
		}
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return &LocalForward{
				Host:      "127.0.0.1",
				Port:      localPort,
				cmd:       cmd,
				cmdExited: exited,
				done:      make(chan struct{}),
			}, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	<-exited
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = "timed out waiting for local forward"
	}
	return nil, fmt.Errorf("ssh -L: %s", msg)
}
