package probers

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

// tracePoint captures timing data at various stages of an HTTP request lifecycle.
type tracePoint struct {
	dnsStartTime time.Time
	dnsDoneTime  time.Time
	dnsDuration  float64

	connStartTime time.Time
	connDoneTime  time.Time
	connDuration  float64

	tlsStartTime time.Time
	tlsDoneTime  time.Time
	tlsDuration  float64

	gotConnTime     time.Time
	gotConnDuration float64

	firstByteTime     time.Time
	firstByteDuration float64

	totalStartTime time.Time
	totalDoneTime  time.Time
	totalDuration  float64

	statusCode string

	err error
}

func newTracePoint() *tracePoint {
	return &tracePoint{}
}

func (t *tracePoint) setDNSDuration() {
	t.dnsDuration = (t.dnsDoneTime.Sub(t.dnsStartTime)).Seconds()
}

func (t *tracePoint) dnsStartHandler(_ httptrace.DNSStartInfo) {
	t.dnsStartTime = time.Now()
}

func (t *tracePoint) dnsDoneHandler(d httptrace.DNSDoneInfo) {
	if d.Err != nil {
		t.err = fmt.Errorf("DNS resolution failed: %w", d.Err)

		return
	}

	t.dnsDoneTime = time.Now()
}

func (t *tracePoint) setConnDuration() {
	t.connDuration = (t.connDoneTime.Sub(t.connStartTime)).Seconds()
}

func (t *tracePoint) connStartHandler(_, _ string) {
	t.connStartTime = time.Now()
}

func (t *tracePoint) connDoneHandler(_, _ string, err error) {
	if err != nil {
		t.err = fmt.Errorf("TCP connection failed: %w", err)

		return
	}

	t.connDoneTime = time.Now()
}

func (t *tracePoint) setTLSDuration() {
	t.tlsDuration = (t.tlsDoneTime.Sub(t.tlsStartTime)).Seconds()
}

func (t *tracePoint) tlsStartHandler() {
	t.tlsStartTime = time.Now()
}

func (t *tracePoint) tlsDoneHandler(_ tls.ConnectionState, err error) {
	if err != nil {
		t.err = fmt.Errorf("TLS handshake failed: %w", err)

		return
	}

	t.tlsDoneTime = time.Now()
}

func (t *tracePoint) getConnTimeHandler(_ string) {
	t.totalStartTime = time.Now()
}

func (t *tracePoint) setGotConnDuration() {
	t.gotConnDuration = (t.gotConnTime.Sub(t.totalStartTime)).Seconds()
}

func (t *tracePoint) gotConnTimeHandler(_ httptrace.GotConnInfo) {
	t.gotConnTime = time.Now()
}

func (t *tracePoint) setFirstByteDuration() {
	t.firstByteDuration = (t.firstByteTime.Sub(t.totalStartTime)).Seconds()
}

func (t *tracePoint) firstByteTimeHandler() {
	t.firstByteTime = time.Now()
}

func (t *tracePoint) setTotalDuration() {
	t.totalDuration = (t.totalDoneTime.Sub(t.totalStartTime)).Seconds()
}

func (t *tracePoint) totalDoneHandler() {
	t.totalDoneTime = time.Now()
}

func (h *HTTPTrace) getClient() *http.Client {
	if h.reuseConnection {
		return h.client
	}

	return getCustomClient(h.reuseConnection, h.skipTLS)
}

func (h *HTTPTrace) trace(ctx context.Context) (*tracePoint, error) {
	t := newTracePoint()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint, nil)
	if err != nil {
		return t, fmt.Errorf("creation of new request failed: %w", err)
	}

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

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := h.getClient().Do(req)
	if err != nil {
		return t, fmt.Errorf("request failed: %w", err)
	}

	// Read and close response body
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return t, fmt.Errorf("reading response body failed: %w", err)
	}

	if err = resp.Body.Close(); err != nil {
		return t, fmt.Errorf("closing response body failed: %w", err)
	}

	t.statusCode = strconv.Itoa(resp.StatusCode)

	t.totalDoneHandler()

	if t.err != nil {
		return t, fmt.Errorf("trace failed: %w", t.err)
	}

	// Calculate all durations
	t.setDNSDuration()
	t.setConnDuration()
	t.setTLSDuration()
	t.setGotConnDuration()
	t.setFirstByteDuration()
	t.setTotalDuration()

	log.Debugf("Response Code: %v", t.statusCode)
	log.Debugf("DNS Latency: %v", t.dnsDuration)
	log.Debugf("Connection Latency: %v", t.connDuration)
	log.Debugf("TLS Latency: %v", t.tlsDuration)
	log.Debugf("GotConnection Latency: %v", t.gotConnDuration)
	log.Debugf("TimeToFirstByte Latency: %v", t.firstByteDuration)
	log.Debugf("Total Latency: %v", t.totalDuration)

	return t, nil
}
