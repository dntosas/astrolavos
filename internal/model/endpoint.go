// Package model contains domain types shared across the application.
package model

import "time"

// Endpoint encapsulates information needed to run probes against a destination.
type Endpoint struct {
	URI                 string
	Interval            time.Duration
	Tag                 string
	Retries             int
	ProberType          string
	ReuseConnection     bool
	SkipTLSVerification bool
	TCPTimeout          time.Duration
	// Timeout bounds a single probe attempt (DNS, connect, TLS, request and
	// body for httpTrace; the dial for tcp).
	Timeout time.Duration
}
