package handlers

import (
	"net/http"

	"github.com/coder/websocket"
	log "github.com/sirupsen/logrus"
)

// NewEchoHandler upgrades the request to a WebSocket and echoes every message
// back with its original type until the client closes. Pings are answered
// by the library while reading, so a websocket stream prober only needs this
// handler to be reachable. Any origin is accepted: this is a probe target,
// not a browser-facing API.
//
// The upgrade hijacks the connection, so the http.Server read/write timeouts
// no longer apply to it; the connection lives as long as the peer keeps it.
func NewEchoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			log.WithError(err).Debug("echo: websocket accept failed")

			return
		}

		defer func() { _ = c.CloseNow() }()

		for {
			typ, data, err := c.Read(r.Context())
			if err != nil {
				return
			}

			if err := c.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	}
}
