package ws

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestAcceptKeyRFCVector(t *testing.T) {
	// RFC 6455 §1.3 worked example.
	got := AcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	if got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept key = %q", got)
	}
}

// writeMaskedTextFrame writes a client->server text frame (must be masked).
func writeMaskedTextFrame(w io.Writer, payload []byte) {
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	n := len(payload)
	hdr := []byte{0x81, 0x80 | byte(n)} // FIN+text, MASK + 7-bit len (n<126 in tests)
	hdr = append(hdr, mask[:]...)
	w.Write(hdr)
	masked := make([]byte, n)
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	w.Write(masked)
}

func TestFrameRoundTrip(t *testing.T) {
	clientEnd, serverEnd := net.Pipe()
	defer clientEnd.Close()
	srv := newConn(serverEnd, nil)

	// Client sends a masked text frame; server should read it unmasked.
	go writeMaskedTextFrame(clientEnd, []byte(`{"type":"offer"}`))
	serverEnd.SetReadDeadline(time.Now().Add(time.Second))
	msg, err := srv.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(msg) != `{"type":"offer"}` {
		t.Fatalf("got %q", msg)
	}

	// Server replies; client should receive an UNMASKED text frame.
	done := make(chan string, 1)
	go func() {
		br := bufio.NewReader(clientEnd)
		h0, _ := br.ReadByte()
		h1, _ := br.ReadByte()
		if h0&0x0F != opText {
			done <- "bad-opcode"
			return
		}
		if h1&0x80 != 0 {
			done <- "server-frame-must-not-be-masked"
			return
		}
		n := int(h1 & 0x7F)
		if n == 126 {
			var ext [2]byte
			io.ReadFull(br, ext[:])
			n = int(binary.BigEndian.Uint16(ext[:]))
		}
		buf := make([]byte, n)
		io.ReadFull(br, buf)
		done <- string(buf)
	}()
	if err := srv.WriteText([]byte("pong-reply")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case got := <-done:
		if got != "pong-reply" {
			t.Fatalf("client got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client frame")
	}
}
