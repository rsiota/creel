package db

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
)

// LocalForward is a 127.0.0.1 TCP listener that proxies each accepted
// connection through dial to remoteAddr. Used so an external tool like
// mysqldump can reach a MySQL server that Creel only reaches via SSH.
type LocalForward struct {
	Host string
	Port int

	ln     net.Listener
	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	remote string

	closeOnce sync.Once
	done      chan struct{}
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
			select {
			case <-f.done:
			default:
			}
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

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remote, local)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(local, remote)
		done <- struct{}{}
	}()
	<-done
}

// Close stops accepting and closes the listener. In-flight proxies finish on
// their own when either side hangs up.
func (f *LocalForward) Close() error {
	if f == nil {
		return nil
	}
	var err error
	f.closeOnce.Do(func() {
		close(f.done)
		err = f.ln.Close()
	})
	return err
}
