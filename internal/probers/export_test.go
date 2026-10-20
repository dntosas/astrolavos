package probers

// Test-only exports so the external test package can drive the stream
// engine with a fake transport.

// StreamDialer is the dialer contract used by Stream.
type StreamDialer = streamDialer

// StreamConn is the connection contract used by Stream.
type StreamConn = streamConn

// ConnectTimings are the connect phases reported by a dialer.
type ConnectTimings = connectTimings

// NewTestStream builds a Stream over an arbitrary dialer.
func NewTestStream(c ProberConfig, proberType string, d StreamDialer) *Stream {
	return &Stream{ProberConfig: c, proberType: proberType, dialer: d}
}
