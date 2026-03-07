package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"forge/pkg/types"
)

// NewMiddleware returns an http middleware that validates Bearer tokens.
// If apiKey is empty, the middleware is a no-op (passthrough).
// If apiKey is set, requests without a valid "Authorization: Bearer <key>"
// header are rejected with 401.
//
// Usage: r.Use(auth.NewMiddleware(cfg.APIKey))
func NewMiddleware(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// No API key configured → passthrough (no auth required).
		if apiKey == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				types.WriteError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, types.ErrorTypeAuthentication, "Invalid or missing API key")
				return
			}

			if !strings.HasPrefix(header, "Bearer ") {
				types.WriteError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, types.ErrorTypeAuthentication, "Invalid or missing API key")
				return
			}

			token := strings.TrimPrefix(header, "Bearer ")
			if token == "" {
				types.WriteError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, types.ErrorTypeAuthentication, "Invalid or missing API key")
				return
			}

			if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
				types.WriteError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, types.ErrorTypeAuthentication, "Invalid or missing API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
