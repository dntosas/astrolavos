package probers

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

// connectTimings are the phases of one successful stream connect, in
// seconds. Zero values are not observed: tcpStream has no DNS, TLS or
// first-byte phase.
type connectTimings struct {
	dns, conn, tls, gotConn, firstByte, total float64
}

// streamConn is one long-lived connection the engine heartbeats.
type streamConn interface {
	// Heartbeat performs one round-trip; it must honour ctx's deadline.
	Heartbeat(ctx context.Context) error
	Close() error
}

// streamDialer opens a streamConn.
type streamDialer interface {
	Dial(ctx context.Context) (streamConn, connectTimings, error)
}

// Stream keeps a single connection open and heartbeats it every interval,
// reconnecting with backoff when a heartbeat or connect fails. It models the
// long-lived connections of real workloads: a flow that is silently
// black-holed shows up as a heartbeat timeout followed by a reconnect,
// while short-lived probers would only see the healthy new connection.
//
// Connects are recorded as fresh-connection requests and heartbeats as
// reused-connection requests, so the conn_reused label and the alerts built
// on it apply to streams as well.
type Stream struct {
	ProberConfig

	proberType string
	dialer     streamDialer
}

// String returns a human-readable description of the prober configuration.
func (s *Stream) String() string {
	return fmt.Sprintf("%s Prober Endpoint: %s - Interval: %v - Tag: %s - Timeout: %v", s.proberType, s.endpoint, s.interval, s.tag, s.timeout)
}

// Run connects, heartbeats and reconnects until ctx is cancelled. In one-off
// mode it connects, sends a single heartbeat and returns.
func (s *Stream) Run(ctx context.Context) {
	defer s.wg.Done()

	log.Infof("Starting %s", s)

	attempt := 0

	for ctx.Err() == nil {
		conn, ok := s.connect(ctx)
		if !ok {
			if s.isOneOff {
				return // one attempt, already recorded as a failed connect
			}

			if !s.sleepBackoff(ctx, attempt) {
				break
			}

			attempt++

			continue
		}

		attempt = 0

		if s.isOneOff {
			_ = s.heartbeat(ctx, conn, time.Now())
			_ = conn.Close()

			return
		}

		s.serve(ctx, conn)
	}

	log.Infof("%s: received shutdown signal, exiting", s)
}

// connect dials once under the attempt timeout and records the attempt as a
// fresh-connection request.
func (s *Stream) connect(ctx context.Context) (streamConn, bool) {
	actx, cancel := s.attemptContext(ctx)
	defer cancel()

	conn, tm, err := s.dialer.Dial(actx)

	s.promC.UpdateRequestsCounter(s.endpoint, s.proberType, s.tag, "", false)

	if err != nil {
		if ctx.Err() == nil {
			log.WithError(err).Warnf("%s: connect failed", s)
			s.promC.UpdateErrorsCounter(s.endpoint, s.proberType, s.tag, false, err)
		}

		s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, 0)

		return nil, false
	}

	s.observeConnect(tm)

	return conn, true
}

// observeConnect records the connect phases into the latency histograms as a
// fresh connection, skipping phases the transport does not have.
func (s *Stream) observeConnect(tm connectTimings) {
	observe := func(f func(string, string, string, bool, float64), v float64) {
		if v > 0 {
			f(s.endpoint, s.proberType, s.tag, false, v)
		}
	}

	observe(s.promC.UpdateDNSHistogram, tm.dns)
	observe(s.promC.UpdateConnHistogram, tm.conn)
	observe(s.promC.UpdateTLSHistogram, tm.tls)
	observe(s.promC.UpdateGotConnHistogram, tm.gotConn)
	observe(s.promC.UpdateFirstByteHistogram, tm.firstByte)
	observe(s.promC.UpdateTotalHistogram, tm.total)
}

// serve heartbeats conn every interval until a heartbeat fails or ctx is
// done. The connection is closed on return; a failure (not a shutdown) is
// counted as a reconnect.
func (s *Stream) serve(ctx context.Context, conn streamConn) {
	defer func() { _ = conn.Close() }()

	connectedAt := time.Now()

	s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, 0)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.heartbeat(ctx, conn, connectedAt)
			if err == nil {
				continue
			}

			if ctx.Err() == nil {
				s.promC.IncStreamReconnects(s.endpoint, s.proberType, s.tag, err)
			}

			s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, 0)

			return
		}
	}
}

// heartbeat performs one round-trip under the attempt timeout and records it
// as a reused-connection request.
func (s *Stream) heartbeat(ctx context.Context, conn streamConn, connectedAt time.Time) error {
	actx, cancel := s.attemptContext(ctx)
	defer cancel()

	start := time.Now()
	err := conn.Heartbeat(actx)

	s.promC.UpdateRequestsCounter(s.endpoint, s.proberType, s.tag, "", true)

	if err != nil {
		if ctx.Err() == nil {
			log.WithError(err).Warnf("%s: heartbeat failed, reconnecting", s)
			s.promC.UpdateErrorsCounter(s.endpoint, s.proberType, s.tag, true, err)
		}

		return err
	}

	s.promC.UpdateTotalHistogram(s.endpoint, s.proberType, s.tag, true, time.Since(start).Seconds())
	s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, time.Since(connectedAt).Seconds())

	return nil
}

// sleepBackoff waits 100ms·2^attempt, capped at the probe interval, before
// the next connect. It returns false when ctx ended while waiting.
func (s *Stream) sleepBackoff(ctx context.Context, attempt int) bool {
	shift := min(attempt, 30)
	delay := min(time.Duration(100*(1<<uint(shift)))*time.Millisecond, s.interval)

	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}
