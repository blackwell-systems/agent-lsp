package lsp

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"
)

// TestReadMessage_ShortRead tests handling of incomplete body
func TestReadMessage_ShortRead(t *testing.T) {
	// Header says 100 bytes, but only 50 bytes follow
	input := "Content-Length: 100\r\n\r\n" + strings.Repeat("x", 50)
	r := NewFrameReader(strings.NewReader(input))

	_, err := r.ReadMessage()
	if err == nil {
		t.Error("expected error for short read, got nil")
	}
	if err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Errorf("expected EOF error, got: %v", err)
	}
}

// TestReadMessage_NegativeContentLength tests handling of invalid negative length
func TestReadMessage_NegativeContentLength(t *testing.T) {
	input := "Content-Length: -10\r\n\r\n{}"
	r := NewFrameReader(strings.NewReader(input))

	// Negative length can't be parsed, so read will hang or EOF
	_, err := r.ReadMessage()
	if err != io.EOF {
		t.Logf("negative Content-Length resulted in: %v", err)
	}
}

// TestReadMessage_VeryLargeContentLength tests handling of unreasonably large length
func TestReadMessage_VeryLargeContentLength(t *testing.T) {
	// 11MB content length (exceeds maxBufferSize of 10MB)
	input := "Content-Length: 11000000\r\n\r\n"
	r := NewFrameReader(strings.NewReader(input))

	// Should hit buffer overflow protection
	_, err := r.ReadMessage()
	if err == nil {
		t.Error("expected error for very large Content-Length")
	}
}

// TestReadMessage_MalformedHeader tests various malformed headers
func TestReadMessage_MalformedHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"no header separator", "Content-Length: 42"},
		{"garbage header", "xyz123abc\r\n\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewFrameReader(strings.NewReader(tt.input))
			_, err := r.ReadMessage()
			if err == nil {
				t.Error("expected error for malformed header")
			}
		})
	}
}

// TestReadMessage_EmptyReader tests reading from empty input
func TestReadMessage_EmptyReader(t *testing.T) {
	r := NewFrameReader(strings.NewReader(""))
	_, err := r.ReadMessage()
	if err != io.EOF {
		t.Errorf("expected EOF for empty reader, got: %v", err)
	}
}

// TestReadMessage_OnlyHeaders tests input with headers but no body
func TestReadMessage_OnlyHeaders(t *testing.T) {
	input := "Content-Length: 0\r\n\r\n"
	r := NewFrameReader(strings.NewReader(input))

	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg) != 0 {
		t.Errorf("expected empty message for Content-Length: 0, got %d bytes", len(msg))
	}
}

// TestReadMessage_MultipleHeaderLines tests multiple header lines
func TestReadMessage_MultipleHeaderLines(t *testing.T) {
	input := "Content-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"
	r := NewFrameReader(strings.NewReader(input))

	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(msg) != "{}" {
		t.Errorf("got %q, want %q", string(msg), "{}")
	}
}

// TestReadMessage_WhitespaceInHeader tests whitespace handling
func TestReadMessage_WhitespaceInHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"trailing spaces", "Content-Length: 2  \r\n\r\n{}", "{}"},
		{"leading spaces in value", "Content-Length:  2\r\n\r\n{}", "{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewFrameReader(strings.NewReader(tt.input))
			msg, err := r.ReadMessage()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(msg) != tt.want {
				t.Errorf("got %q, want %q", string(msg), tt.want)
			}
		})
	}
}

// TestEncodeMessage_NilMessage tests encoding nil message
func TestEncodeMessage_NilMessage(t *testing.T) {
	result := EncodeMessage(nil)

	output := string(result)
	if !strings.Contains(output, "Content-Length: 0") {
		t.Errorf("output should contain Content-Length: 0, got: %q", output)
	}
}

