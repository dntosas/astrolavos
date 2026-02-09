package metrics

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "unknown",
		},
		{
			name:     "dns no such host",
			err:      fmt.Errorf("lookup example.com: no such host"),
			expected: "dns_error",
		},
		{
			name:     "dns generic",
			err:      fmt.Errorf("DNS resolution failed: server misbehaving"),
			expected: "dns_error",
		},
		{
			name:     "connection refused",
			err:      fmt.Errorf("dial tcp 1.2.3.4:443: connection refused"),
			expected: "connection_refused",
		},
		{
			name:     "timeout string",
			err:      fmt.Errorf("i/o timeout"),
			expected: "timeout",
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: "timeout",
		},
		{
			name:     "wrapped deadline exceeded",
			err:      fmt.Errorf("request failed: %w", context.DeadlineExceeded),
			expected: "timeout",
		},
		{
			name:     "tls handshake failure",
			err:      fmt.Errorf("tls: handshake failure"),
			expected: "tls_error",
		},
		{
			name:     "x509 certificate error",
			err:      fmt.Errorf("x509: certificate signed by unknown authority"),
			expected: "tls_error",
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: "canceled",
		},
		{
			name:     "wrapped context canceled",
			err:      fmt.Errorf("operation failed: %w", context.Canceled),
			expected: "canceled",
		},
		{
			name:     "connection reset",
			err:      fmt.Errorf("read: connection reset by peer"),
			expected: "connection_reset",
		},
		{
			name:     "unexpected eof",
			err:      fmt.Errorf("unexpected EOF"),
			expected: "eof",
		},
		{
			name:     "unknown error",
			err:      errors.New("something went wrong"),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CategorizeError(tt.err)
			if result != tt.expected {
				t.Errorf("CategorizeError(%v) = %q, want %q", tt.err, result, tt.expected)
			}
		})
	}
}
