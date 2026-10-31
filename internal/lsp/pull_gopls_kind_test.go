package lsp

import (
	"context"
	"errors"
	"testing"
	"time"
)

// gopls (v0.23) answers a pull with kind "" on full reports. That is accepted
// for gopls only; any other server's empty kind still verifies nothing.
func TestPullDiagnostics_EmptyKind(t *testing.T) {
	for _, tc := range []struct {
		serverPath string
		wantOK     bool
	}{{"/usr/local/bin/gopls", true}, {"some-server", false}} {
		t.Run(tc.serverPath, func(t *testing.T) {
			c, serverW, clientR := newTestClient(t)
			c.serverPath = tc.serverPath
			c.capsMu.Lock()
			c.capabilities["diagnosticProvider"] = map[string]any{"interFileDependencies": true}
			c.capsMu.Unlock()

			done := make(chan error, 1)
			var got int
			go func() {
				d, err := c.PullDiagnostics(context.Background(), "file:///x.go")
				got = len(d)
				done <- err
			}()
			req := readNextMsg(t, clientR)
			if req == nil {
				t.Fatal("expected diagnostic request")
			}
			_ = writeMsg(serverW, map[string]any{"jsonrpc": "2.0", "id": req["id"], "result": map[string]any{
				"kind": "",
				"items": []any{map[string]any{
					"range":    map[string]any{"start": map[string]any{"line": 2, "character": 32}, "end": map[string]any{"line": 2, "character": 39}},
					"severity": 1, "message": "invalid operation: mismatched types",
				}},
			}})
			select {
			case err := <-done:
				if tc.wantOK && (err != nil || got != 1) {
					t.Fatalf("err=%v n=%d, want the one diagnostic", err, got)
				}
				if !tc.wantOK && !errors.Is(err, ErrPullDiagnosticsNoReport) {
					t.Fatalf("err=%v, want ErrPullDiagnosticsNoReport", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timeout")
			}
		})
	}
}

func TestPrefersPullDiagnostics_GoplsOnly(t *testing.T) {
	for path, want := range map[string]bool{"gopls": true, "pyright-langserver": false} {
		c := NewLSPClient(path, nil)
		c.capabilities["diagnosticProvider"] = true
		if got := c.PrefersPullDiagnostics(); got != want {
			t.Errorf("%s: PrefersPullDiagnostics = %v, want %v", path, got, want)
		}
	}
	if NewLSPClient("gopls", nil).PrefersPullDiagnostics() {
		t.Error("gopls without diagnosticProvider must not prefer pull")
	}
}
