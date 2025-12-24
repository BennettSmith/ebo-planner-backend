# Service Implementation Plan (Go) — Spec-first + Hexagonal + TDD

This plan is designed to be executed **top-to-bottom**. Progress is tracked via **GitHub-flavored checklists** (`- [ ]` / `- [x]`).

---

## How we track progress

- **Rule**: only check off a task when it meets its acceptance criteria.
- **Workflow**:
  - Pick the next unchecked task in the current milestone.
  - Do TDD (test first), implement, refactor, run `make test` + `make cover`.
  - Check the box and move on.

---

## Constraints / decisions (locked in)

- **HTTP**: `net/http` + `chi`
- **OpenAPI generation**: `oapi-codegen`
- **Server style**: **strict handler interfaces**
- **Auth**: in-scope (OpenAPI uses `bearerAuth` JWT)
- **DB (later)**: Postgres using `pgx` + `pgxpool`
- **Testing**: mostly unit tests for now; later API-layer integration tests
- **Coverage**: reporting only (no hard gate initially)

---

## Auth configuration (deployment-provided)

- [x] **JWT verification mode (now)**: **Real verification via JWKS**
  - Validate JWT signature via configured **JWKS URL**
  - Validate standard claims: `iss`, `aud`, `exp` (and `nbf` if present)
  - Extract authenticated subject from `sub`

Required runtime configuration (provided at deployment time):
- `JWT_ISSUER` (required)
- `JWT_AUDIENCE` (required)
- `JWT_JWKS_URL` (required)

Acceptance criteria:
- Middleware returns `401` on missing/invalid auth.
- On success, request context contains an **authenticated subject** we can map to a member (e.g., `subjectID` string).

---

## Current codebase starting point (what exists today)

- Generated OpenAPI code lives in `internal/adapters/httpapi/oas/` and is produced by `make gen-openapi`.
- HTTP router wiring exists in `internal/adapters/httpapi/router.go`.
- `cmd/api/main.go` starts a server using `oas.Unimplemented{}`.
- `internal/app`, `internal/domain`, `internal/ports`, `internal/adapters/postgres` exist but are currently empty.

---

## Target hexagonal architecture (package layout)

We’ll keep OpenAPI-generated DTOs isolated and translate them at the edges.

- **Domain** (`internal/domain`): entities/value objects + pure rules (no DB, no HTTP)
- **Application** (`internal/app`): use-cases (business workflows), depends on ports
- **Ports** (`internal/ports`):
  - inbound ports: what the app offers (use-case interfaces) — *optional, often just app services*
  - outbound ports: what the app needs (repos/stores/clock/auth) — *interfaces*
- **Adapters** (`internal/adapters/...`):
  - `httpapi`: HTTP delivery adapter (chi + oapi-codegen)
  - `memory`: in-memory implementations of outbound ports (first)
  - `postgres`: Postgres implementations of outbound ports (later)

---

## Spec-first workflow (OpenAPI → generated server glue)

We will treat `openapi.yaml` as the source of truth.

- [x] **Switch generation to strict server interfaces**
  - Update `Makefile` `gen-openapi` to generate strict server code (and regenerate).
  - Update `internal/adapters/httpapi` router wiring to use the strict handler wiring provided by `oapi-codegen`.

Acceptance criteria:
- `make gen-openapi` regenerates code cleanly.
- `go test ./...` succeeds after regeneration.

> Note: today the repo generates `types,chi-server` and does not include strict server types. We’ll align generation with “strict handler interfaces” before implementing handlers.

---

## Milestones

### Milestone 0 — Baseline dev ergonomics (fast feedback loop)

- [ ] Add `make test` target (`go test ./...`)
- [ ] Add `make cover` target (prints coverage summary; optionally writes `coverage.out`)
- [ ] Add `make fmt` target (`gofmt` + `goimports` if adopted)
- [ ] Document the local dev loop in `README.md` (how to run, test, regenerate OpenAPI)
- [ ] Add `.env.example` documenting required runtime config (including `JWT_ISSUER`, `JWT_AUDIENCE`, `JWT_JWKS_URL`)

Acceptance criteria:
- A new contributor can run: `make gen-openapi && make test && make cover`.

---

### Milestone 1 — HTTP adapter skeleton + auth + error mapping (no real business logic yet)

**Goal**: Stand up the HTTP layer “for real” (generated strict handlers, middleware, consistent errors), still backed by stub/in-memory app services.

