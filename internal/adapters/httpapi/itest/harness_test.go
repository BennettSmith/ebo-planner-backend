package itest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/httpapi"
	memclock "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/clock"
	memidempotency "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/idempotency"
	memmemberrepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/memberrepo"
	memrsvprepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/rsvprepo"
	memtriprepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/triprepo"
	pgidempotency "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/idempotency"
	pgmemberrepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/memberrepo"
	pgrsvprepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/rsvprepo"
	postgres_testutil "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/testutil"
	pgtriprepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/triprepo"
	"github.com/Overland-East-Bay/trip-planner-api/internal/app/members"
	"github.com/Overland-East-Bay/trip-planner-api/internal/app/trips"
	idempotencyport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/idempotency"
	memberrepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
	rsvprepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/rsvprepo"
	triprepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/triprepo"
)

type backend string

const (
	backendMemory   backend = "memory"
	backendPostgres backend = "postgres"
)

func backendsFromEnv(t *testing.T) []backend {
	t.Helper()
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ITEST_BACKEND"))) {
	case "", "memory":
		return []backend{backendMemory}
	case "postgres":
		return []backend{backendPostgres}
	case "all":
		return []backend{backendMemory, backendPostgres}
	default:
		t.Fatalf("unknown ITEST_BACKEND value (expected memory|postgres|all)")
		return nil
	}
}

type testServer struct {
	baseURL string
	client  *http.Client
}

func newTestServer(t *testing.T, b backend) *testServer {
	t.Helper()

	const issuer = "itest-issuer"
	clk := memclock.NewManualClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

	var (
		memberRepo memberrepoport.Repository
		tripRepo   triprepoport.Repository
		rsvpRepo   rsvprepoport.Repository
		idemStore  idempotencyport.Store
	)

	switch b {
	case backendPostgres:
		pool := postgres_testutil.OpenMigratedPool(t)
		memberRepo = pgmemberrepo.NewRepo(pool, issuer)
		tripRepo = pgtriprepo.NewRepo(pool)
		rsvpRepo = pgrsvprepo.NewRepo(pool)
		idemStore = pgidempotency.NewStore(pool, issuer)
	case backendMemory:
		memberRepo = memmemberrepo.NewRepo()
		tripRepo = memtriprepo.NewRepo()
		rsvpRepo = memrsvprepo.NewRepo()
		idemStore = memidempotency.NewStore()
	default:
		t.Fatalf("unknown backend: %s", b)
	}

	memberSvc := members.NewService(memberRepo, clk)
	tripSvc := trips.NewService(tripRepo, memberRepo, rsvpRepo)
	api := httpapi.NewServer(memberSvc, tripSvc, idemStore)

	// Integration tests use the dev auth middleware to stay fully local and deterministic.
	// We pass empty default subject to ensure requests MUST provide X-Debug-Subject, allowing
	// auth-failure coverage.
	authMW := httpapi.NewDevAuthMiddleware("")
	handler := httpapi.NewRouterWithOptions(api, httpapi.RouterOptions{AuthMiddleware: authMW})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &testServer{
		baseURL: srv.URL,
		client:  srv.Client(),
	}
}

func (s *testServer) url(path string) string {
	if strings.HasPrefix(path, "/") {
		return s.baseURL + path
	}
	return s.baseURL + "/" + path
}

func (s *testServer) doJSON(t *testing.T, method string, path string, subject string, body any) (int, []byte, http.Header) {
	t.Helper()

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.url(path), r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if subject != "" {
		req.Header.Set("X-Debug-Subject", subject)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out, resp.Header
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func mustUnmarshal[T any](t *testing.T, b []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v\nbody=%s", err, string(b))
	}
	return out
}

func requireErrorCode(t *testing.T, status int, body []byte, wantStatus int, wantCode string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status=%d want=%d body=%s", status, wantStatus, string(body))
	}
	got := mustUnmarshal[errorResponse](t, body)
	if got.Error.Code != wantCode {
		t.Fatalf("error.code=%q want=%q body=%s", got.Error.Code, wantCode, string(body))
	}
}

func requireHeaderPresent(t *testing.T, h http.Header, key string) {
	t.Helper()
	if strings.TrimSpace(h.Get(key)) == "" {
		t.Fatalf("expected header %q to be present", key)
	}
}
