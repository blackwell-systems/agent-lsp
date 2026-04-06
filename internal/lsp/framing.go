package lsp

import (
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
		// Try to parse from buffer
		if msg, rest, ok := tryParse(fr.buf); ok {
			fr.buf = rest
			return msg, nil
		}

		// Read more data
		n, err := fr.r.Read(tmp)
		if n > 0 {
			fr.buf = append(fr.buf, tmp[:n]...)
			// Overflow protection: discard entire buffer
			if len(fr.buf) > maxBufferSize {
				fr.buf = nil
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
	idx := -1
	for i := 0; i < len(buf)-3; i++ {
		if buf[i] == '\r' && buf[i+1] == '\n' && buf[i+2] == '\r' && buf[i+3] == '\n' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, buf, false
	}

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