- [ ] Implement **auth middleware** for `bearerAuth` (real JWT verification)
  - Extract `Authorization: Bearer ...`
  - Verify signature (JWKS)
  - Validate claims (`iss`, `aud`, `exp`; optionally `nbf`)
  - Populate request context with `subjectID` (from `sub`)
  - Return OpenAPI-shaped `401` error on failure
- [ ] Add auth configuration in `internal/platform/config` (or equivalent)
  - `JWT_ISSUER` (required)
  - `JWT_AUDIENCE` (required)
  - `JWT_JWKS_URL` (required)
  - Timeouts and caching: `JWT_JWKS_REFRESH_INTERVAL`, `JWT_JWKS_MIN_REFRESH_INTERVAL`, `JWT_CLOCK_SKEW` (names TBD)
- [ ] Add JWKS caching + rotation behavior
  - Cache keys in-memory
  - Refresh on interval, and on unknown `kid` (bounded to avoid thundering herd)
  - Fail closed (401) when verification cannot be performed
- [ ] TDD for auth middleware
  - Missing header → 401
  - Malformed `Authorization` → 401
  - Token with bad signature → 401
  - Token expired → 401
  - Wrong `iss`/`aud` → 401
  - Valid token → request proceeds; `subjectID` present in context
  - JWKS rotates keys → old `kid` rejected, new accepted after refresh
  - Tests run without external dependencies by using an in-test JWKS server (`httptest`) serving a generated public key set
- [ ] Define a small **HTTP request context** helper in `internal/adapters/httpapi` (e.g., `SubjectFromContext(ctx)`)
- [ ] Define **error mapping** conventions:
  - application/domain error → OpenAPI `ErrorResponse` (`code`, `message`, optional `details`, `requestId`)
  - map “not found/validation/conflict/unauthorized” consistently
- [ ] Implement **health** endpoints policy:
  - keep `/healthz` out-of-spec (already exists) for infra checks
  - (optional) add `/readyz` if we want readiness checks later (DB connectivity, etc.)
- [ ] Replace `oas.Unimplemented{}` in `cmd/api/main.go` with a real server implementation that:
  - delegates into the application layer (still using in-memory adapters)
  - returns JSON responses matching the spec

Acceptance criteria:
- A protected endpoint returns `401` when auth header is missing.
- With auth header, endpoints return “not implemented yet” *only where expected* (we’ll remove these as we implement milestones).
- Errors match `components/schemas/ErrorResponse`.

---

### Milestone 2 — Ports + in-memory adapters (foundation for TDD)

**Goal**: Define outbound ports and provide in-memory adapters to support fast unit tests.

- [ ] Create outbound port interfaces in `internal/ports` (suggested packages):
  - `internal/ports/out/memberrepo`
  - `internal/ports/out/triprepo`
  - `internal/ports/out/rsvprepo`
  - `internal/ports/out/idempotency`
  - `internal/ports/out/clock` (if we want deterministic time)
- [ ] Implement in-memory adapters in `internal/adapters/memory/...`
  - deterministic time + controllable clock for tests
  - thread-safe stores (mutex) so `httptest`/concurrency doesn’t flake
  - optional: simple “transaction-ish” boundary (copy-on-write) for test determinism
- [ ] Add unit tests for each in-memory adapter (behavior + race-safety expectations)

Acceptance criteria:
- App layer can be instantiated with only in-memory adapters.
- `go test -race ./...` passes (when we choose to run it locally).

---

### Milestone 3 — Members use-cases end-to-end (HTTP → app → memory)

**Endpoints**:
- `GET /members`
- `GET /members/search`
- `POST /members` (createMyMember)
- `GET /members/me`
- `PATCH /members/me`

- [ ] Define domain model for `Member` (IDs, display name, email, vehicle profile)
- [ ] Implement application services for Member use-cases in `internal/app`
- [ ] Implement HTTP adapter handlers (translate OAS DTOs ↔ domain/app types)
- [ ] Unit tests:
  - app-layer tests for each use-case (happy + edge cases)
  - adapter tests for request/response mapping (table-driven)

Acceptance criteria:
- All Member endpoints behave per spec and use cases docs.
- `422` validation errors follow a consistent shape (even if details are minimal at first).

---

### Milestone 4 — Trips read paths (visibility rules, details, listings)

**Endpoints**:
- `GET /trips`
- `GET /trips/drafts`
- `GET /trips/{tripId}`

- [ ] Define domain model for `Trip` read projection(s): summary vs details
- [ ] Implement visibility rules (published/canceled visible; drafts visible per rules)
- [ ] Implement app services + in-memory repos to support read queries
- [ ] Implement HTTP handlers + tests

