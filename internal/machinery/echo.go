package machinery

import (
	"context"
	"fmt"
	"io"
	"net"

	log "github.com/sirupsen/logrus"
)

// startEchoListener serves a plain TCP echo on port so this instance can be
// the peer of tcpStream probers. Every accepted connection is copied back
// onto itself until the client closes or ctx is cancelled. A port of 0
// disables the listener and returns nil.
func startEchoListener(ctx context.Context, port int) (net.Listener, error) {
	if port == 0 {
		return nil, nil //nolint:nilnil // disabled is not an error and there is nothing to close
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("echo listener on port %d: %w", port, err)
	}

	log.WithField("port", port).Info("Starting TCP echo listener")

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				log.Debug("TCP echo listener stopped")

				return
			}

			go echoConn(ctx, c)
		}
	}()

	return ln, nil
}

// echoConn copies c onto itself; cancelling ctx closes the connection.
func echoConn(ctx context.Context, c net.Conn) {
	defer func() { _ = c.Close() }()

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-done:
		}
	}()

	_, _ = io.Copy(c, c)
}
