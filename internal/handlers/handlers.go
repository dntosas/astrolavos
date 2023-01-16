// Package handlers represent
package handlers

import (
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
)

const (
	maxPayloadSize = 10485760
)

// OKHandler empty handler that sends back 200 with 0 content
func OKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Header().Add("Content-Length", "0")
}

// LatencyHandler handles requests sent on /latency endpoint
func LatencyHandler(w http.ResponseWriter, r *http.Request) {
	queryMap := r.URL.Query()
	payloadSize := queryMap.Get("payloadSize")
	if payloadSize == "" {
		w.Header().Add("Content-Length", "0")
		w.WriteHeader(200)
		return
	}

	i, err := strconv.Atoi(payloadSize)
	if err != nil {
		w.Header().Add("Content-Length", "0")
		w.WriteHeader(400)
		log.Error(err)
		return
	}

	if i > maxPayloadSize {
		payloadSizeExceeded := "Exceeded max allowed payloadSize: " + strconv.Itoa(maxPayloadSize)
		w.Header().Add("Content-Length", strconv.Itoa(len(payloadSizeExceeded)))
		w.WriteHeader(400)
		_, err = w.Write([]byte(payloadSizeExceeded))
		if err != nil {
			log.Error(err)
		}
		return
	}

	w.Header().Add("Content-Length", payloadSize)
	w.WriteHeader(200)
	_, err = w.Write(make([]byte, i))
	if err != nil {
		log.Error(err)
	}
}
