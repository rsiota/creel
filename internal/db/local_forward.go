package db

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
)

// LocalForward is a 127.0.0.1 endpoint that reaches a remote MySQL address
// through SSH so tools like mysqldump can connect on localhost.
//
// Two backends:
//   - in-process: ln accepts and dial proxies via crypto/ssh
//   - OpenSSH:    cmd runs `ssh -L`; ln/dial are nil
type LocalForward struct {
	Host string
	Port int

	ln     net.Listener
	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	remote string
	cmd    *exec.Cmd
	// cmdExited is closed after cmd.Wait returns (OpenSSH backend only).
	cmdExited <-chan struct{}

	closeOnce sync.Once
	done      chan struct{} // closed when fully shut down
}

// StartLocalForward listens on 127.0.0.1:0 and forwards to remoteAddr via dial.
func StartLocalForward(
	dial func(ctx context.Context, network, addr string) (net.Conn, error),
	remoteAddr string,
) (*LocalForward, error) {
	if dial == nil {
		return nil, fmt.Errorf("local forward: nil dialer")
	}
	if remoteAddr == "" {
		return nil, fmt.Errorf("local forward: empty remote address")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		ln.Close()
		return nil, fmt.Errorf("local forward: unexpected listener addr %T", ln.Addr())
	}
	f := &LocalForward{
		Host:   "127.0.0.1",
		Port:   addr.Port,
		ln:     ln,
		dial:   dial,
		remote: remoteAddr,
		done:   make(chan struct{}),
	}
	go f.serve()
	return f, nil
}

// StartLocalForward opens a localhost TCP proxy through this SSH tunnel.
func (t *SSHTunnel) StartLocalForward(remoteAddr string) (*LocalForward, error) {
	if t == nil || t.client == nil {
		return nil, fmt.Errorf("no active SSH tunnel")
	}
	return StartLocalForward(t.DialContext, remoteAddr)
}

func (f *LocalForward) serve() {
	for {
		local, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.proxy(local)
	}
}

func (f *LocalForward) proxy(local net.Conn) {
	remote, err := f.dial(context.Background(), "tcp", f.remote)
	if err != nil {
		_ = local.Close()
		return
	}
	defer local.Close()
	defer remote.Close()

	// Bidirectional copy with half-close and larger buffers. Waiting on only
	// the first finished direction and then Close()'ing both used to truncate
	// mysqldump mid-result. Half-close keeps the download side alive.
	const bufSize = 256 << 10
	done := make(chan struct{})
	go func() {
		buf := make([]byte, bufSize)
		_, _ = io.CopyBuffer(remote, local, buf)
		closeWrite(remote)
		close(done)
	}()
	buf := make([]byte, bufSize)
	_, _ = io.CopyBuffer(local, remote, buf)
	closeWrite(local)
	<-done
}

// closeWrite shuts down the write half of c when supported so the peer can
// finish reading without tearing down the opposite direction.
func closeWrite(c net.Conn) {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}

// Close stops the forward: closes the in-process listener and/or kills ssh -L.
func (f *LocalForward) Close() error {
	if f == nil {
		return nil
	}
	var err error
	f.closeOnce.Do(func() {
		if f.cmd != nil && f.cmd.Process != nil {
			_ = f.cmd.Process.Kill()
			if f.cmdExited != nil {
				<-f.cmdExited
			} else {
				_, _ = f.cmd.Process.Wait()
			}
		}
		if f.ln != nil {
			err = f.ln.Close()
		}
		close(f.done)
	})
	return err
}
