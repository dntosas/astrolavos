package machinery

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

func TestStartEchoListener_DisabledOnPortZero(t *testing.T) {
	ln, err := startEchoListener(context.Background(), 0)
	if err != nil || ln != nil {
		t.Fatalf("expected (nil, nil) for port 0, got (%v, %v)", ln, err)
	}
}

func TestStartEchoListener_EchoesAndStopsOnCancel(t *testing.T) {
	// Grab a free port, release it, and let the listener take it.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	ctx, cancel := context.WithCancel(context.Background())

	ln, err := startEchoListener(ctx, port)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	defer func() { _ = ln.Close() }()

	c, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer func() { _ = c.Close() }()

	_ = c.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err := c.Write([]byte("ping 1\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || line != "ping 1\n" {
		t.Fatalf("echo = %q, %v", line, err)
	}

	cancel()

	// After cancellation the server side closes: the next read hits EOF.
	buf := make([]byte, 1)

	if _, err := c.Read(buf); err == nil {
		t.Fatal("expected the connection to be closed after cancel")
	}
}
