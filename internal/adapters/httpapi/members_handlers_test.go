package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ebo-planner-backend/internal/adapters/httpapi/oas"
	memclock "ebo-planner-backend/internal/adapters/memory/clock"
	memidempotency "ebo-planner-backend/internal/adapters/memory/idempotency"
	memmemberrepo "ebo-planner-backend/internal/adapters/memory/memberrepo"
	memrsvprepo "ebo-planner-backend/internal/adapters/memory/rsvprepo"
	memtriprepo "ebo-planner-backend/internal/adapters/memory/triprepo"
	"ebo-planner-backend/internal/app/members"
	"ebo-planner-backend/internal/app/trips"
	"ebo-planner-backend/internal/platform/auth/jwks_testutil"
	"ebo-planner-backend/internal/platform/auth/jwtverifier"
	"ebo-planner-backend/internal/platform/config"
)

type fixedClockMembers struct{ t time.Time }

func (c fixedClockMembers) Now() time.Time { return c.t }

func newTestMemberRouter(t *testing.T) (http.Handler, func(now time.Time, kid string) string) {
	t.Helper()

	kp, err := jwks_testutil.GenerateRSAKeypair("kid-1")
	if err != nil {
		t.Fatalf("GenerateRSAKeypair: %v", err)
	}
	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	t.Cleanup(jwksSrv.Close)
	setKeys([]jwks_testutil.Keypair{kp})

	jwtCfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    10 * time.Minute,
		JWKSMinRefreshInterval: time.Second,
		HTTPTimeout:            2 * time.Second,
	}
	v := jwtverifier.NewWithOptions(jwtCfg, nil, fixedClockMembers{t: time.Unix(1700000000, 0)})

	clk := memclock.NewManualClock(time.Unix(100, 0).UTC())
	repo := memmemberrepo.NewRepo()
	idem := memidempotency.NewStore()
	memberSvc := members.NewService(repo, clk)

	tripRepo := memtriprepo.NewRepo()
	rsvpRepo := memrsvprepo.NewRepo()
	tripSvc := trips.NewService(tripRepo, repo, rsvpRepo)
	api := NewServer(memberSvc, tripSvc, idem)
	h := NewRouterWithOptions(api, RouterOptions{AuthMiddleware: NewAuthMiddleware(v)})

	mint := func(now time.Time, kid string) string {
		jwt, err := jwks_testutil.MintRS256JWT(
			jwks_testutil.Keypair{Kid: kid, Private: kp.Private},
			jwtCfg.Issuer,
			jwtCfg.Audience,
			"sub-1",
			now,
			10*time.Minute,
			nil,
		)
		if err != nil {
			t.Fatalf("MintRS256JWT: %v", err)
		}
		return jwt
	}

	return h, mint
}

func TestMembers_GetMe_NotProvisioned_404(t *testing.T) {
	t.Parallel()

	h, mint := newTestMemberRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/members/me", nil)
	req.Header.Set("Authorization", "Bearer "+mint(time.Unix(1700000000, 0), "kid-1"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var er oas.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if er.Error.Code != "MEMBER_NOT_PROVISIONED" {
		t.Fatalf("code=%q", er.Error.Code)
	}
}

func TestMembers_CreateThenGetMe_200(t *testing.T) {
	t.Parallel()

	h, mint := newTestMemberRouter(t)

	createBody := `{"displayName":"Alice Smith","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/members", bytes.NewBufferString(createBody))
	req.Header.Set("Authorization", "Bearer "+mint(time.Unix(1700000000, 0), "kid-1"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/members/me", nil)
	req2.Header.Set("Authorization", "Bearer "+mint(time.Unix(1700000000, 0), "kid-1"))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestMembers_UpdateMe_IdempotentReplayAndConflictOnReuse(t *testing.T) {
	t.Parallel()

	h, mint := newTestMemberRouter(t)
	authz := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1")

	// Provision first.
	reqCreate := httptest.NewRequest(http.MethodPost, "/members", bytes.NewBufferString(`{"displayName":"Alice","email":"alice@example.com"}`))
	reqCreate.Header.Set("Authorization", authz)
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()
	h.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", recCreate.Code, recCreate.Body.String())
	}

	idemKey := "idem-12345678"
	body1 := `{"displayName":"  Alice   Smith "}`
	req1 := httptest.NewRequest(http.MethodPatch, "/members/me", bytes.NewBufferString(body1))
	req1.Header.Set("Authorization", authz)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idemKey)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("patch1 status=%d body=%s", rec1.Code, rec1.Body.String())
	}

	// Same key + same semantic payload (after normalization) should replay.
	body2 := `{"displayName":"Alice   Smith"}`
	req2 := httptest.NewRequest(http.MethodPatch, "/members/me", bytes.NewBufferString(body2))
	req2.Header.Set("Authorization", authz)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idemKey)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("patch2 status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	// Same key + different payload should be 409.
	body3 := `{"displayName":"Bob"}` // different
	req3 := httptest.NewRequest(http.MethodPatch, "/members/me", bytes.NewBufferString(body3))
	req3.Header.Set("Authorization", authz)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", idemKey)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusConflict {
		t.Fatalf("patch3 status=%d body=%s", rec3.Code, rec3.Body.String())
	}
}
