package lsp

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// serveFakeTCPLSP accepts one connection and answers every request with an
// empty result ({"capabilities":{}} for initialize), ignoring notifications.
// It reports the methods it received on the returned channel.
func serveFakeTCPLSP(t *testing.T) (addr string, methods <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan string, 64)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		fr := NewFrameReader(conn)
		for {
			raw, err := fr.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(raw, &msg) != nil || msg.Method == "" {
				continue
			}
			select {
			case got <- msg.Method:
			default:
			}
			if len(msg.ID) == 0 {
				continue // notification
			}
			var result any
			if msg.Method == "initialize" {
				result = map[string]any{"capabilities": map[string]any{}}
			}
			_ = writeMsg(conn, map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
		}
	}()
	return ln.Addr().String(), got
}

// TestPassiveClient_Initialize covers issue #74: Initialize spawned a process
// for passive clients, so start_lsp connect=<addr> always failed with
// "exec: no command". It must run the handshake over the existing socket.
func TestPassiveClient_Initialize(t *testing.T) {
	addr, methods := serveFakeTCPLSP(t)
	client, err := NewPassiveClient(addr)
	if err != nil {
		t.Fatalf("NewPassiveClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer client.Shutdown(ctx)

	if err := client.Initialize(ctx, t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !client.IsInitialized() {
		t.Fatal("client not initialized after Initialize")
	}
	seen := map[string]bool{}
	deadline := time.After(2 * time.Second)
	for !seen["initialized"] {
		select {
		case m := <-methods:
			seen[m] = true
		case <-deadline:
			t.Fatalf("server saw %v, want initialize and initialized", seen)
		}
	}
	if !seen["initialize"] {
		t.Fatalf("server saw %v, want initialize", seen)
	}
}
