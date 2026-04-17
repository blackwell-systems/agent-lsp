package httpauth

import "net/http"

// BearerTokenMiddleware wraps an http.Handler with Bearer token validation.
// If token is empty, all requests pass through without auth.
//
// This is a scaffold stub. Agent B (Wave 1) will replace this body with the
// full implementation using crypto/subtle for timing-safe comparison and
// returning HTTP 401 with {"error":"unauthorized"} on mismatch.
func BearerTokenMiddleware(token string, next http.Handler) http.Handler {
	panic("httpauth: BearerTokenMiddleware is a scaffold stub — not yet implemented")
}
