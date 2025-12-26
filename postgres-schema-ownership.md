# Postgres schema ownership (Milestone 7)

## Source of truth

- **Migrations in `migrations/` are the source of truth** for the database schema.
- **No manual schema edits** (no ad-hoc `ALTER TABLE` in prod/dev) outside migrations.
- **Adapters must target the migrations**, not an imagined schema.

## Where invariants live

- **Database constraints/triggers** enforce cross-row invariants and race-prone rules (e.g. capacity, state transitions).
- **Application layer** enforces request validation and authorization and should fail fast with domain/application errors.
- When both enforce the same rule, **the app is the primary user-facing error**, and the DB is the correctness backstop.

## Adding/changing schema

- Add a new numbered migration pair:
  - `migrations/0000xx_name.up.sql`
  - `migrations/0000xx_name.down.sql`
- Migrations must be:
  - **Idempotent** where feasible (use `IF NOT EXISTS` guards).
  - **Forward-only safe** for local dev (down migrations are best-effort).
- Any schema change that affects persistence behavior should come with:
  - A **repository contract test** update (so memory + Postgres stay behavior-aligned).
  - Adapter updates in `internal/adapters/postgres`.


