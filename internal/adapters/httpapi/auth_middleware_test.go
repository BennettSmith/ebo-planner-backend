package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/httpapi/oas"
	"github.com/Overland-East-Bay/trip-planner-api/internal/platform/auth/jwks_testutil"
	"github.com/Overland-East-Bay/trip-planner-api/internal/platform/auth/jwtverifier"
	"github.com/Overland-East-Bay/trip-planner-api/internal/platform/config"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type authProbeServer struct {
	StrictUnimplemented
}

func (s authProbeServer) ListMembers(ctx context.Context, _ oas.ListMembersRequestObject) (oas.ListMembersResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok || sub == "" {
		er := notImplementedError()
		er.Error.Code = "MISSING_SUBJECT"
		er.Error.Message = "subject missing from context"
		return oas.ListMembers500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(er)}, nil
	}
	return oas.ListMembers200JSONResponse{Members: []oas.MemberDirectoryEntry{}}, nil
}

func newTestAuthRouter(t *testing.T) (http.Handler, func(now time.Time, kid string) string) {
	t.Helper()

	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	t.Cleanup(jwksSrv.Close)

	kp, err := jwks_testutil.GenerateRSAKeypair("kid-1")
	if err != nil {
		t.Fatalf("GenerateRSAKeypair: %v", err)
	}
	setKeys([]jwks_testutil.Keypair{kp})

	cfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    10 * time.Minute,
		JWKSMinRefreshInterval: 0,
		HTTPTimeout:            2 * time.Second,
	}

	clk := fixedClock{t: time.Unix(1700000000, 0)}
	v := jwtverifier.NewWithOptions(cfg, nil, clk)

	mint := func(now time.Time, kid string) string {
		if kid != kp.Kid {
			t.Fatalf("unsupported kid in test: %s", kid)
		}
		jwt, err := jwks_testutil.MintRS256JWT(kp, cfg.Issuer, cfg.Audience, "member-123", now, 5*time.Minute, nil)
		if err != nil {
			t.Fatalf("MintRS256JWT: %v", err)
		}
		return jwt
	}

	h := NewRouterWithOptions(authProbeServer{}, RouterOptions{
		AuthMiddleware: NewAuthMiddleware(v),
	})

	return h, mint
}

func TestAuthMiddleware_MissingHeader_401(t *testing.T) {
	t.Parallel()

	h, _ := newTestAuthRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
	}
	var er oas.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if er.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("code: got %q", er.Error.Code)
	}
	if !er.Error.RequestId.IsSpecified() || er.Error.RequestId.IsNull() {
		t.Fatalf("expected requestId to be set")
	}
	if rid, err := er.Error.RequestId.Get(); err != nil || rid == "" {
		t.Fatalf("expected requestId to be a non-empty string")
	}
}

func TestAuthMiddleware_MalformedHeader_401(t *testing.T) {
	t.Parallel()

	h, _ := newTestAuthRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	req.Header.Set("Authorization", "Basic abc")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_ValidToken_AllowsRequestAndSetsSubject(t *testing.T) {
	t.Parallel()

	h, mint := newTestAuthRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	req.Header.Set("Authorization", "Bearer "+mint(time.Unix(1700000000, 0), "kid-1"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestStrictRequestError_Is422JSON(t *testing.T) {
	t.Parallel()

	h, mint := newTestAuthRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/members", bytes.NewBufferString("{"))
	req.Header.Set("Authorization", "Bearer "+mint(time.Unix(1700000000, 0), "kid-1"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	var er oas.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if er.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("code: got %q", er.Error.Code)
	}
}
