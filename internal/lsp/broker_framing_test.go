package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestReadFramedMessage_SingleMessage verifies basic Content-Length framing roundtrip.
func TestReadFramedMessage_SingleMessage(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"initialize"}`
	msg := "Content-Length: " + itoa(len(body)) + "\r\n\r\n" + body

	reader := bufio.NewReader(strings.NewReader(msg))
	got, err := readFramedMessage(reader)
	if err != nil {
		t.Fatalf("readFramedMessage: %v", err)
	}
	if string(got) != body {
		t.Errorf("got %q, want %q", got, body)
	}
}

// TestReadFramedMessage_MultipleHeaders verifies that non-Content-Length headers are ignored.
func TestReadFramedMessage_MultipleHeaders(t *testing.T) {
	body := `{"id":1}`
	msg := "Content-Type: application/json\r\nContent-Length: " + itoa(len(body)) + "\r\n\r\n" + body

	reader := bufio.NewReader(strings.NewReader(msg))
	got, err := readFramedMessage(reader)
	if err != nil {
		t.Fatalf("readFramedMessage: %v", err)
	}
	if string(got) != body {
		t.Errorf("got %q, want %q", got, body)
	}
}

// TestReadFramedMessage_NoContentLength verifies error on missing Content-Length.
func TestReadFramedMessage_NoContentLength(t *testing.T) {
	msg := "Content-Type: application/json\r\n\r\n{}"
	reader := bufio.NewReader(strings.NewReader(msg))
	_, err := readFramedMessage(reader)
	if err == nil {
		t.Fatal("expected error for missing Content-Length")
	}
	if !strings.Contains(err.Error(), "no Content-Length") {
		t.Errorf("expected 'no Content-Length' error, got: %v", err)
	}
}

// TestReadFramedMessage_InvalidContentLength verifies error on non-numeric Content-Length.
func TestReadFramedMessage_InvalidContentLength(t *testing.T) {
	msg := "Content-Length: abc\r\n\r\n{}"
	reader := bufio.NewReader(strings.NewReader(msg))
	_, err := readFramedMessage(reader)
	if err == nil {
		t.Fatal("expected error for invalid Content-Length")
	}
	if !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Errorf("expected 'invalid Content-Length' error, got: %v", err)
	}
}

// TestReadFramedMessage_EOF verifies error on empty input.
func TestReadFramedMessage_EOF(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(""))
	_, err := readFramedMessage(reader)
	if err == nil {
		t.Fatal("expected error on empty input")
	}
}

// TestWriteFramedMessage_Roundtrip verifies write then read roundtrip.
func TestWriteFramedMessage_Roundtrip(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":42,"result":null}`)

	var buf bytes.Buffer
	if err := writeFramedMessage(&buf, body); err != nil {
		t.Fatalf("writeFramedMessage: %v", err)
	}

	reader := bufio.NewReader(&buf)
	got, err := readFramedMessage(reader)
	if err != nil {
		t.Fatalf("readFramedMessage: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("roundtrip mismatch: got %q, want %q", got, body)
	}
}

// TestWriteFramedMessage_Format verifies the exact framing output.
func TestWriteFramedMessage_Format(t *testing.T) {
	body := []byte(`{"test":true}`)
	var buf bytes.Buffer
	if err := writeFramedMessage(&buf, body); err != nil {
		t.Fatalf("writeFramedMessage: %v", err)
	}

	expected := "Content-Length: 13\r\n\r\n{\"test\":true}"
	if buf.String() != expected {
		t.Errorf("got %q, want %q", buf.String(), expected)
	}
}

// TestWriteFramedMessage_EmptyBody verifies framing of an empty body.
func TestWriteFramedMessage_EmptyBody(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFramedMessage(&buf, []byte{}); err != nil {
		t.Fatalf("writeFramedMessage: %v", err)
	}

	expected := "Content-Length: 0\r\n\r\n"
	if buf.String() != expected {
		t.Errorf("got %q, want %q", buf.String(), expected)
	}
}

// TestWriteFramedMessage_MultipleMessages verifies multiple messages written sequentially.
func TestWriteFramedMessage_MultipleMessages(t *testing.T) {
	bodies := [][]byte{
		[]byte(`{"id":1}`),
		[]byte(`{"id":2}`),
		[]byte(`{"id":3}`),
	}

	var buf bytes.Buffer
	for _, b := range bodies {
		if err := writeFramedMessage(&buf, b); err != nil {
			t.Fatalf("writeFramedMessage: %v", err)
		}
	}

	reader := bufio.NewReader(&buf)
	for i, want := range bodies {
		got, err := readFramedMessage(reader)
		if err != nil {
			t.Fatalf("msg %d: readFramedMessage: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("msg %d: got %q, want %q", i, got, want)
		}
	}
}

// TestReadFramedMessage_LFOnly verifies reading with LF-only line endings (no CR).
func TestReadFramedMessage_LFOnly(t *testing.T) {
	body := `{"ok":true}`
	// Some implementations may send LF-only (non-standard but occurs in practice).
	msg := "Content-Length: " + itoa(len(body)) + "\n\n" + body

	reader := bufio.NewReader(strings.NewReader(msg))
	got, err := readFramedMessage(reader)
	if err != nil {
		t.Fatalf("readFramedMessage with LF-only: %v", err)
	}
	if string(got) != body {
		t.Errorf("got %q, want %q", got, body)
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
