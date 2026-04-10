// Package uri provides helpers for converting between LSP file:// URIs and
// local filesystem paths, and for applying in-memory range edits to file
// content strings. URIToPath and PathToURI handle the encoding/decoding
// roundtrip. ApplyRangeEdit is the canonical implementation used by both
// LSPClient (workspace/applyEdit) and SessionManager (simulate_edit).
package uri
