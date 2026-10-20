package probers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptrace"

	"github.com/coder/websocket"
)

// NewWebSocketStream creates a Stream prober that holds a WebSocket open to
// the configured ws:// or wss:// endpoint and heartbeats it with ping/pong
// control frames. Every RFC 6455 server must answer pings, so any WebSocket
// endpoint is a valid target, not only the astrolavos /echo handler.
func NewWebSocketStream(c ProberConfig) *Stream {
	return &Stream{
		ProberConfig: c,
		proberType:   "websocket",
		dialer:       &wsDialer{url: c.endpoint, client: c.client},
	}
}

// wsDialer opens WebSocket connections, tracing the HTTP upgrade so the
// connect phases land in the same histograms as httpTrace probes.
type wsDialer struct {
	url    string
	client *http.Client
}

func (d *wsDialer) Dial(ctx context.Context) (streamConn, connectTimings, error) {
	t := newTracePoint()

	trace := &httptrace.ClientTrace{
		GetConn:              t.getConnTimeHandler,
		DNSStart:             t.dnsStartHandler,
		DNSDone:              t.dnsDoneHandler,
		ConnectStart:         t.connStartHandler,
		ConnectDone:          t.connDoneHandler,
		TLSHandshakeStart:    t.tlsStartHandler,
		TLSHandshakeDone:     t.tlsDoneHandler,
		GotConn:              t.gotConnTimeHandler,
		GotFirstResponseByte: t.firstByteTimeHandler,
	}

	//nolint:bodyclose // the response body of a successful upgrade is the connection itself
	c, _, err := websocket.Dial(httptrace.WithClientTrace(ctx, trace), d.url, &websocket.DialOptions{HTTPClient: d.client})
	if err != nil {
		return nil, connectTimings{}, fmt.Errorf("websocket dial failed: %w", err)
	}

	t.totalDoneHandler()
	t.setDNSDuration()
	t.setConnDuration()
	t.setTLSDuration()
	t.setGotConnDuration()
	t.setFirstByteDuration()
	t.setTotalDuration()

	// CloseRead runs the reader goroutine that delivers pongs to Ping; the
	// returned context ends when the connection is closed by either side.
	readCtx := c.CloseRead(context.Background())

	return &wsConn{c: c, readCtx: readCtx}, connectTimings{
		dns:       t.dnsDuration,
		conn:      t.connDuration,
		tls:       t.tlsDuration,
		gotConn:   t.gotConnDuration,
		firstByte: t.firstByteDuration,
		total:     t.totalDuration,
	}, nil
}

// wsConn heartbeats a WebSocket with ping/pong frames.
type wsConn struct {
	c       *websocket.Conn
	readCtx context.Context
}

func (w *wsConn) Heartbeat(ctx context.Context) error {
	if err := w.readCtx.Err(); err != nil {
		return fmt.Errorf("websocket closed by peer: %w", context.Cause(w.readCtx))
	}

	// Ping waits for the matching pong; when ctx expires the library closes
	// the connection, which is what a black-holed flow deserves. The library
	// does not wrap ctx.Err(), so add it for metrics.CategorizeError.
	if err := w.c.Ping(ctx); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("websocket ping failed: %w: %w", ctxErr, err)
		}

		return fmt.Errorf("websocket ping failed: %w", err)
	}

	return nil
}

// Close tears the connection down without the close handshake: a graceful
// Close waits for the peer's close frame, which a black-holed peer never
// sends, and would stall the reconnect for the library's internal timeout.
func (w *wsConn) Close() error {
	return w.c.CloseNow()
}
