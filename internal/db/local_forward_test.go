package db

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestLocalForwardProxiesBytes(t *testing.T) {
	remoteLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer remoteLn.Close()

	go func() {
		c, err := remoteLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		n, err := c.Read(buf)
		if err != nil {
			return
		}
		_, _ = c.Write([]byte("pong:" + string(buf[:n])))
	}()

	fwd, err := StartLocalForward(
		func(_ context.Context, network, addr string) (net.Conn, error) {
			return net.Dial(network, addr)
		},
		remoteLn.Addr().String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer fwd.Close()

	client, err := net.DialTimeout("tcp", net.JoinHostPort(fwd.Host, fmt.Sprintf("%d", fwd.Port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pong:ping" {
		t.Fatalf("got %q", got)
	}
}

func TestLocalForwardNilDial(t *testing.T) {
	if _, err := StartLocalForward(nil, "127.0.0.1:1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSSHTunnelStartLocalForwardNil(t *testing.T) {
	var tnl *SSHTunnel
	if _, err := tnl.StartLocalForward("127.0.0.1:3306"); err == nil {
		t.Fatal("expected error")
	}
}
