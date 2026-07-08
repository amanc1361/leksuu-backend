// Package ws is a tiny, dependency-free WebSocket (RFC 6455) server built on the
// standard library's http.Hijacker. It implements exactly what the live-class
// signaling needs: the upgrade handshake, masked client->server frames, and
// unmasked server->client text frames, plus ping/pong/close control frames.
//
// It is deliberately self-contained: the platform runs self-hosted in networks
// where Go module proxies and container registries are blocked, so adding an
// external WebSocket library is undesirable. Signaling messages are small text
// frames, so the minimal subset here (no permessage-deflate, no extensions) is
// sufficient and easy to audit.
package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const acceptMagic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Opcodes (RFC 6455 §5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

const maxMessageBytes = 1 << 20 // 1 MiB cap; signaling messages are tiny.

var (
	ErrNotWebSocket = errors.New("ws: not a websocket upgrade request")
	ErrClosed       = errors.New("ws: connection closed")
	ErrTooLarge     = errors.New("ws: message exceeds limit")
)

// AcceptKey computes the Sec-WebSocket-Accept value for a client key.
func AcceptKey(key string) string {
	h := sha1.New()
	io.WriteString(h, key+acceptMagic)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

type Conn struct {
	rwc         net.Conn
	br          *bufio.Reader
	wmu         sync.Mutex // serialises writes
	closed      bool
	readTimeout time.Duration
}

// Upgrade performs the RFC 6455 opening handshake and hijacks the connection.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, ErrNotWebSocket
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, ErrNotWebSocket
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("ws: response writer does not support hijacking")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + AcceptKey(key) + "\r\n\r\n"
	if _, err := brw.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := brw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return newConn(conn, brw.Reader), nil
}

func newConn(c net.Conn, br *bufio.Reader) *Conn {
	if br == nil {
		br = bufio.NewReader(c)
	}
	return &Conn{rwc: c, br: br, readTimeout: 70 * time.Second}
}

// ReadMessage returns the next complete text/binary message, transparently
// handling fragmentation and replying to ping/close control frames.
func (c *Conn) ReadMessage() ([]byte, error) {
	var msg []byte
	for {
		fin, opcode, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case opPing:
			_ = c.writeFrame(opPong, payload)
			continue
		case opPong:
			continue
		case opClose:
			_ = c.writeFrame(opClose, nil)
			c.Close()
			return nil, ErrClosed
		case opText, opBinary, opContinuation:
			msg = append(msg, payload...)
			if len(msg) > maxMessageBytes {
				c.Close()
				return nil, ErrTooLarge
			}
			if fin {
				return msg, nil
			}
		}
	}
}

func (c *Conn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	if c.readTimeout > 0 {
		_ = c.rwc.SetReadDeadline(time.Now().Add(c.readTimeout))
	}
	var hdr [2]byte
	if _, err = io.ReadFull(c.br, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	opcode = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxMessageBytes {
		err = ErrTooLarge
		return
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

// WriteText sends a single unmasked text frame.
func (c *Conn) WriteText(b []byte) error { return c.writeFrame(opText, b) }

// WritePing sends a ping control frame (keepalive).
func (c *Conn) WritePing() error { return c.writeFrame(opPing, nil) }

// SetReadTimeout bounds how long a single frame read may idle.
func (c *Conn) SetReadTimeout(d time.Duration) { c.readTimeout = d }

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return ErrClosed
	}
	var hdr []byte
	b0 := byte(0x80 | opcode) // FIN set, no RSV
	n := len(payload)
	switch {
	case n < 126:
		hdr = []byte{b0, byte(n)}
	case n < 1<<16:
		hdr = []byte{b0, 126, byte(n >> 8), byte(n)}
	default:
		hdr = make([]byte, 10)
		hdr[0], hdr[1] = b0, 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	if _, err := c.rwc.Write(hdr); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.rwc.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func (c *Conn) Close() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}
