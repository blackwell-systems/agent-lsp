// CodeRabbit #60 re-review thread 2: an explicitly supplied empty
// occurrence_ids list must survive the toolArgsToMap round-trip (the
// *[]string field drops only nil), so parseReplaceParams can refuse it on
// apply instead of silently applying every occurrence.
package main

import (
	"testing"
)

func TestReplaceInFilesArgsEmptyOccurrenceIdsReachHandler(t *testing.T) {
	empty := []string{}
	m := toolArgsToMap(ReplaceInFilesArgs{Needle: "n", Repl: "r", OccurrenceIds: &empty})
	raw, ok := m["occurrence_ids"].([]any)
	if !ok {
		t.Fatalf("explicit empty occurrence_ids must reach the handler as a JSON array, got %T %v", m["occurrence_ids"], m["occurrence_ids"])
	}
	if len(raw) != 0 {
		t.Fatalf("expected an empty array, got %v", raw)
	}
	// A populated list survives too.
	ids := []string{"a:1@deadbeefdeadbeef"}
	m = toolArgsToMap(ReplaceInFilesArgs{Needle: "n", Repl: "r", OccurrenceIds: &ids})
	if raw, ok = m["occurrence_ids"].([]any); !ok || len(raw) != 1 {
		t.Fatalf("populated occurrence_ids must survive the round-trip, got %v", m["occurrence_ids"])
	}
	// A nil pointer stays omitted: schema-optional and treated as apply-all.
	m = toolArgsToMap(ReplaceInFilesArgs{Needle: "n", Repl: "r"})
	if _, present := m["occurrence_ids"]; present {
		t.Fatalf("nil occurrence_ids must stay omitted, got %v", m["occurrence_ids"])
	}
}
