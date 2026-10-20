package handlers_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dntosas/astrolavos/internal/handlers"

	"github.com/coder/websocket"
)

func TestEchoHandler_EchoesMessagesAndAnswersPings(t *testing.T) {
	srv := httptest.NewServer(handlers.NewEchoHandler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, "ws://"+strings.TrimPrefix(srv.URL, "http://"), nil) //nolint:bodyclose // the body is the connection
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer func() { _ = c.CloseNow() }()

	if err := c.Write(ctx, websocket.MessageText, []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	typ, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if typ != websocket.MessageText || string(data) != "hello" {
		t.Errorf("echo = (%v, %q), want (text, hello)", typ, data)
	}

	// Ping needs a concurrent reader to receive the pong.
	readCtx := c.CloseRead(ctx)

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	if readCtx.Err() != nil {
		t.Fatalf("connection closed unexpectedly: %v", context.Cause(readCtx))
	}
}
