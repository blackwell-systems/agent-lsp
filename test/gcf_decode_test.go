package main_test

import (
	"strings"
	"testing"

	gcfgo "github.com/blackwell-systems/gcf-go"
)

// decodeGraph decodes a GCF graph-profile response (as produced by
// gcf.EncodeGraph on the server) into a Payload. Returns an error if the
// text is not valid GCF graph format. It is the true inverse of the server's
// EncodeResult path for *gcfgo.Payload data.
func decodeGraph(text string) (*gcfgo.Payload, error) {
	return gcfgo.Decode(text)
}

// decodeGeneric decodes a GCF generic/tabular-profile response into a Go value
// (typically a []any of maps or a map[string]any). Returns an error if the
// text is not valid GCF generic format. It is the inverse of the server's
// gcf.Encode (EncodeGenericChecked) path.
func decodeGeneric(text string) (any, error) {
	return gcfgo.DecodeGeneric(text)
}

// graphSymbolNames returns the trailing name segment of every symbol's
// QualifiedName in the payload (e.g. "pkg/foo.Person" -> "Person"). A nil
// payload yields an empty slice.
func graphSymbolNames(p *gcfgo.Payload) []string {
	if p == nil {
		return nil
	}
	names := make([]string, 0, len(p.Symbols))
	for _, s := range p.Symbols {
		qn := s.QualifiedName
		if i := strings.LastIndex(qn, "."); i >= 0 {
			qn = qn[i+1:]
		}
		names = append(names, qn)
	}
	return names
}

// graphEdgeCount returns len(p.Edges) for a decoded graph Payload, or 0 for a
// nil payload.
func graphEdgeCount(p *gcfgo.Payload) int {
	if p == nil {
		return 0
	}
	return len(p.Edges)
}

// TestGcfDecodeHelpers round-trips a tiny payload through the real gcf-go
// encoder and the decode helpers, proving the helpers work without needing a
// language server installed. The edge's endpoints are both present as symbols
// so it survives encode/decode.
func TestGcfDecodeHelpers(t *testing.T) {
	in := &gcfgo.Payload{
		Tool: "test",
		Symbols: []gcfgo.Symbol{
			{QualifiedName: "pkg/foo.Person", Kind: "type", Score: 0.9, Provenance: "lsp_resolved"},
			{QualifiedName: "pkg/foo.Greet", Kind: "function", Score: 0.8, Provenance: "lsp_resolved"},
		},
		Edges: []gcfgo.Edge{
			{Source: "pkg/foo.Greet", Target: "pkg/foo.Person", EdgeType: "references"},
		},
	}

	// gcfgo.Encode returns a single string value (no error).
	encoded := gcfgo.Encode(in)

	got, err := decodeGraph(encoded)
	if err != nil {
		t.Fatalf("decodeGraph returned error: %v", err)
	}

	names := graphSymbolNames(got)
	if !containsString(names, "Person") {
		t.Errorf("graphSymbolNames = %v, want to contain %q", names, "Person")
	}

	if n := graphEdgeCount(got); n != 1 {
		t.Errorf("graphEdgeCount = %d, want 1", n)
	}

	// nil-payload behavior.
	if names := graphSymbolNames(nil); len(names) != 0 {
		t.Errorf("graphSymbolNames(nil) = %v, want empty", names)
	}
	if n := graphEdgeCount(nil); n != 0 {
		t.Errorf("graphEdgeCount(nil) = %d, want 0", n)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
