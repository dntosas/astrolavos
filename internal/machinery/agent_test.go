package machinery

import (
	"testing"

	"github.com/dntosas/astrolavos/internal/model"
)

func TestNewAgent_CreatesProbers(t *testing.T) {
	endpoints := []*model.Endpoint{
		{
			URI:        "https://example.com",
			ProberType: "httpTrace",
			Retries:    1,
		},
		{
			URI:        "example.com:443",
			ProberType: "tcp",
			Retries:    1,
		},
	}

	a := newAgent(endpoints, true, nil)

	if len(a.probers) != 2 {
		t.Errorf("expected 2 probers, got %d", len(a.probers))
	}
}

func TestNewAgent_SkipsUnknownProberType(t *testing.T) {
	endpoints := []*model.Endpoint{
		{
			URI:        "https://example.com",
			ProberType: "httpTrace",
			Retries:    1,
		},
		{
			URI:        "example.com:443",
			ProberType: "unknown",
			Retries:    1,
		},
	}

	a := newAgent(endpoints, true, nil)

	if len(a.probers) != 1 {
		t.Errorf("expected 1 prober (unknown type skipped), got %d", len(a.probers))
	}
}

func TestNewAgent_EmptyEndpoints(t *testing.T) {
	endpoints := []*model.Endpoint{}

	a := newAgent(endpoints, true, nil)

	if len(a.probers) != 0 {
		t.Errorf("expected 0 probers, got %d", len(a.probers))
	}
}
