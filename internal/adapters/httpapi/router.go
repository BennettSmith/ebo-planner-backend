package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"ebo-planner-backend/internal/adapters/httpapi/oas"
)

type RouterOptions struct {
	AuthMiddleware func(http.Handler) http.Handler
}

// NewRouter constructs the API HTTP router.
//
// This is intentionally a thin adapter:
// - the generated OpenAPI layer handles request decoding + validation
// - this package wires routes/middleware and delegates to a ServerInterface implementation
func NewRouter(ssi oas.StrictServerInterface) http.Handler {
	return NewRouterWithOptions(ssi, RouterOptions{})
}

func NewRouterWithOptions(ssi oas.StrictServerInterface, opts RouterOptions) http.Handler {
	r := chi.NewRouter()

	// Baseline production-safe middleware (minimal but useful).
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	if opts.AuthMiddleware != nil {
		r.Use(opts.AuthMiddleware)
	}

	// Health endpoint is deliberately out-of-spec (used for infra checks).
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Strict handler wiring:
	// - app/adapter implements `oas.StrictServerInterface`
	// - generated strict handler adapts it to the legacy `oas.ServerInterface`
	sh := oas.NewStrictHandlerWithOptions(ssi, nil, oas.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			// JSON decode / parameter coercion errors (client input).
			writeOASError(w, req, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error(), nil)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			// Unexpected server-side errors.
			writeOASError(w, req, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", map[string]any{
				"cause": err.Error(),
			})
		},
	})
	_ = oas.HandlerFromMux(sh, r)
	return r
}
