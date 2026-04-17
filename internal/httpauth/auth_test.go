package httpauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubNext is a simple handler that records calls and writes 200 OK.
func stubNext() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestBearerTokenMiddleware_EmptyToken verifies that when the token is empty,
// all requests pass through without any Authorization check.
func TestBearerTokenMiddleware_EmptyToken(t *testing.T) {
	t.Run("GET with no Authorization header passes through", func(t *testing.T) {
		h := BearerTokenMiddleware("", stubNext())
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rr.Code)
		}
	})

	t.Run("POST with no Authorization header passes through", func(t *testing.T) {
		h := BearerTokenMiddleware("", stubNext())
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rr.Code)
		}
	})

	t.Run("request with arbitrary Authorization header passes through when token is empty", func(t *testing.T) {
		h := BearerTokenMiddleware("", stubNext())
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer anything")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rr.Code)
		}
	})
}

// TestBearerTokenMiddleware_CorrectToken verifies that a correctly formed
// Authorization: Bearer <token> header is accepted.
func TestBearerTokenMiddleware_CorrectToken(t *testing.T) {
	const token = "supersecret"
	h := BearerTokenMiddleware(token, stubNext())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
}

// TestBearerTokenMiddleware_WrongToken verifies that a wrong token receives
// HTTP 401 with {"error":"unauthorized"}.
func TestBearerTokenMiddleware_WrongToken(t *testing.T) {
	const token = "supersecret"
	h := BearerTokenMiddleware(token, stubNext())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	assertUnauthorizedBody(t, rr)
}

// TestBearerTokenMiddleware_MissingHeader verifies that a request with no
// Authorization header receives HTTP 401.
func TestBearerTokenMiddleware_MissingHeader(t *testing.T) {
	const token = "supersecret"
	h := BearerTokenMiddleware(token, stubNext())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	assertUnauthorizedBody(t, rr)
}

// TestBearerTokenMiddleware_CorrectPrefixLongerValue verifies that a token
// that is a prefix of the expected value is rejected (timing-safe).
func TestBearerTokenMiddleware_CorrectPrefixLongerValue(t *testing.T) {
	const token = "secret"
	h := BearerTokenMiddleware(token, stubNext())

	// "Bearer secret" is valid; "Bearer secretXXX" must be rejected.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secretXXX")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for extended token, got %d", rr.Code)
	}
	assertUnauthorizedBody(t, rr)
}

// TestBearerTokenMiddleware_CaseSensitive verifies that "bearer token"
// (lowercase scheme) is rejected when the expected scheme is "Bearer".
func TestBearerTokenMiddleware_CaseSensitive(t *testing.T) {
	const token = "mytoken"
	h := BearerTokenMiddleware(token, stubNext())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer "+token) // lowercase b
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for lowercase bearer, got %d", rr.Code)
	}
	assertUnauthorizedBody(t, rr)
}

// assertUnauthorizedBody checks that the response body is the expected JSON
// error object and that Content-Type is application/json.
func assertUnauthorizedBody(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("want Content-Type application/json, got %q", ct)
	}

	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if wwwAuth != "Bearer" {
		t.Errorf("want WWW-Authenticate: Bearer, got %q", wwwAuth)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("want error=unauthorized, got %q", body["error"])
	}
}
