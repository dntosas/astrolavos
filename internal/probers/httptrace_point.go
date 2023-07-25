package probers

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Desc
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
		t.err = errors.Wrap(d.Err, "Error occurred while tracing on DNS part")
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

func (t *tracePoint) connDoneHandler(net, addr string, err error) {
	if err != nil {
		t.err = errors.Wrap(err, "Error occurred while tracing on TCP connection part")
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
		t.err = errors.Wrap(err, "Error occured while tracing on TLS part")
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
	return h.client
}

func (h *HTTPTrace) trace() (*tracePoint, error) {
	t := newTracePoint()

	// TODO: make sure our endpoint is valid http/https uri
	req, err := http.NewRequest("GET", h.endpoint, nil)
	if err != nil {
		return t, errors.Wrap(err, "Creation of new request failed")
	}

	// TODO: research if we need a new connection each time
	// If we don't make sure we have proper handling for 0 timings
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
		return t, errors.Wrap(err, "Request failed")
	}

	// Close Response
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return t, errors.Wrap(err, "Reading request body failed")
	}
	err = resp.Body.Close()
	if err != nil {
		return t, errors.Wrap(err, "Closing request body failed")
	}

	t.statusCode = strconv.Itoa(resp.StatusCode)

	t.totalDoneHandler()
	if t.err != nil {
		return t, errors.Wrap(t.err, "trace function failed")
	}

	// Call duration handlers
	t.setDNSDuration()
	t.setConnDuration()
	t.setTLSDuration()
	t.setGotConnDuration()
	t.setFirstByteDuration()
	t.setTotalDuration()

	log.Debugf("Response Code: %v\n", t.statusCode)
	log.Debugf("DNS Latency: %v\n", t.dnsDuration)
	log.Debugf("Connection Latency: %v\n", t.connDuration)
	log.Debugf("TLS Latency: %v\n", t.tlsDuration)
	log.Debugf("GotConnection Latency: %v\n", t.gotConnDuration)
	log.Debugf("TimeToFirstByte Latency: %v\n", t.firstByteDuration)
	log.Debugf("Total Latency: %v\n", t.totalDuration)

	return t, nil
}
