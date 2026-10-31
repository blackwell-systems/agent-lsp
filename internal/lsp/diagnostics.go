package lsp

import (
	"context"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// DiagnosticsMark records where a document's diagnostics stood when an edit
// was sent, so a later wait can tell publications that cover the edit from
// ones that were already there. Take it with MarkDiagnostics before sending
// the change, then set Version to the document version the change produced.
type DiagnosticsMark struct {
	// Gen is the publish count for the URI when the mark was taken.
	Gen uint64
	// Version is the document version the wait must see diagnostics for, when
	// the server reports versions. Zero means no version requirement.
	Version int
}

// MarkDiagnostics returns the current publish count for uri.
func (c *LSPClient) MarkDiagnostics(uri string) DiagnosticsMark {
	gen, _, _ := c.diagnosticsState(uri)
	return DiagnosticsMark{Gen: gen}
}

// coverage classifies a URI's diagnostics against a mark.
type coverage int

const (
	covered coverage = iota // a publication covers the marked state
	silent                  // nothing published since the mark
	stale                   // published since the mark, but for an older version
)

// diagnosticsCoverage compares what the server has published for uri with
// mark. When the last publication carried a document version it must reach
// mark.Version; otherwise any publication after the mark covers it.
func diagnosticsCoverage(client *LSPClient, uri string, mark DiagnosticsMark) coverage {
	gen, version, hasVersion := client.diagnosticsState(uri)
	if mark.Version > 0 && hasVersion && version >= mark.Version {
		return covered
	}
	if gen == mark.Gen {
		return silent
	}
	if mark.Version > 0 && hasVersion {
		return stale
	}
	return covered
}

// WaitForDiagnostics waits for fresh diagnostics for every uri followed by a
// 500ms quiet window. It returns nil on timeout; callers that must know whether
// the diagnostics settled use WaitForDiagnosticsSettled.
func WaitForDiagnostics(ctx context.Context, client *LSPClient, uris []string, timeoutMs int) error {
	_, err := WaitForDiagnosticsSettled(ctx, client, uris, nil, timeoutMs)
	return err
}

// WaitForDiagnosticsSettled waits until the diagnostics for every uri cover
// the state in marks and no publication has arrived for 500ms. A uri without a
// mark needs a publication that arrives during the wait.
//
// settled is false when the deadline passed first; err is non-nil only when
// ctx ended. A timeout must not be read as "no new errors": the cache may still
// hold the diagnostics from before the edit.
//
// Many servers (typescript-language-server, rust-analyzer, Dart) publish
// nothing when an edit leaves the diagnostics unchanged, so for them silence
// until the deadline is the expected result of a clean edit and counts as
// settled. For a server known to publish for every version (gopls), silence
// means it has not analysed the edit, and the wait is not settled. A publication for
// an older document version is never settled.
func WaitForDiagnosticsSettled(ctx context.Context, client *LSPClient, uris []string, marks map[string]DiagnosticsMark, timeoutMs int) (settled bool, err error) {
	if len(uris) == 0 {
		return true, nil
	}

	notify := make(chan struct{}, 1)
	cb := types.DiagnosticUpdateCallback(func(string, []types.LSPDiagnostic) {
		select {
		case notify <- struct{}{}:
		default:
		}
	})

	// Marks for unmarked URIs are taken before subscribing so a publication
	// racing the subscription is still counted as fresh.
	targets := make(map[string]DiagnosticsMark, len(uris))
	for _, uri := range uris {
		if m, ok := marks[uri]; ok {
			targets[uri] = m
		} else {
			targets[uri] = client.MarkDiagnostics(uri)
		}
	}

	client.SubscribeToDiagnostics(cb)
	defer client.UnsubscribeFromDiagnostics(cb)

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	const quietWindow = 500 * time.Millisecond

	// lastPublish starts at the wait's start: even diagnostics that were
	// already complete get one quiet window, as before.
	lastPublish := time.Now()
	seenGen := make(map[string]uint64, len(uris))
	for uri := range targets {
		seenGen[uri], _, _ = client.diagnosticsState(uri)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
		case <-notify:
		}

		allCovered, anyStale := true, false
		for uri, mark := range targets {
			if gen, _, _ := client.diagnosticsState(uri); gen != seenGen[uri] {
				seenGen[uri] = gen
				lastPublish = time.Now()
			}
			switch diagnosticsCoverage(client, uri, mark) {
			case covered:
			case stale:
				allCovered, anyStale = false, true
			default:
				allCovered = false
			}
		}
		quiet := time.Since(lastPublish) >= quietWindow
		if allCovered && quiet {
			return true, nil
		}
		if time.Now().After(deadline) {
			// Settled only if nothing is still arriving, no publication was
			// stale, and the silent URIs belong to a server that stays silent
			// on unchanged diagnostics.
			return quiet && !anyStale && !client.RepublishesUnchangedDiagnostics(), nil
		}
	}
}
