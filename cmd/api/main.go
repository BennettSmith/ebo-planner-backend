package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ebo-planner-backend/internal/adapters/httpapi"
	memidempotency "ebo-planner-backend/internal/adapters/memory/idempotency"
	memmemberrepo "ebo-planner-backend/internal/adapters/memory/memberrepo"
	memrsvprepo "ebo-planner-backend/internal/adapters/memory/rsvprepo"
	memtriprepo "ebo-planner-backend/internal/adapters/memory/triprepo"
	postgres "ebo-planner-backend/internal/adapters/postgres"
	pgidempotency "ebo-planner-backend/internal/adapters/postgres/idempotency"
	pgmemberrepo "ebo-planner-backend/internal/adapters/postgres/memberrepo"
	pgrsvprepo "ebo-planner-backend/internal/adapters/postgres/rsvprepo"
	pgtriprepo "ebo-planner-backend/internal/adapters/postgres/triprepo"
	"ebo-planner-backend/internal/app/members"
	"ebo-planner-backend/internal/app/trips"
	"ebo-planner-backend/internal/platform/auth/jwtverifier"
	platformclock "ebo-planner-backend/internal/platform/clock"
	"ebo-planner-backend/internal/platform/config"
	idempotencyport "ebo-planner-backend/internal/ports/out/idempotency"
	memberrepoport "ebo-planner-backend/internal/ports/out/memberrepo"
	rsvprepoport "ebo-planner-backend/internal/ports/out/rsvprepo"
	triprepoport "ebo-planner-backend/internal/ports/out/triprepo"
)

func main() {
	port := getenv("PORT", "8080")

	// Auth configuration:
	// - Production: require JWT_* env vars and enforce bearer auth
	// - Local dev: set AUTH_MODE=dev to bypass JWT verification and use X-Debug-Subject
	authMode := getenv("AUTH_MODE", "jwt")
	var authMW func(http.Handler) http.Handler
	authIssuer := ""
	switch authMode {
	case "dev":
		authMW = httpapi.NewDevAuthMiddleware(getenv("DEV_SUBJECT", "dev|local"))
		authIssuer = getenv("DEV_ISSUER", "dev")
	default:
		jwtCfg, err := config.LoadJWTConfigFromEnv()
		if err != nil {
			log.Fatalf("invalid auth config: %v", err)
		}
		verifier := jwtverifier.New(jwtCfg)
		authMW = httpapi.NewAuthMiddleware(verifier)
		authIssuer = jwtCfg.Issuer
	}

	clk := platformclock.NewSystemClock()

	storageBackend := getenv("STORAGE_BACKEND", "memory")
	var (
		memberRepo memberrepoport.Repository
		tripRepo   triprepoport.Repository
		rsvpRepo   rsvprepoport.Repository
		idemStore  idempotencyport.Store
		cleanup    func()
	)

	switch storageBackend {
	case "postgres":
		dsn := os.Getenv("DATABASE_URL")
		pool, err := postgres.NewPool(context.Background(), dsn, postgres.PoolOptions{})
		if err != nil {
			log.Fatalf("invalid postgres config: %v", err)
		}
		cleanup = pool.Close

		memberRepo = pgmemberrepo.NewRepo(pool, authIssuer)
		tripRepo = pgtriprepo.NewRepo(pool)
		rsvpRepo = pgrsvprepo.NewRepo(pool)
		idemStore = pgidempotency.NewStore(pool, authIssuer)
	default:
		memberRepo = memmemberrepo.NewRepo()
		tripRepo = memtriprepo.NewRepo()
		rsvpRepo = memrsvprepo.NewRepo()
		idemStore = memidempotency.NewStore()
	}

	if cleanup != nil {
		defer cleanup()
	}

	memberSvc := members.NewService(memberRepo, clk)
	tripSvc := trips.NewService(tripRepo, memberRepo, rsvpRepo)

	// Real server implementation for Members; other endpoints remain strict-unimplemented.
	api := httpapi.NewServer(memberSvc, tripSvc, idemStore)

	handler := httpapi.NewRouterWithOptions(
		api,
		httpapi.RouterOptions{AuthMiddleware: authMW},
	)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("api listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
