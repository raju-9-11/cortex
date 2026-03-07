package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge/pkg/types"
)

// dummyHandler returns a 200 with body "ok" — proves the request got through.
var dummyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

func TestPassthroughWhenNoAPIKey(t *testing.T) {
	mw := NewMiddleware("")
	handler := mw(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body 'ok', got %q", rec.Body.String())
	}
}

func TestRejectsMissingAuthorizationHeader(t *testing.T) {
	mw := NewMiddleware("secret-key")
	handler := mw(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertUnauthorized(t, rec)
}

func TestRejectsNonBearerScheme(t *testing.T) {
	mw := NewMiddleware("secret-key")
	handler := mw(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertUnauthorized(t, rec)
}

func TestRejectsEmptyBearerToken(t *testing.T) {
	mw := NewMiddleware("secret-key")
	handler := mw(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertUnauthorized(t, rec)
}

func TestRejectsWrongAPIKey(t *testing.T) {
	mw := NewMiddleware("secret-key")
	handler := mw(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertUnauthorized(t, rec)
}

func TestAcceptsCorrectAPIKey(t *testing.T) {
	mw := NewMiddleware("secret-key")
	handler := mw(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body 'ok', got %q", rec.Body.String())
	}
}

// assertUnauthorized checks the response is a 401 with the correct structured error body.
func assertUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var resp types.APIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.Error.Code != types.ErrCodeUnauthorized {
		t.Fatalf("expected error code %q, got %q", types.ErrCodeUnauthorized, resp.Error.Code)
	}
	if resp.Error.Type != types.ErrorTypeAuthentication {
		t.Fatalf("expected error type %q, got %q", types.ErrorTypeAuthentication, resp.Error.Type)
	}
	if resp.Error.Message != "Invalid or missing API key" {
		t.Fatalf("expected message 'Invalid or missing API key', got %q", resp.Error.Message)
	}
}
