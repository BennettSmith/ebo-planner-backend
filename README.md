# East Bay Overland — Local Dev

This repo uses:

- **Postgres via Docker Compose** (`db` service)
- **golang-migrate** via the `migrate/migrate` image (`migrate` service)
- A Go API (`api` service) behind Caddy locally (`caddy` service)

### Local dev (DB + API)

Bring up the database and API (and the local proxy):

```bash
docker compose up -d --build db api caddy
```

Database connection string (from host):

```bash
postgres://eb:eb@localhost:5432/eastbay?sslmode=disable
```

### Run migrations

Apply migrations (defaults to `up`):

```bash
docker compose run --rm migrate
```

Reset schema (destructive):

```bash
docker compose run --rm migrate down -all
docker compose run --rm migrate
```

### Optional: dev seed (NOT part of migrations)

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
