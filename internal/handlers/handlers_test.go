package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOKHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()

	OKHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if cl := w.Header().Get("Content-Length"); cl != "0" {
		t.Errorf("expected Content-Length '0', got %q", cl)
	}
}

func TestLatencyHandler_NoPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency", nil)
	w := httptest.NewRecorder()

	LatencyHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestLatencyHandler_ValidPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=100", nil)
	w := httptest.NewRecorder()

	LatencyHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.Len() != 100 {
		t.Errorf("expected body length 100, got %d", w.Body.Len())
	}
}

func TestLatencyHandler_InvalidPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=abc", nil)
	w := httptest.NewRecorder()

	LatencyHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestLatencyHandler_ExceedsMaxPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=99999999", nil)
	w := httptest.NewRecorder()

	LatencyHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if w.Body.Len() == 0 {
		t.Error("expected error message in body")
	}
}

func TestLatencyHandler_ZeroPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=0", nil)
	w := httptest.NewRecorder()

	LatencyHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}
