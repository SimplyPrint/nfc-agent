package certs

import (
	"crypto/tls"
	"net"
	"time"
)

// tempError wraps an error to be treated as temporary by http.Server.
// This prevents the server from exiting when a connection fails during
// protocol detection (e.g., client disconnects before sending data).
type tempError struct {
	err error
}

func (e *tempError) Error() string   { return e.err.Error() }
func (e *tempError) Timeout() bool   { return false }
func (e *tempError) Temporary() bool { return true }

// MuxListener wraps a net.Listener and routes connections to either
// TLS or plain HTTP based on the first byte of the connection.
// TLS connections start with 0x16 (TLS handshake), HTTP starts with ASCII.
type MuxListener struct {
	net.Listener
	tlsConfig *tls.Config
}

// NewMuxListener creates a listener that handles both HTTP and HTTPS on the same port.
func NewMuxListener(ln net.Listener, tlsConfig *tls.Config) *MuxListener {
	return &MuxListener{
		Listener:  ln,
		tlsConfig: tlsConfig,
	}
}

// Accept waits for and returns the next connection.
// It peeks at the first byte to determine if it's TLS or plain HTTP.
func (m *MuxListener) Accept() (net.Conn, error) {
	conn, err := m.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Set a deadline for the peek to avoid hanging on slow/malicious clients
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Wrap connection to peek at first byte
	pc := &peekConn{Conn: conn}
	b, err := pc.peek()

	// Clear the deadline for normal operation
	conn.SetReadDeadline(time.Time{})

	if err != nil {
		conn.Close()
		// Wrap as temporary error so http.Server retries instead of exiting
		return nil, &tempError{err: err}
	}

	// TLS handshake starts with 0x16 (ContentType: Handshake)
	if b == 0x16 {
		// It's TLS - wrap with TLS server
		tlsConn := tls.Server(pc, m.tlsConfig)
		return tlsConn, nil
	}

	// Plain HTTP
	return pc, nil
}

// peekConn wraps a connection to allow peeking at the first byte.
type peekConn struct {
	net.Conn
	peeked bool
	first  byte
	hasErr bool
	err    error
}

// peek reads and stores the first byte without consuming it.
func (p *peekConn) peek() (byte, error) {
	if p.peeked {
		return p.first, p.err
	}

	buf := make([]byte, 1)
	n, err := p.Conn.Read(buf)
	p.peeked = true
	if err != nil {
		p.hasErr = true
		p.err = err
		return 0, err
	}
	if n > 0 {
		p.first = buf[0]
	}
	return p.first, nil
}

// Read returns the peeked byte first, then reads from the underlying connection.
func (p *peekConn) Read(b []byte) (int, error) {
	if p.hasErr {
		return 0, p.err
	}
	if p.peeked {
		p.peeked = false
		if len(b) > 0 {
			b[0] = p.first
			if len(b) == 1 {
				return 1, nil
			}
			n, err := p.Conn.Read(b[1:])
			return n + 1, err
		}
	}
	return p.Conn.Read(b)
}
