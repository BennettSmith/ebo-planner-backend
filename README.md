# East Bay Overland — Local Dev

This repo uses:

- **Postgres via Docker Compose** (`db` service)
- **golang-migrate** via the `migrate/migrate` image (`migrate` service)
- A Go API (`api` service) behind Caddy locally (`caddy` service)

## Diagrams

- **Domain model (v1)**: `docs/diagrams/domain-model.md`
- **Database schema (v1)**: `docs/diagrams/database-schema.md`

## Local dev (DB + API)

Bring up the database and API (and the local proxy):

```bash
docker compose up -d --build db api caddy
```

Or, use the repo `Makefile` helpers (recommended):

```bash
make up
make db-migrate
```

### Developer flows (Docker Desktop)

Common compose-based workflows:

- **Start everything (db + migrate + api + caddy)**:

```bash
make up
```

- **Local dev auth**: this compose setup runs the API with `AUTH_MODE=dev`, which means:
  - Requests authenticate via `X-Debug-Subject: <some-subject>`
  - The API still enforces “member must be provisioned” for many endpoints, so you’ll typically create a member first.

- **JWT auth options (real Bearer tokens)**: set `AUTH_MODE=jwt` and provide `JWT_ISSUER`, `JWT_AUDIENCE`, and `JWT_JWKS_URL`.

#### Option A: local `devjwt` (fastest, no external IdP)

This repo includes a tiny `devjwt` service that:
- serves JWKS at `/.well-known/jwks.json`
- mints RS256 tokens at `/token?sub=...`

Start the stack in JWT mode (this turns on real Bearer verification in the API):

```bash
make up AUTH_MODE=jwt
```

Mint a token (from host) and call the API:

```bash
# Requires jq (brew install jq)
TOKEN="$(curl -sS 'http://localhost:5556/token?sub=dev%7Calice' | jq -r .token)"
curl -sS http://localhost:8081/members -H "Authorization: Bearer $TOKEN"
```

If you want to override the expected issuer/audience/JWKS in docker compose:

```bash
make up AUTH_MODE=jwt \
  JWT_ISSUER='http://devjwt:5556' \
  JWT_AUDIENCE='east-bay-overland' \
  JWT_JWKS_URL='http://devjwt:5556/.well-known/jwks.json'
```

#### Option B: Keycloak (more realistic)

You can run Keycloak locally via docker compose (recommended) or run it elsewhere (Docker Desktop, k8s, hosted).

##### Run Keycloak locally (docker compose)

This repo includes a `keycloak` compose service (disabled by default) and a small realm import:
- Realm: `ebo`
- Client: `ebo-api` (direct access grants enabled)
- User: `alice` / password: `alice`

Start the stack with Keycloak + JWT auth:

```bash
make up-keycloak
```

Keycloak Admin UI:

```bash
make keycloak-ui
```

Then open `http://localhost:8082/admin` and log in as `admin` / `admin`.

Get a token from Keycloak (password grant) and call the API:

```bash
TOKEN="$(make token-keycloak)"

# First-time: provision a member for this subject (sub comes from the JWT)
curl -sS -X POST http://localhost:8081/members \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"displayName":"Alice","email":"alice@example.com"}'

curl -sS http://localhost:8081/members -H "Authorization: Bearer $TOKEN"
```

If Keycloak is still starting, tail logs:

```bash
make logs-keycloak
```

##### Point the API at an existing Keycloak

If you run Keycloak elsewhere, point the API at it:

- `JWT_ISSUER`: Keycloak realm issuer, typically `http://localhost:8082/realms/<realm>`
- `JWT_JWKS_URL`: typically `http://localhost:8082/realms/<realm>/protocol/openid-connect/certs`
- `JWT_AUDIENCE`: whatever your token’s `aud` contains for this API (configure via client/audience settings)

Example (Keycloak on host port `8082`, realm `ebo`, client `ebo-api`):

```bash
make up AUTH_MODE=jwt \
  JWT_ISSUER='http://host.docker.internal:8082/realms/ebo' \
  JWT_JWKS_URL='http://host.docker.internal:8082/realms/ebo/protocol/openid-connect/certs' \
  JWT_AUDIENCE='ebo-api'
```

Then fetch a token (password grant shown for quick local testing; use whichever grant you prefer):

