package config //nolint:testpackage // tests access unexported methods for thorough validation

import (
	"testing"
	"time"
)

func TestGetCleanEndpoint_Defaults(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com",
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.ProberType != "httpTrace" {
		t.Errorf("expected prober type 'httpTrace', got %q", ep.ProberType)
	}

	if ep.Retries != 1 {
		t.Errorf("expected retries 1, got %d", ep.Retries)
	}

	if ep.URI != "http://example.com" {
		t.Errorf("expected URI 'http://example.com', got %q", ep.URI)
	}

	expectedInterval := 5000 * time.Millisecond
	if ep.Interval != expectedInterval {
		t.Errorf("expected interval %v, got %v", expectedInterval, ep.Interval)
	}
}

func TestGetCleanEndpoint_HTTPS(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com",
		HTTPS:  true,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.URI != "https://example.com" {
		t.Errorf("expected URI 'https://example.com', got %q", ep.URI)
	}
}

func TestGetCleanEndpoint_TCP(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com:443",
		Prober: "tcp",
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.URI != "example.com:443" {
		t.Errorf("expected URI 'example.com:443', got %q", ep.URI)
	}

	if ep.ProberType != "tcp" {
		t.Errorf("expected prober type 'tcp', got %q", ep.ProberType)
	}
}

func TestGetCleanEndpoint_CustomRetries(t *testing.T) {
	retries := 5
	ye := &YamlEndpoint{
		Domain:  "example.com",
		Retries: &retries,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.Retries != 5 {
		t.Errorf("expected retries 5, got %d", ep.Retries)
	}
}

func TestGetCleanEndpoint_IntervalTooSmall(t *testing.T) {
	interval := 500 * time.Millisecond
	ye := &YamlEndpoint{
		Domain:   "example.com",
		Interval: &interval,
	}

	_, err := ye.getCleanEndpoint()
	if err == nil {
		t.Fatal("expected error for interval too small")
	}
}

func TestGetCleanEndpoint_InvalidProber(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com",
		Prober: "invalid",
	}

	_, err := ye.getCleanEndpoint()
	if err == nil {
		t.Fatal("expected error for invalid prober type")
	}
}

func TestGetCleanEndpoints_Empty(t *testing.T) {
	ye := &YamlEndpoints{}

	_, err := ye.getCleanEndpoints()
	if err == nil {
		t.Fatal("expected error for empty endpoints")
	}
}

func TestGetCleanEndpoints_AllInvalid(t *testing.T) {
	ye := &YamlEndpoints{
		Endpoints: []YamlEndpoint{
			{Domain: "example.com", Prober: "invalid"},
		},
	}

	_, err := ye.getCleanEndpoints()
	if err == nil {
		t.Fatal("expected error when all endpoints are invalid")
	}
}

func TestGetCleanEndpoints_MixedValid(t *testing.T) {
	ye := &YamlEndpoints{
		Endpoints: []YamlEndpoint{
			{Domain: "example.com"},
			{Domain: "invalid.com", Prober: "invalid"},
		},
	}

	endpoints, err := ye.getCleanEndpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(endpoints) != 1 {
		t.Errorf("expected 1 valid endpoint, got %d", len(endpoints))
	}
}

func TestGetCleanEndpoint_ReuseConnection(t *testing.T) {
	ye := &YamlEndpoint{
		Domain:          "example.com",
		ReuseConnection: true,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ep.ReuseConnection {
		t.Error("expected ReuseConnection to be true")
	}
}

func TestGetCleanEndpoint_SkipTLSVerification(t *testing.T) {
	ye := &YamlEndpoint{
		Domain:              "example.com",
		SkipTLSVerification: true,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ep.SkipTLSVerification {
		t.Error("expected SkipTLSVerification to be true")
	}
}
