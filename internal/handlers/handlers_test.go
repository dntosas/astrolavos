package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dntosas/astrolavos/internal/handlers"
	"github.com/dntosas/astrolavos/internal/model"
)

func TestOKHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()

	handlers.OKHandler(w, req)

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

	handler := handlers.NewLatencyHandler(0)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestLatencyHandler_ValidPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=100", nil)
	w := httptest.NewRecorder()

	handler := handlers.NewLatencyHandler(0)
	handler(w, req)

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

	handler := handlers.NewLatencyHandler(0)
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestLatencyHandler_ExceedsMaxPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=99999999", nil)
	w := httptest.NewRecorder()

	handler := handlers.NewLatencyHandler(0)
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if w.Body.Len() == 0 {
		t.Error("expected error message in body")
	}
}

func TestLatencyHandler_CustomMaxPayload(t *testing.T) {
	// Custom limit of 50 bytes
	handler := handlers.NewLatencyHandler(50)

	// Under limit: should work
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=30", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Over limit: should fail
	req = httptest.NewRequest(http.MethodGet, "/latency?payloadSize=100", nil)
	w = httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestLatencyHandler_ZeroPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/latency?payloadSize=0", nil)
	w := httptest.NewRecorder()

	handler := handlers.NewLatencyHandler(0)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestStatusHandler(t *testing.T) {
	endpoints := []*model.Endpoint{
		{
			URI:        "https://example.com",
			ProberType: "httpTrace",
			Interval:   5 * time.Second,
			Retries:    3,
			Tag:        "prod",
		},
		{
			URI:        "db.internal:5432",
			ProberType: "tcp",
			Interval:   10 * time.Second,
			Retries:    1,
		},
	}

	handler := handlers.NewStatusHandler("v1.0.0", endpoints)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if resp["version"] != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got %v", resp["version"])
	}

	eps, ok := resp["endpoints"].([]interface{})
	if !ok {
		t.Fatal("expected endpoints to be an array")
	}

	if len(eps) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(eps))
	}
}
