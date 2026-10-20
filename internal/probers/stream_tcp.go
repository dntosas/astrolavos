package probers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// NewTCPStream creates a Stream prober that holds a plain TCP connection to
// host:port and heartbeats it with newline-framed "ping <seq>" lines that
// the peer must echo back (see the astrolavos TCP echo listener).
func NewTCPStream(c ProberConfig) *Stream {
	return &Stream{
		ProberConfig: c,
		proberType:   "tcpstream",
		dialer:       &tcpDialer{addr: c.endpoint},
	}
}

// tcpDialer opens plain TCP connections; the dial time is the only connect phase.
type tcpDialer struct {
	addr string
}

func (d *tcpDialer) Dial(ctx context.Context) (streamConn, connectTimings, error) {
	start := time.Now()

	var dialer net.Dialer

	c, err := dialer.DialContext(ctx, "tcp", d.addr)
	if err != nil {
		return nil, connectTimings{}, fmt.Errorf("tcp dial failed: %w", err)
	}

	elapsed := time.Since(start).Seconds()

	return &tcpConn{c: c, r: bufio.NewReader(c)}, connectTimings{conn: elapsed, total: elapsed}, nil
}

// tcpConn heartbeats a TCP connection with an echoed line.
type tcpConn struct {
	c   net.Conn
	r   *bufio.Reader
	seq uint64
}

func (t *tcpConn) Heartbeat(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(DefaultTimeout)
	}

	if err := t.c.SetDeadline(deadline); err != nil {
		return fmt.Errorf("tcp heartbeat set deadline failed: %w", err)
	}

	t.seq++
	msg := fmt.Sprintf("ping %d\n", t.seq)

	if _, err := io.WriteString(t.c, msg); err != nil {
		return fmt.Errorf("tcp heartbeat write failed: %w", wrapTimeout(err))
	}

	line, err := t.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("tcp heartbeat read failed: %w", wrapTimeout(err))
	}

	if line != msg {
		return fmt.Errorf("tcp heartbeat mismatch: sent %q, got %q", msg, line)
	}

	return nil
}

func (t *tcpConn) Close() error {
	return t.c.Close()
}

// wrapTimeout maps a net.Error deadline expiry to context.DeadlineExceeded so
// metrics.CategorizeError buckets it as "timeout" like every other prober.
func wrapTimeout(err error) error {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Errorf("%w: %w", context.DeadlineExceeded, err)
	}

	return err
}
