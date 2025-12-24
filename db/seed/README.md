# Dev seed scripts (optional)

- **Schema source of truth**: `migrations/` (golang-migrate). Seed scripts must **not** be added to migrations.
- **Intended use**: local/dev/demo environments only.

## Run the seed against a running local DB container

```bash
docker compose exec -T db psql -U eb -d eastbay -v ON_ERROR_STOP=1 < db/seed/seed_dev_optional.sql
```

## Run the seed from your host via `DATABASE_URL`

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/seed/seed_dev_optional.sql
```