Acceptance criteria:
- Returned JSON matches OpenAPI schemas (`TripSummary`, `TripDetails`, etc.).
- Visibility rules are covered by unit tests.

---

### Milestone 5 — Trips write paths (draft/publish/cancel/update/organizers)

**Endpoints**:
- `POST /trips` (create draft)
- `PATCH /trips/{tripId}` (update)
- `PUT /trips/{tripId}/draft-visibility`
- `POST /trips/{tripId}/publish`
- `POST /trips/{tripId}/cancel`
- `POST /trips/{tripId}/organizers`
- `DELETE /trips/{tripId}/organizers/{memberId}`

- [ ] Implement trip lifecycle invariants (draft → published → canceled)
- [ ] Implement organizer authorization checks (creator vs organizer)
- [ ] Implement idempotency behavior for endpoints requiring `Idempotency-Key`
  - minimal first pass: dedupe by key + route + subject + request body hash
  - store the prior response body/status for replay
- [ ] Unit test suite for trip invariants + idempotency
- [ ] HTTP handler tests for each endpoint mapping + error cases

Acceptance criteria:
- Mutations are idempotent where required by the spec.
- `409` vs `422` vs `404` are consistently applied.

---

### Milestone 6 — RSVP use-cases (write + read)

**Endpoints**:
- `PUT /trips/{tripId}/rsvp`
- `GET /trips/{tripId}/rsvp/me`
- `GET /trips/{tripId}/rsvps`

- [ ] Implement RSVP domain + invariants (YES/NO/UNSET, timestamps)
- [ ] Implement app services + in-memory storage
- [ ] Ensure RSVP counts affect `attendingRigs` rules (as defined by use-cases)
- [ ] Tests for RSVP summary correctness and edge cases

Acceptance criteria:
- RSVP summary matches OpenAPI schema and updates correctly after changes.

---

### Milestone 7 (later) — Postgres adapters (pgxpool) + migrations alignment

**Goal**: Replace in-memory adapters with Postgres-backed ones behind the same ports.

- [ ] Define Postgres schema ownership rules (migrations are the source of truth)
- [ ] Implement `internal/adapters/postgres`:
  - connect via `pgxpool.Pool`
  - implement repositories for members/trips/rsvps/idempotency
  - transaction boundaries where needed
- [ ] Add “repository contract tests” runnable against both memory + postgres adapters
- [ ] Add seed/dev data strategy (keep seed scripts out of migrations; already true)

Acceptance criteria:
- App runs with either memory or Postgres adapters via configuration.
- Repo contract tests pass for both adapter sets.

---

### Milestone 8 (later) — API-layer integration tests (httptest)

**Goal**: Comprehensive black-box tests from HTTP layer down (initially with in-memory, later with Postgres).

- [ ] Add `internal/adapters/httpapi/itest` package (or `internal/itest`)
- [ ] For each endpoint:
  - happy path
  - auth failure
  - validation failure
  - not-found/conflict cases
- [ ] Add “golden JSON” response assertions where stable

Acceptance criteria:
- Running `go test ./...` executes API integration tests deterministically.

---

## Testing strategy (TDD-first)

- **Domain tests**: invariants and transformations (pure, fast)
- **App tests**: use-case behavior using fakes/memory adapters (fast)
- **Adapter tests (HTTP)**: request mapping + response mapping + error mapping
- **Later integration tests**: `httptest` from router down

---

## Local testing notes (JWT verification)

We want JWT verification tests to be **fully local** and **deterministic** (no calls to a real IdP).

- **Approach**:
  - Generate an RSA keypair in-test.
  - Stand up a local `httptest` server that serves a JWKS containing the public key.
  - Configure the verifier with:
    - `JWT_JWKS_URL` = the test server URL
    - `JWT_ISSUER` / `JWT_AUDIENCE` = fixed test values
  - Mint JWTs in-test with the private key (vary `iss`, `aud`, `exp`, `kid`) and assert `401` vs success.
- **Where helpers live**:
  - Prefer `internal/platform/auth/jwks_testutil` (or similar) so tests across packages can reuse:
    - JWKS server helper
    - token minting helper
    - clock control (for expiry tests)

---

## Coverage reporting

- [ ] `make cover` prints a readable summary (and optionally writes `coverage.out`)
- [ ] Optional: `make cover-html` to generate/report a browsable HTML report

Acceptance criteria:
- Coverage is easy to inspect locally and in CI logs (even without gating).


