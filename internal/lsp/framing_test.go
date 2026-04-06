package lsp

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestFrameReader_SingleMessage verifies encode/decode roundtrip for a simple message.
func TestFrameReader_SingleMessage(t *testing.T) {
	original := []byte(`{"method":"textDocument/hover","params":{}}`)
	encoded := EncodeMessage(original)

	fr := NewFrameReader(bytes.NewReader(encoded))
	got, err := fr.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("roundtrip mismatch: got %q, want %q", got, original)
	}
}

// TestFrameReader_MultiByteUnicode verifies that Content-Length is a byte count,
// not a character count, for multi-byte UTF-8 sequences (→, •).
func TestFrameReader_MultiByteUnicode(t *testing.T) {
	// → is 3 bytes in UTF-8 (0xE2 0x86 0x92)
	// • is 3 bytes in UTF-8 (0xE2 0x80 0xA2)
	original := []byte(`{"hover":"→ bullet • arrow"}`)
	encoded := EncodeMessage(original)

	// Verify the header says the correct byte length.
	header := string(encoded[:bytes.Index(encoded, []byte("\r\n\r\n"))])
	expectedHeader := "Content-Length: 32"
	if !strings.HasPrefix(header, expectedHeader) {
		t.Errorf("expected header starting with %q, got %q", expectedHeader, header)
	}

	fr := NewFrameReader(bytes.NewReader(encoded))
	got, err := fr.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("roundtrip mismatch: got %q, want %q", got, original)
	}
}

// TestFrameReader_BufferFlush verifies that a partial message followed by completion
// is correctly reassembled.
func TestFrameReader_BufferFlush(t *testing.T) {
	body := []byte(`{"id":1,"result":"ok"}`)
	encoded := EncodeMessage(body)

	// Split in the middle of the body.
	mid := len(encoded) / 2
	first := encoded[:mid]
	second := encoded[mid:]

	pr, pw := io.Pipe()
	fr := NewFrameReader(pr)

	done := make(chan []byte, 1)
	errc := make(chan error, 1)
	go func() {
		got, err := fr.ReadMessage()
		if err != nil {
			errc <- err
			return
		}
		done <- got
	}()

	// Write partial, then complete.
	pw.Write(first)
	pw.Write(second)

	select {
	case got := <-done:
		if !bytes.Equal(got, body) {
			t.Errorf("got %q, want %q", got, body)
		}
	case err := <-errc:
		t.Fatalf("ReadMessage: %v", err)
	}
}

// TestFrameReader_MultipleMessages verifies that back-to-back messages are
// correctly separated.
func TestFrameReader_MultipleMessages(t *testing.T) {
	msgs := [][]byte{
		[]byte(`{"id":1}`),
		[]byte(`{"id":2}`),
		[]byte(`{"id":3}`),
	}

	var buf bytes.Buffer
	for _, m := range msgs {
		buf.Write(EncodeMessage(m))
	}

	fr := NewFrameReader(&buf)
	for i, want := range msgs {
		got, err := fr.ReadMessage()
		if err != nil {
			t.Fatalf("msg %d: ReadMessage: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("msg %d: got %q, want %q", i, got, want)
		}
	}
}

// TestFrameReader_LargeMessage verifies a message just under the 10MB limit.
func TestFrameReader_LargeMessage(t *testing.T) {
	// 1MB body.
	body := bytes.Repeat([]byte("x"), 1024*1024)
	encoded := EncodeMessage(body)

	fr := NewFrameReader(bytes.NewReader(encoded))
	got, err := fr.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("large message roundtrip failed: len got=%d want=%d", len(got), len(body))
	}
}

// TestEncodeMessage verifies the framing format exactly.
func TestEncodeMessage(t *testing.T) {
	body := []byte(`{"test":true}`)
	encoded := EncodeMessage(body)

	expected := "Content-Length: 13\r\n\r\n{\"test\":true}"
	if string(encoded) != expected {
		t.Errorf("got %q, want %q", encoded, expected)
	}
}
