package idempotency

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	postgres "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/idempotency"
)

// Store is a Postgres implementation of idempotency.Store.
type Store struct {
	pool   *pgxpool.Pool
	issuer string
}

func NewStore(pool *pgxpool.Pool, jwtIssuer string) *Store {
	return &Store{pool: pool, issuer: jwtIssuer}
}

func (s *Store) Get(ctx context.Context, fp idempotency.Fingerprint) (idempotency.Record, bool, error) {
	if s.pool == nil {
		return idempotency.Record{}, false, errors.New("nil postgres pool")
	}
	row := s.pool.QueryRow(ctx, `
		SELECT status_code, content_type, body, created_at
		FROM idempotency_keys
		WHERE idempotency_key = $1
		  AND subject_iss = $2
		  AND subject_sub = $3
		  AND method = $4
		  AND route = $5
		  AND body_hash = $6
	`,
		string(fp.Key),
		s.issuer,
		string(fp.Subject),
		fp.Method,
		fp.Route,
		fp.BodyHash,
	)
	var rec idempotency.Record
	if err := row.Scan(&rec.StatusCode, &rec.ContentType, &rec.Body, &rec.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return idempotency.Record{}, false, nil
		}
		return idempotency.Record{}, false, err
	}
	rec.CreatedAt = rec.CreatedAt.UTC()
	return rec, true, nil
}

func (s *Store) Put(ctx context.Context, fp idempotency.Fingerprint, rec idempotency.Record) error {
	if s.pool == nil {
		return errors.New("nil postgres pool")
	}
	createdAt := rec.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO idempotency_keys (
			idempotency_key,
			subject_iss,
			subject_sub,
			method,
			route,
			body_hash,
			status_code,
			content_type,
			body,
			created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (idempotency_key, subject_iss, subject_sub, method, route, body_hash)
		DO UPDATE SET
			status_code = EXCLUDED.status_code,
			content_type = EXCLUDED.content_type,
			body = EXCLUDED.body,
			created_at = EXCLUDED.created_at
	`,
		string(fp.Key),
		s.issuer,
		string(fp.Subject),
		fp.Method,
		fp.Route,
		fp.BodyHash,
		rec.StatusCode,
		rec.ContentType,
		rec.Body,
		createdAt.UTC(),
	)
	if err != nil {
		if pe, ok := postgres.AsPgError(err); ok && pe.Code == postgres.ForeignKeyViolationCode {
			// No FK constraints exist for subject-based idempotency in v1; treat as generic error.
			return err
		}
		return err
	}
	return nil
}