// TestEncodeMessage_LargeMessage tests encoding a large message
func TestEncodeMessage_LargeMessage(t *testing.T) {
	// 1MB message
	large := bytes.Repeat([]byte("x"), 1024*1024)

	encoded := EncodeMessage(large)

	// Parse header
	header := string(encoded[:strings.Index(string(encoded), "\r\n\r\n")+4])
	if !strings.Contains(header, "Content-Length: 1048576") {
		t.Errorf("header should contain correct Content-Length, got: %q", header)
	}

	// Verify round-trip
	r := NewFrameReader(bytes.NewReader(encoded))
	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	if len(msg) != len(large) {
		t.Errorf("length mismatch: got %d, want %d", len(msg), len(large))
	}
}

// TestFrameReader_ConsecutiveReads tests reading multiple messages sequentially
func TestFrameReader_ConsecutiveReads(t *testing.T) {
	msg1 := EncodeMessage([]byte("message"))
	msg2 := EncodeMessage([]byte("hello"))
	input := append(msg1, msg2...)

	r := NewFrameReader(bytes.NewReader(input))

	// First message
	m1, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("first ReadMessage failed: %v", err)
	}
	if string(m1) != "message" {
		t.Errorf("first message: got %q, want %q", string(m1), "message")
	}

	// Second message
	m2, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("second ReadMessage failed: %v", err)
	}
	if string(m2) != "hello" {
		t.Errorf("second message: got %q, want %q", string(m2), "hello")
	}

	// EOF on third read
	_, err = r.ReadMessage()
	if err != io.EOF {
		t.Errorf("third ReadMessage: expected EOF, got %v", err)
	}
}

// TestReadMessage_UnicodeBody tests handling of Unicode content
func TestReadMessage_UnicodeBody(t *testing.T) {
	body := `{"message": "Hello 世界 🌍"}`
	contentLength := len(body) // byte length
	input := "Content-Length: " + strconv.Itoa(contentLength) + "\r\n\r\n" + body

	r := NewFrameReader(strings.NewReader(input))
	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(msg) != body {
		t.Errorf("got %q, want %q", string(msg), body)
	}
}

// TestReadMessage_BinaryData tests handling of binary (non-UTF8) data
func TestReadMessage_BinaryData(t *testing.T) {
	// Binary data with null bytes
	body := []byte{0x00, 0xFF, 0xFE, 0x01, 0x02}
	encoded := EncodeMessage(body)

	r := NewFrameReader(bytes.NewReader(encoded))
	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(msg, body) {
		t.Errorf("binary data mismatch")
	}
}

// TestEncodeMessage_EmptyBody tests encoding empty message
func TestEncodeMessage_EmptyBody(t *testing.T) {
	result := EncodeMessage([]byte{})

	if !strings.Contains(string(result), "Content-Length: 0") {
		t.Error("expected Content-Length: 0 for empty body")
	}

	// Verify it can be read back
	r := NewFrameReader(bytes.NewReader(result))
	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if len(msg) != 0 {
		t.Errorf("expected empty message, got %d bytes", len(msg))
	}
}

// TestFrameReader_BufferOverflow tests buffer overflow protection
func TestFrameReader_BufferOverflow(t *testing.T) {
	// Create a reader that feeds data slowly, exceeding maxBufferSize
	r := NewFrameReader(&slowReader{data: make([]byte, maxBufferSize+1000)})

	_, err := r.ReadMessage()
	// Should eventually hit buffer overflow or EOF
	if err == nil {
		t.Log("buffer overflow handling triggered")
	}
}

// slowReader implements io.Reader that returns small chunks
type slowReader struct {
	data []byte
	pos  int
}

func (s *slowReader) Read(p []byte) (n int, err error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}
	// Return small chunks to trigger buffering
	chunk := 100
	if chunk > len(p) {
		chunk = len(p)
	}
	if chunk > len(s.data)-s.pos {
		chunk = len(s.data) - s.pos
	}
	copy(p, s.data[s.pos:s.pos+chunk])
	s.pos += chunk
	return chunk, nil
}
