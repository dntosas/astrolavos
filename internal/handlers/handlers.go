// Package handlers provides HTTP request handlers for the Astrolavos server.
package handlers

import (
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// DefaultMaxPayloadSize is the default maximum payload size (10MB) for the latency endpoint.
const DefaultMaxPayloadSize = 10485760

// OKHandler responds with HTTP 200 and an empty body.
// Used for liveness and readiness probes.
func OKHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusOK)
}

// NewLatencyHandler creates a latency handler with a configurable max payload size.
// If maxPayloadSize is 0, DefaultMaxPayloadSize is used.
func NewLatencyHandler(maxPayloadSize int) http.HandlerFunc {
	if maxPayloadSize <= 0 {
		maxPayloadSize = DefaultMaxPayloadSize
	}

	return func(w http.ResponseWriter, r *http.Request) {
		queryMap := r.URL.Query()

		payloadSize := queryMap.Get("payloadSize")
		if payloadSize == "" {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusOK)

			return
		}

		i, err := strconv.Atoi(payloadSize)
		if err != nil {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusBadRequest)
			log.Error(err)

			return
		}

		if i > maxPayloadSize {
			payloadSizeExceeded := "Exceeded max allowed payloadSize: " + strconv.Itoa(maxPayloadSize)
			w.Header().Set("Content-Length", strconv.Itoa(len(payloadSizeExceeded)))
			w.WriteHeader(http.StatusBadRequest)

			_, err = w.Write([]byte(payloadSizeExceeded))
			if err != nil {
				log.Error(err)
			}

			return
		}

		w.Header().Set("Content-Length", payloadSize)
		w.WriteHeader(http.StatusOK)

		_, err = w.Write(make([]byte, i))
		if err != nil {
			log.Error(err)
		}
	}
}
