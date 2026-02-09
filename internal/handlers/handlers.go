// Package handlers provides HTTP request handlers for the Astrolavos server.
package handlers

import (
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
)

const (
	maxPayloadSize = 10485760
)

// OKHandler responds with HTTP 200 and an empty body.
// Used for liveness and readiness probes.
func OKHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusOK)
}

// LatencyHandler handles requests on the /latency endpoint.
// It optionally returns a response body of a configurable size
// via the payloadSize query parameter.
func LatencyHandler(w http.ResponseWriter, r *http.Request) {
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
