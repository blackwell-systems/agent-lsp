// Package httpauth provides HTTP authentication middleware for agent-lsp.
package httpauth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// BearerTokenMiddleware returns an http.Handler that enforces Bearer token
// authentication when token is non-empty.
//
// When token is empty, all requests pass through without authentication,
// preserving the current stdio-only behavior for local usage.
//
// When token is non-empty, requests must supply:
//
//	Authorization: Bearer <token>
//
// On mismatch: HTTP 401, Content-Type: application/json,
// body: {"error":"unauthorized"}
//
// The comparison is done with subtle.ConstantTimeCompare to avoid
// timing side-channels.
func BearerTokenMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	expected := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(expected, got) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
