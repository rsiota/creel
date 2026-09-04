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

// TestLocalForwardLargeDownloadAfterClientWriteIdle mirrors mysqldump: the
// client sends a short request then only reads, while the server streams a
// large payload. The old proxy closed both sides when client→server Copy
// finished, truncating the download.
func TestLocalForwardLargeDownloadAfterClientWriteIdle(t *testing.T) {
	const payloadSize = 8 << 20 // 8 MiB

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
		buf := make([]byte, 16)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
		// Client is done writing; stream a large response (mysqldump result).
		chunk := make([]byte, 64<<10)
		for i := range chunk {
			chunk[i] = byte(i)
		}
		remaining := payloadSize
		for remaining > 0 {
			n := len(chunk)
			if n > remaining {
				n = remaining
			}
			if _, err := c.Write(chunk[:n]); err != nil {
				return
			}
			remaining -= n
		}
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

	client, err := net.Dial("tcp", net.JoinHostPort(fwd.Host, fmt.Sprintf("%d", fwd.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))

	if _, err := client.Write([]byte("SELECT big stuff")); err != nil {
		t.Fatal(err)
	}
	// Half-close write side like a client that only reads the result set.
	if cw, ok := client.(*net.TCPConn); ok {
		_ = cw.CloseWrite()
	}

	got, err := io.Copy(io.Discard, client)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != payloadSize {
		t.Fatalf("downloaded %d bytes, want %d (truncated proxy?)", got, payloadSize)
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
