package probers

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

// HTTPTrace struct holds information for httpTrace probes and
// implements Prober interface.
type HTTPTrace struct {
	ProberConfig
}

// NewHTTPTrace is the constructor function of httptrace struct.
func NewHTTPTrace(c ProberConfig) *HTTPTrace {
	return &HTTPTrace{c}
}

// String is used when we want to print info about httpTrace prober.
func (h *HTTPTrace) String() string {
	return fmt.Sprintf("httpTrace Prober Endpoint: %s - Interval: %v - Tag: %s - Retries: %d", h.endpoint, h.interval, h.tag, h.retries)
}

// Run is responsible for holding the logic or running the probing httpTrace
// measurements for an endpoint.
func (h *HTTPTrace) Run() {
	defer h.wg.Done()

	if h.isOneOff {
		h.runOneOff()
	} else {
		h.runInterval()
	}
}

// Stop sends a message to httpTrace's exit channel when it's time to stop.
func (h *HTTPTrace) Stop() {
	log.Debugf("Prober: %s will stop now", h)
	h.exit <- true
}

// runOneOff runs httpTrace probing measurement once with
// a number of retries and then exits.
func (h *HTTPTrace) runOneOff() {
	var isSuccess bool

	var err error

	var t *tracePoint

	log.Infof("Starting (OneOff) %s", h)

	loop := true
	for i := 0; i < h.retries && loop; i++ {
		select {
		case <-h.exit:
			log.Infof("HTTPTrace (OneOff): %s got message in exit channel, exiting", h)

			return
		default:
			log.Debugf("HTTPTrace (OneOff) for %s starts new trace probe", h)

			t, err = h.trace()
			if err == nil {
				isSuccess = true
				loop = false
			}
		}
	}

	h.promC.UpdateRequestsCounter(h.endpoint, "httptrace", h.tag, t.statusCode)

	if !isSuccess {
		log.Errorf("HTTPTrace (OneOff) of %s error: %v", h, err)
		h.promC.UpdateErrorsCounter(h.endpoint, "httptrace", h.tag, err.Error())
	} else {
		// Update all exposed Prometheus metrics histograms
		h.promC.UpdateDNSHistogram(h.endpoint, "httptrace", h.tag, t.dnsDuration)
		h.promC.UpdateConnHistogram(h.endpoint, "httptrace", h.tag, t.connDuration)
		h.promC.UpdateTLSHistogram(h.endpoint, "httptrace", h.tag, t.tlsDuration)
		h.promC.UpdateGotConnHistogram(h.endpoint, "httptrace", h.tag, t.gotConnDuration)
		h.promC.UpdateFirstByteHistogram(h.endpoint, "httptrace", h.tag, t.firstByteDuration)
		h.promC.UpdateTotalHistogram(h.endpoint, "httptrace", h.tag, t.totalDuration)
	}
}

// runInterval starts a loop with a ticker that run the tracing
// probing measurements in the workers interval.
func (h *HTTPTrace) runInterval() {
	ticker := time.NewTicker(h.interval)
	log.Infof("Starting %s", h)

	for {
		select {
		case <-h.exit:
			log.Infof("HTTPTrace: %s got message in exit channel, exiting", h)

			return
		case <-ticker.C:
			log.Debugf("HTTPTrace for %s starts new trace probe", h)
			h.executeWithRetry()
		}
	}
}

// executeWithRetry performs HTTP trace with exponential backoff retry logic
func (h *HTTPTrace) executeWithRetry() {
	var t *tracePoint
	var err error
	var isSuccess bool

	// Try with exponential backoff
	for attempt := 0; attempt < h.retries; attempt++ {
		t, err = h.trace()

		if err == nil {
			isSuccess = true
			break
		}

		// Don't sleep on the last attempt
		if attempt < h.retries-1 {
			// Exponential backoff: 100ms, 200ms, 400ms, etc.
			backoffDuration := time.Duration(100*(1<<uint(attempt))) * time.Millisecond
			log.Debugf("HTTPTrace attempt %d/%d failed for %s, retrying after %v", attempt+1, h.retries, h.endpoint, backoffDuration)
			time.Sleep(backoffDuration)
		}
	}

	h.promC.UpdateRequestsCounter(h.endpoint, "httptrace", h.tag, t.statusCode)

	if !isSuccess {
		log.Errorf("HTTPTrace of %s failed after %d attempts, error: %v", h, h.retries, err)
		h.promC.UpdateErrorsCounter(h.endpoint, "httptrace", h.tag, err.Error())
	} else {
		// Update all exposed Prometheus metrics histograms
		h.promC.UpdateDNSHistogram(h.endpoint, "httptrace", h.tag, t.dnsDuration)
		h.promC.UpdateConnHistogram(h.endpoint, "httptrace", h.tag, t.connDuration)
		h.promC.UpdateTLSHistogram(h.endpoint, "httptrace", h.tag, t.tlsDuration)
		h.promC.UpdateGotConnHistogram(h.endpoint, "httptrace", h.tag, t.gotConnDuration)
		h.promC.UpdateFirstByteHistogram(h.endpoint, "httptrace", h.tag, t.firstByteDuration)
		h.promC.UpdateTotalHistogram(h.endpoint, "httptrace", h.tag, t.totalDuration)
	}
}
