package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dntosas/astrolavos/internal/health"
)

func TestState_DefaultsToNotAliveNotReady(t *testing.T) {
	s := health.NewState()

	if s.IsAlive() {
		t.Error("expected alive=false on new state")
	}

	if s.IsReady() {
		t.Error("expected ready=false on new state")
	}
}

func TestState_SetAliveReady(t *testing.T) {
	s := health.NewState()
	s.SetAlive()
	s.SetReady()

	if !s.IsAlive() {
		t.Error("expected alive=true after SetAlive")
	}

	if !s.IsReady() {
		t.Error("expected ready=true after SetReady")
	}
}

func TestState_SetNotReady(t *testing.T) {
	s := health.NewState()
	s.SetReady()
	s.SetNotReady()

	if s.IsReady() {
		t.Error("expected ready=false after SetNotReady")
	}
}

func TestState_SetNotAlive(t *testing.T) {
	s := health.NewState()
	s.SetAlive()
	s.SetNotAlive()

	if s.IsAlive() {
		t.Error("expected alive=false after SetNotAlive")
	}
}

func TestLiveHandler_Alive(t *testing.T) {
	s := health.NewState()
	s.SetAlive()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	health.LiveHandler(s)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	assertJSONStatus(t, w, "ok")
}

func TestLiveHandler_NotAlive(t *testing.T) {
	s := health.NewState()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	health.LiveHandler(s)(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	assertJSONStatus(t, w, "unavailable")
}

func TestReadyHandler_Ready(t *testing.T) {
	s := health.NewState()
	s.SetReady()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	health.ReadyHandler(s)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	assertJSONStatus(t, w, "ok")
}

func TestReadyHandler_NotReady(t *testing.T) {
	s := health.NewState()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	health.ReadyHandler(s)(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	assertJSONStatus(t, w, "unavailable")
}

func TestReadyHandler_ShutdownTransition(t *testing.T) {
	s := health.NewState()
	s.SetAlive()
	s.SetReady()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	health.ReadyHandler(s)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 while ready, got %d", w.Code)
	}

	s.SetNotReady()

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ready", nil)
	health.ReadyHandler(s)(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 after SetNotReady, got %d", w.Code)
	}
}

func TestPreStopHandler_MarksNotReadyAndBlocks(t *testing.T) {
	s := health.NewState()
	s.SetAlive()
	s.SetReady()

	drain := 100 * time.Millisecond
	handler := health.PreStopHandler(s, drain)

	start := time.Now()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/prestop", nil)
	handler(w, req)

	elapsed := time.Since(start)

	if s.IsReady() {
		t.Error("expected ready=false after PreStopHandler")
	}

	if s.IsAlive() != true {
		t.Error("expected alive=true, prestop should not change liveness")
	}

	if elapsed < drain {
		t.Errorf("expected handler to block at least %v, returned after %v", drain, elapsed)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	assertJSONStatus(t, w, "draining")
}

func TestPreStopHandler_ReadyProbeFailsDuringDrain(t *testing.T) {
	s := health.NewState()
	s.SetAlive()
	s.SetReady()

	drain := 200 * time.Millisecond
	prestopHandler := health.PreStopHandler(s, drain)
	readyHandler := health.ReadyHandler(s)

	done := make(chan struct{})

	go func() {
		defer close(done)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/prestop", nil)
		prestopHandler(w, req)
	}()

	// Give the goroutine a moment to mark not-ready.
	time.Sleep(20 * time.Millisecond)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readyHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected /ready to return 503 during preStop drain, got %d", w.Code)
	}

	<-done
}

func assertJSONStatus(t *testing.T, w *httptest.ResponseRecorder, expected string) {
	t.Helper()

	var resp struct {
		Status string `json:"status"`
		Uptime string `json:"uptime"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if resp.Status != expected {
		t.Errorf("expected status %q, got %q", expected, resp.Status)
	}

	if resp.Uptime == "" {
		t.Error("expected non-empty uptime field")
	}
}
