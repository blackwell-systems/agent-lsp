package gcf

import gcfgo "github.com/blackwell-systems/gcf-go"

// Encode converts any structured data to GCF tabular format string.
// Uses gcf-go's EncodeGenericChecked, which handles arbitrary Go values via
// reflection and produces compact text output. The Checked variant returns an
// error instead of panicking when a value falls outside the canonical numeric
// domain (an integer beyond int64, per gcf spec v3.5.3); callers such as
// EncodeResult treat that error as a GCF failure and fall back to JSON. agent-lsp
// data (line/column numbers, counts, ranges) is well within int64, so this path
// is defensive rather than expected.
func Encode(data any) (string, error) {
	if data == nil {
		return "", nil
	}
	return gcfgo.EncodeGenericChecked(data)
}
