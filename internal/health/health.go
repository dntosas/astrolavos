// Package health provides thread-safe application health state tracking
// for Kubernetes liveness, readiness, and startup probes.
package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

// State tracks the application's liveness and readiness atomically.
// The zero value is not-ready and not-alive, which is the correct initial
// state: the app must explicitly declare itself healthy after initialization.
type State struct {
	alive     atomic.Bool
	ready     atomic.Bool
	startedAt time.Time
}

// NewState returns a State with the clock started.
// Both alive and ready default to false.
func NewState() *State {
	return &State{startedAt: time.Now()}
}

// SetAlive marks the process as alive (liveness probe passes).
func (s *State) SetAlive() { s.alive.Store(true) }

// SetNotAlive marks the process as not alive (liveness probe fails).
func (s *State) SetNotAlive() { s.alive.Store(false) }

// IsAlive reports whether the process is alive.
func (s *State) IsAlive() bool { return s.alive.Load() }

// SetReady marks the application as ready to serve traffic.
func (s *State) SetReady() { s.ready.Store(true) }

// SetNotReady marks the application as not ready (e.g. during shutdown).
func (s *State) SetNotReady() { s.ready.Store(false) }

// IsReady reports whether the application is ready to serve traffic.
func (s *State) IsReady() bool { return s.ready.Load() }

// probeResponse is the JSON body returned by health endpoints.
type probeResponse struct {
	Status string `json:"status"`
	Uptime string `json:"uptime"`
}

// LiveHandler returns 200 when the process is alive, 503 otherwise.
func LiveHandler(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeProbe(w, s.IsAlive(), s.startedAt)
	}
}

// ReadyHandler returns 200 when the application is ready to serve traffic,
// 503 otherwise. During shutdown, the app marks itself not-ready so
// Kubernetes removes it from Service endpoints before the process exits.
func ReadyHandler(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeProbe(w, s.IsReady(), s.startedAt)
	}
}

// PreStopHandler is called by the Kubernetes preStop lifecycle hook via
// httpGet *before* SIGTERM is sent. It immediately marks the app as
// not-ready so the next readiness probe fails, then blocks for the
// given drain duration. This holds the pod alive while kube-proxy
// propagates the endpoint removal — preventing traffic from reaching
// a pod that is about to shut down.
//
// The handler returns 200 after the drain completes, at which point the
// kubelet sends SIGTERM and the normal shutdown path takes over.
func PreStopHandler(s *State, drain time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		s.SetNotReady()
		log.WithField("drain", drain).Info("preStop: marked not-ready, draining")

		time.Sleep(drain)

		log.Info("preStop: drain complete, returning to kubelet")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_ = json.NewEncoder(w).Encode(probeResponse{
			Status: "draining",
			Uptime: time.Since(s.startedAt).Truncate(time.Millisecond).String(),
		})
	}
}

func writeProbe(w http.ResponseWriter, ok bool, startedAt time.Time) {
	status := "ok"
	code := http.StatusOK

	if !ok {
		status = "unavailable"
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	_ = json.NewEncoder(w).Encode(probeResponse{
		Status: status,
		Uptime: time.Since(startedAt).Truncate(time.Millisecond).String(),
	})
}
