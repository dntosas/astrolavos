package machinery_test

import (
	"testing"
	"time"

	"github.com/dntosas/astrolavos/internal/machinery"
	"github.com/dntosas/astrolavos/internal/model"
)

func TestNewAstrolavos(_ *testing.T) {
	endpoints := []*model.Endpoint{
		{
			URI:        "https://example.com",
			ProberType: "httpTrace",
			Interval:   5 * time.Second,
			Retries:    1,
		},
		{
			URI:        "example.com:443",
			ProberType: "tcp",
			Interval:   5 * time.Second,
			Retries:    1,
			TCPTimeout: 10 * time.Second,
		},
	}

	_ = machinery.NewAstrolavos(3000, endpoints, "localhost", "dev", 0, true)
}
