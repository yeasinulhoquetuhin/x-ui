package internet

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const tdzPayloadMaxHeader = 65536
const tdzPayloadChunkSize = 4096
const tdzPayloadTimeout = 8 * time.Second
const tdzPayloadResponse = "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"

// Match resources/payload-proxy.sh: consume exactly one opaque header ending
// in CRLFCRLF, send the fixed TDZ response, then expose every remaining byte.
// Do not parse methods, Host, CONNECT targets, WebSocket keys or subsequent
// headers. Transport and user authentication remain the inbound's job.
func tdzAcceptPayload(conn net.Conn) (net.Conn, error) {
	return tdzAcceptPayloadTimeout(conn, tdzPayloadTimeout)
}

func tdzAcceptPayloadTimeout(conn net.Conn, timeout time.Duration) (net.Conn, error) {
	data := make([]byte, 0, tdzPayloadChunkSize)
	chunk := make([]byte, tdzPayloadChunkSize)
	defer conn.SetReadDeadline(time.Time{})
	for {
		// The legacy proxy checks the accumulated length before each read.
		if len(data) >= tdzPayloadMaxHeader {
			return nil, errors.New("payload header too large")
		}
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return nil, err
		}
		n, err := conn.Read(chunk)
		data = append(data, chunk[:n]...)
		if end := bytes.Index(data, []byte("\r\n\r\n")); end >= 0 {
			if err := conn.SetReadDeadline(time.Time{}); err != nil {
				return nil, err
			}
			response := []byte(tdzPayloadResponse)
			for len(response) > 0 {
				written, writeErr := conn.Write(response)
				if writeErr != nil {
					return nil, writeErr
				}
				if written == 0 {
					return nil, io.ErrShortWrite
				}
				response = response[written:]
			}
			return &tdzPayloadConn{Conn: conn, pending: append([]byte(nil), data[end+4:]...), readErr: err}, nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

type tdzPayloadConn struct {
	net.Conn
	pending []byte
	readErr error
}

func (c *tdzPayloadConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		return n, nil
	}
	if c.readErr != nil {
		err := c.readErr
		c.readErr = nil
		return 0, err
	}
	return c.Conn.Read(p)
}

// Parsing runs in each transport's connection worker, never in the listener's
// Accept loop, so an incomplete header cannot block other clients.
type tdzPayloadListener struct{ net.Listener }

func TDZPayloadListener(listener net.Listener, enabled bool) net.Listener {
	if !enabled {
		return listener
	}
	return &tdzPayloadListener{Listener: listener}
}

func (l *tdzPayloadListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &tdzPayloadLazyConn{Conn: conn}, nil
}

type tdzPayloadLazyConn struct {
	net.Conn
	once   sync.Once
	stream net.Conn
	err    error
}

func (c *tdzPayloadLazyConn) init() {
	c.once.Do(func() {
		c.stream, c.err = tdzAcceptPayload(c.Conn)
		if c.err != nil {
			c.Conn.Close()
		}
	})
}

func (c *tdzPayloadLazyConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.init()
	if c.err != nil {
		return 0, c.err
	}
	return c.stream.Read(p)
}

func (c *tdzPayloadLazyConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.init()
	if c.err != nil {
		return 0, c.err
	}
	return c.stream.Write(p)
}
