package internet

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const tdzPayloadMaxHeader = 65536
const tdzPayloadChunkSize = 4096
const tdzPayloadTimeout = 8 * time.Second
const tdzPayloadResponse = "HTTP/1.1 101 <b><font color=\"red\" size=\"7\">Script By:</font> <font color=\"#0057B7\" size=\"7\">tuhinbro.com</font></b>\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"

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

// Sniff only the first handshake. Native binary/TLS and HTTP/2 are replayed
// byte-for-byte; native HTTP transport requests take precedence over a payload
// with identical bytes. The protocol layer still performs authentication.
func tdzAcceptAutoPayload(conn net.Conn, network, path string, timeout time.Duration) (net.Conn, error) {
	data := make([]byte, 0, tdzPayloadChunkSize)
	chunk := make([]byte, tdzPayloadChunkSize)
	defer conn.SetReadDeadline(time.Time{})
	for {
		if len(data) >= tdzPayloadMaxHeader {
			return nil, errors.New("payload header too large")
		}
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return nil, err
		}
		n, err := conn.Read(chunk)
		data = append(data, chunk[:n]...)
		end := bytes.Index(data, []byte("\r\n\r\n"))
		prefix := data
		if end >= 0 {
			prefix = data[:end+4]
		}
		native := bytes.HasPrefix(data, []byte("PRI * HTTP/2.0"))
		for _, b := range prefix {
			if b != '\r' && b != '\n' && b != '\t' && (b < 32 || b > 126) {
				native = true
				break
			}
		}
		if native || end >= 0 && tdzNativeHTTP(prefix, network, path) {
			return &tdzPayloadConn{Conn: conn, pending: append([]byte(nil), data...), readErr: err}, nil
		}
		if end >= 0 {
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

func tdzNativeHTTP(header []byte, network, path string) bool {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(header)))
	if err != nil {
		return false
	}
	if req.Header.Get("X-TDZ-Payload") == "1" {
		return false
	}
	if network == "splithttp" {
		return req.Method != "CONNECT" && strings.HasPrefix(req.URL.Path, path)
	}
	if network != "websocket" && network != "httpupgrade" {
		return false
	}
	if req.Method != "GET" || req.URL.Path != path || !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		return false
	}
	upgrade := false
	for _, token := range strings.Split(req.Header.Get("Connection"), ",") {
		upgrade = upgrade || strings.EqualFold(strings.TrimSpace(token), "upgrade")
	}
	if !upgrade {
		return false
	}
	if network == "httpupgrade" {
		return true
	}
	key, err := base64.StdEncoding.DecodeString(req.Header.Get("Sec-WebSocket-Key"))
	return err == nil && len(key) == 16 && req.Header.Get("Sec-WebSocket-Version") == "13"
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
type tdzPayloadListener struct {
	net.Listener
	network string
	path    string
}

func TDZPayloadListener(listener net.Listener, enabled bool, transport ...string) net.Listener {
	if !enabled {
		return listener
	}
	wrapped := &tdzPayloadListener{Listener: listener}
	if len(transport) > 0 {
		wrapped.network = transport[0]
	}
	if len(transport) > 1 {
		wrapped.path = transport[1]
	}
	return wrapped
}

func (l *tdzPayloadListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &tdzPayloadLazyConn{Conn: conn, network: l.network, path: l.path}, nil
}

type tdzPayloadLazyConn struct {
	net.Conn
	once    sync.Once
	stream  net.Conn
	err     error
	network string
	path    string
}

func (c *tdzPayloadLazyConn) init() {
	c.once.Do(func() {
		c.stream, c.err = tdzAcceptAutoPayload(c.Conn, c.network, c.path, tdzPayloadTimeout)
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