```bash
TOKEN="$(
  curl -sS -X POST 'http://localhost:8082/realms/ebo/protocol/openid-connect/token' \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode 'grant_type=password' \
    --data-urlencode 'client_id=ebo-api' \
    --data-urlencode 'username=alice' \
    --data-urlencode 'password=alice' \
  | jq -r .access_token
)"
curl -sS http://localhost:8081/members -H "Authorization: Bearer $TOKEN"
```

Notes:
- The API’s verifier currently requires **RS256** and a JWT header `kid`.
- Ensure your token’s `iss` matches `JWT_ISSUER` exactly (including scheme/host/port).
- If you see `aud mismatch`, adjust `JWT_AUDIENCE` or Keycloak client/audience mapper so the token includes the expected audience.

- **If you get “port 5432 is already allocated”** (another Postgres is already using it):

```bash
make up POSTGRES_PORT=5433
```

- **Check status / ports**:

```bash
make ps
```

- **Tail logs**:

```bash
make logs-api   # API only
make logs-all   # everything
```

- **Rebuild only the API image (compose)**:

```bash
make rebuild-api
make up
```

- **Reset the Postgres volume (destructive; wipes all local data)**:

```bash
make reset-volumes
make up
```

Optional: build/run the API image without compose (useful for validating the container itself):

```bash
make image
make image-run DATABASE_URL='postgres://eb:eb@host.docker.internal:5432/eastbay?sslmode=disable'
```

### Quick curl checks

- **API health (through Caddy)**:

```bash
curl -i http://localhost:8081/healthz
```

- **Create a member (first-time setup for a subject)**:

```bash
curl -sS -X POST http://localhost:8081/members \
  -H 'Content-Type: application/json' \
  -H 'X-Debug-Subject: dev|alice' \
  -d '{"displayName":"Alice","email":"alice@example.com"}'
```

- **List members**:

```bash
curl -sS http://localhost:8081/members -H 'X-Debug-Subject: dev|alice'
```

Database connection string (from host):

```bash
postgres://eb:eb@localhost:5432/eastbay?sslmode=disable
```

## Fast dev loop (spec-first)

OpenAPI is the source of truth (`openapi.yaml`). Regenerate server glue, format, and run tests:

```bash
make gen-openapi
make fmt
make test
make cover
```

If you want an HTML coverage report:

```bash
make cover-html
```

## Integration tests (HTTP black-box)

Integration tests live in `internal/adapters/httpapi/itest` and spin up an `httptest` server using the real HTTP router + handlers.

- **Run integration tests with the in-memory backend (default)**:

```bash
make itest
```

- **Run integration tests with Postgres** (destructive: resets `public` schema via migrations; use a disposable DB):

```bash
make itest-postgres PG_DSN='postgres://eb:eb@localhost:5432/eastbay?sslmode=disable'
```

- **Run both** (Postgres subtests auto-skip if `PG_DSN` is unset):

```bash
make itest-all
```

## Environment variables

- **Auth (required)**:
  - `JWT_ISSUER`
  - `JWT_AUDIENCE`
  - `JWT_JWKS_URL`
- **Storage backend**:
  - `STORAGE_BACKEND`: `memory` (default) or `postgres`
  - `DATABASE_URL`: required when `STORAGE_BACKEND=postgres`
- **Postgres contract tests (optional)**:
  - `PG_DSN`: if set, Postgres adapter contract tests will run (they reset the `public` schema; use a disposable database).
- **HTTP integration tests (optional)**:
  - `ITEST_BACKEND`: `memory` (default), `postgres`, or `all`
  - `PG_DSN`: required when `ITEST_BACKEND=postgres` (also used by contract tests; destructive: resets `public` schema)

## Run migrations

Apply migrations (defaults to `up`):

```bash
docker compose run --rm migrate
```

Reset schema (destructive):

```bash
docker compose run --rm migrate down -all
docker compose run --rm migrate
```

## Optional: dev seed (NOT part of migrations)

Seed data lives in `db/seed/seed_dev_optional.sql` and is intentionally **not** included in `migrations/`.

If you want sample data in a local dev database, you can run it against the running container:

```bash
docker compose exec -T db psql -U eb -d eastbay -v ON_ERROR_STOP=1 < db/seed/seed_dev_optional.sql
```

Or, run it from your host with `DATABASE_URL`:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/seed/seed_dev_optional.sql
```

Example host DB URL (when using Docker Compose locally):

```bash
postgres://eb:eb@localhost:5432/eastbay?sslmode=disable
```
