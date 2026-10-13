package lsp

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// chunkReader returns data in fixed-size chunks, so header terminators and
// bodies straddle read boundaries.
type chunkReader struct {
	data  []byte
	pos   int
	chunk int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.pos >= len(c.data) {
		return 0, io.EOF
	}
	n := min(c.chunk, len(p), len(c.data)-c.pos)
	copy(p, c.data[c.pos:c.pos+n])
	c.pos += n
	return n, nil
}

// The resumable header search (issue #65) must still find a terminator split
// across reads, wait correctly for a body that arrives after its header, and
// parse back-to-back messages, at every chunk size.
func TestFrameReader_ResumableScanAcrossChunkSizes(t *testing.T) {
	bodies := [][]byte{
		[]byte(`{"jsonrpc":"2.0","id":1,"result":null}`),
		bytes.Repeat([]byte("x"), 9000), // body spans several 4096-byte reads
		[]byte(`{"jsonrpc":"2.0","method":"note"}`),
	}
	var stream []byte
	for _, b := range bodies {
		stream = append(stream, EncodeMessage(b)...)
	}
	for _, chunk := range []int{1, 2, 3, 4, 5, 7, 100, 4096, len(stream)} {
		fr := NewFrameReader(&chunkReader{data: stream, chunk: chunk})
		for i, want := range bodies {
			got, err := fr.ReadMessage()
			if err != nil {
				t.Fatalf("chunk=%d message %d: %v", chunk, i, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("chunk=%d message %d: got %d bytes, want %d", chunk, i, len(got), len(want))
			}
		}
		if _, err := fr.ReadMessage(); err != io.EOF {
			t.Fatalf("chunk=%d: after last message err = %v, want EOF", chunk, err)
		}
	}
}

// After the overflow guard discards a header-less buffer, the scan position
// must reset so a following valid message is still parsed. The junk ends on a
// read boundary: the guard discards everything it holds, so a message that
// shares the overflowing read is dropped with it (long-standing behavior).
func TestFrameReader_RecoversAfterOverflowDiscard(t *testing.T) {
	want := []byte(`{"jsonrpc":"2.0","id":7,"result":"ok"}`)
	stream := append(bytes.Repeat([]byte("z"), maxBufferSize+4096), EncodeMessage(want)...)
	fr := NewFrameReader(&chunkReader{data: stream, chunk: 4096})
	got, err := fr.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A large header-less stream in small reads must be linear: it took minutes
// when every read rescanned the whole buffer (issue #65).
func TestFrameReader_LargeHeaderlessStreamIsLinear(t *testing.T) {
	fr := NewFrameReader(&chunkReader{data: make([]byte, maxBufferSize+1000), chunk: 100})
	start := time.Now()
	_, _ = fr.ReadMessage() // ends with EOF after the overflow discard
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("reading %d header-less bytes took %v; the header search must not rescan the buffer", maxBufferSize, d)
	}
}
