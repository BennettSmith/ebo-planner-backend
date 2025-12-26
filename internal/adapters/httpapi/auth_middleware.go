package httpapi

import (
	"net/http"
	"strings"

	"ebo-planner-backend/internal/platform/auth/jwtverifier"
)

// NewAuthMiddleware enforces Authorization: Bearer <JWT> for all in-spec endpoints.
//
// On success, it stores the authenticated subjectID (JWT `sub`) in request context.
func NewAuthMiddleware(v *jwtverifier.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health endpoint is deliberately out-of-spec and unauthenticated.
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			authz := r.Header.Get("Authorization")
			if authz == "" {
				writeOASError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header", nil)
				return
			}
			const prefix = "Bearer "
			if !strings.HasPrefix(authz, prefix) {
				writeOASError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "malformed Authorization header", nil)
				return
			}
			raw := strings.TrimSpace(strings.TrimPrefix(authz, prefix))
			if raw == "" {
				writeOASError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
				return
			}

			sub, err := v.Verify(r.Context(), raw)
			if err != nil {
				writeOASError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithSubject(r.Context(), sub)))
		})
	}
}

// NewDevAuthMiddleware is a local/dev-only auth shim.
//
// It accepts an explicit subject via X-Debug-Subject and stores it in request context.
// If the header is absent, it falls back to defaultSubject (if provided).
//
// This is intended for local Docker workflows where standing up an OIDC provider + JWKS
// is overkill. Do NOT use this in production deployments.
func NewDevAuthMiddleware(defaultSubject string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health endpoint is deliberately out-of-spec and unauthenticated.
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			sub := strings.TrimSpace(r.Header.Get("X-Debug-Subject"))
			if sub == "" {
				sub = strings.TrimSpace(defaultSubject)
			}
			if sub == "" {
				writeOASError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing subject (set X-Debug-Subject)", nil)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithSubject(r.Context(), sub)))
		})
	}
}
