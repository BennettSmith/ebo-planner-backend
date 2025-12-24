package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"eastbay-overland-rally-planner/internal/adapters/httpapi/oas"
)

// NewRouter constructs the API HTTP router.
//
// This is intentionally a thin adapter:
// - the generated OpenAPI layer handles request decoding + validation
// - this package wires routes/middleware and delegates to a ServerInterface implementation
func NewRouter(ssi oas.StrictServerInterface) http.Handler {
	r := chi.NewRouter()

	// Baseline production-safe middleware (minimal but useful).
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health endpoint is deliberately out-of-spec (used for infra checks).
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Strict handler wiring:
	// - app/adapter implements `oas.StrictServerInterface`
	// - generated strict handler adapts it to the legacy `oas.ServerInterface`
	_ = oas.HandlerFromMux(oas.NewStrictHandler(ssi, nil), r)
	return r
}


