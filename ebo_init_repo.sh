#!/usr/bin/env bash
set -euo pipefail

# Overland East Bay — repo bootstrap (hexagonal Go layout)
# Safe to re-run. Creates directories and starter files if missing.
#
# Assumptions:
# - You already have: ./Makefile (kept as-is), ./Dockerfile (kept as-is)
# - You may or may not have: go.mod, existing source files, etc.

ROOT="$(pwd)"

say() { printf "\n\033[1m%s\033[0m\n" "$*"; }
mkd() { mkdir -p "$1"; }
touch_if_missing() { [[ -f "$1" ]] || { mkdir -p "$(dirname "$1")"; cat >"$1"; chmod 0644 "$1"; }; }

say "Bootstrapping folder structure in: $ROOT"

# --- Core Go hexagon layout ---
mkd "cmd/api"
mkd "internal/domain"
mkd "internal/app"
mkd "internal/ports"
mkd "internal/adapters/http"
mkd "internal/adapters/postgres"
mkd "internal/platform/config"
mkd "internal/platform/logging"
mkd "internal/platform/clock"

# --- DB migrations ---
mkd "migrations"

# --- Local reverse proxy config (Caddy) ---
mkd "deploy"

# --- Optional but handy ---
mkd "scripts"
mkd "docs"

# Starter .gitkeep files so empty dirs are retained (idempotent)
for d in \
  cmd/api \
  internal/domain internal/app internal/ports \
  internal/adapters/http internal/adapters/postgres \
  internal/platform/config internal/platform/logging internal/platform/clock \
  migrations deploy scripts docs
do
  [[ -f "$d/.gitkeep" ]] || : > "$d/.gitkeep"
done

# --- Starter files (only created if missing) ---

# Minimal main.go (wiring only; fill in router/use-cases later)
if [[ ! -f "cmd/api/main.go" ]]; then
  say "Creating cmd/api/main.go (minimal skeleton)"
  cat > "cmd/api/main.go" <<'EOF'
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := getenv("PORT", "8080")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
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
EOF
fi

# Caddyfile for local reverse proxy (creates only if missing)
if [[ ! -f "deploy/Caddyfile" ]]; then
  say "Creating deploy/Caddyfile (local reverse proxy)"
  cat > "deploy/Caddyfile" <<'EOF'
:80 {
	encode gzip

	# Proxy to the API container.
	reverse_proxy api:8080 {
		header_up X-Forwarded-Proto {scheme}
		header_up X-Forwarded-Host {host}
		header_up X-Forwarded-For {remote_host}
	}

	# Optional proxy-level health endpoint
	respond /proxy-healthz 200
}
EOF
fi

# docker-compose.yml (creates only if missing).
# Note: we won't overwrite your existing one if you already have it.
if [[ ! -f "docker-compose.yml" && ! -f "compose.yml" ]]; then
  say "Creating docker-compose.yml (db + api + migrate + caddy)"
  cat > "docker-compose.yml" <<'EOF'
services:
  caddy:
    image: caddy:2
    ports:
      - "8081:80" # host:container
    volumes:
      - ./deploy/Caddyfile:/etc/caddy/Caddyfile:ro
    depends_on:
      - api

  db:
    image: postgres:16
    environment:
      POSTGRES_USER: eb
      POSTGRES_PASSWORD: eb
      POSTGRES_DB: eastbay
    ports:
      - "5432:5432" # optional; remove if you don't want host access
    volumes:
      - dbdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U eb -d eastbay"]
      interval: 5s
      timeout: 3s
      retries: 20

  migrate:
    image: migrate/migrate:v4.17.1
    depends_on:
      db:
        condition: service_healthy
    volumes:
      - ./migrations:/migrations:ro
    entrypoint:
      - migrate
      - -path=/migrations
      - -database
      - postgres://eb:eb@db:5432/eastbay?sslmode=disable
    command: ["up"]

  api:
    build:
      context: .
      dockerfile: Dockerfile
    depends_on:
      db:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://eb:eb@db:5432/eastbay?sslmode=disable
      PORT: "8080"
      TRUST_PROXY_HEADERS: "true"
      PUBLIC_BASE_URL: http://localhost:8081
      # Auth placeholders
      OIDC_ISSUER: http://localhost:5556
      JWT_AUDIENCE: east-bay-overland
    expose:
      - "8080"

volumes:
  dbdata:
EOF
fi

# .env.example (creates only if missing)
if [[ ! -f ".env.example" ]]; then
  say "Creating .env.example"
  cat > ".env.example" <<'EOF'
# Copy to .env for local dev
PORT=8080
DATABASE_URL=postgres://eb:eb@db:5432/eastbay?sslmode=disable

# When behind a reverse proxy (like Caddy), enable trusting forwarded headers.
TRUST_PROXY_HEADERS=true
PUBLIC_BASE_URL=http://localhost:8081

# Auth placeholders (replace with your actual issuer/audience)
OIDC_ISSUER=http://localhost:5556
JWT_AUDIENCE=east-bay-overland
EOF
fi

# README snippet (creates only if missing)
if [[ ! -f "README.md" ]]; then
  say "Creating README.md (minimal)"
  cat > "README.md" <<'EOF'
# Overland East Bay — Trip Planning Service (Go)

## Local dev (Docker)
- Reverse proxy (Caddy): http://localhost:8081
- Postgres: localhost:5432 (user: eb, pass: eb, db: eastbay)

### Start
```bash
docker compose up -d --build
```

### Run migrations (from ./migrations)
```bash
docker compose run --rm migrate
```

### Health checks
- API: http://localhost:8081/healthz
- Proxy: http://localhost:8081/proxy-healthz
```
EOF
fi

say "Done."
say "Notes:"
echo "- Schema is managed via ./migrations (golang-migrate)."
echo "- Your existing Makefile and Dockerfile were not modified."
echo "- docker-compose.yml was created only if you didn't already have docker-compose.yml or compose.yml."
echo ""
echo "Next steps:"
echo "  1) Put SQL migrations in ./migrations."
echo "  2) Implement routes/adapters/use-cases under ./internal."
echo "  3) Run: docker compose up -d --build"
