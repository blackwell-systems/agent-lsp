package lsp

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxBufferSize = 10 * 1024 * 1024 // 10MB

// EncodeMessage encodes a JSON-RPC message with Content-Length header.
// Returns "Content-Length: N\r\n\r\n" + JSON body as bytes.
func EncodeMessage(body []byte) []byte {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	result := make([]byte, len(header)+len(body))
	copy(result, header)
	copy(result[len(header):], body)
	return result
}

// FrameReader reads Content-Length-framed messages from an io.Reader.
type FrameReader struct {
	r   io.Reader
	buf []byte
	// scanFrom is where the next search for the header terminator starts.
	// Searching the whole accumulated buffer on every read was O(n^2) on a
	// large or slow stream without a terminator (10 MB in 100-byte reads took
	// minutes; issue #65). It is the header's own index once found (the body
	// may still be arriving) and is reset whenever buf is replaced.
	scanFrom int
}

// NewFrameReader creates a new FrameReader wrapping r.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: r}
}

// ReadMessage blocks until a complete message is available.
// Returns raw JSON body bytes or error.
func (fr *FrameReader) ReadMessage() ([]byte, error) {
	tmp := make([]byte, 4096)
	for {
		// Try to parse from buffer, resuming the header search where the
		// previous attempt stopped.
		if idx := findHeaderEnd(fr.buf, fr.scanFrom); idx >= 0 {
			if msg, rest, ok := parseFrame(fr.buf, idx); ok {
				fr.buf = rest
				fr.scanFrom = 0
				return msg, nil
			}
			fr.scanFrom = idx // header complete, body still arriving
		} else {
			// Back off so a terminator split across reads is still found.
			fr.scanFrom = max(0, len(fr.buf)-3)
		}

		// Read more data
		n, err := fr.r.Read(tmp)
		if n > 0 {
			fr.buf = append(fr.buf, tmp[:n]...)
			// Overflow protection: discard entire buffer
			if len(fr.buf) > maxBufferSize {
				fr.buf = nil
				fr.scanFrom = 0
			}
		}
		if err != nil {
			return nil, err
		}
	}
}

// tryParse attempts to parse one complete Content-Length framed message from buf.
// Returns (body, remaining, ok).
func tryParse(buf []byte) ([]byte, []byte, bool) {
	idx := findHeaderEnd(buf, 0)
	if idx < 0 {
		return nil, buf, false
	}
	return parseFrame(buf, idx)
}

var headerTerminator = []byte("\r\n\r\n")

// findHeaderEnd returns the index of the first header terminator in buf at or
// after from, or -1 if there is none yet.
func findHeaderEnd(buf []byte, from int) int {
	if from < 0 || from > len(buf) {
		from = 0
	}
	if i := bytes.Index(buf[from:], headerTerminator); i >= 0 {
		return from + i
	}
	return -1
}

// parseFrame parses the message whose header ends at idx (the index of its
// terminator). Returns (body, remaining, ok); ok is false until the whole body
// has arrived or when the header has no usable Content-Length.
func parseFrame(buf []byte, idx int) ([]byte, []byte, bool) {
	header := string(buf[:idx])
	bodyStart := idx + 4

	// Parse Content-Length from header lines
	contentLength := -1
	for _, line := range strings.Split(header, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			valStr := strings.TrimSpace(line[len("content-length:"):])
			v, err := strconv.Atoi(valStr)
			if err == nil {
				contentLength = v
			}
			break
		}
	}
	if contentLength < 0 {
		return nil, buf, false
	}

	if len(buf) < bodyStart+contentLength {
		return nil, buf, false
	}

	body := make([]byte, contentLength)
	copy(body, buf[bodyStart:bodyStart+contentLength])
	remaining := make([]byte, len(buf)-(bodyStart+contentLength))
	copy(remaining, buf[bodyStart+contentLength:])
	return body, remaining, true
}
