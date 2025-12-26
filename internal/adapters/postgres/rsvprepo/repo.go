package rsvprepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ebo-planner-backend/internal/domain"
	"ebo-planner-backend/internal/ports/out/rsvprepo"
)

// Repo is a Postgres implementation of rsvprepo.Repository.
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Get(ctx context.Context, tripID domain.TripID, memberID domain.MemberID) (rsvprepo.RSVP, error) {
	if r.pool == nil {
		return rsvprepo.RSVP{}, errors.New("nil postgres pool")
	}
	tid, err := uuid.Parse(string(tripID))
	if err != nil {
		return rsvprepo.RSVP{}, rsvprepo.ErrNotFound
	}
	mid, err := uuid.Parse(string(memberID))
	if err != nil {
		return rsvprepo.RSVP{}, rsvprepo.ErrNotFound
	}

	row := r.pool.QueryRow(ctx, `
		SELECT r.response, r.updated_at
		FROM trip_rsvps r
		JOIN trips t ON t.id = r.trip_id
		JOIN members m ON m.id = r.member_id
		WHERE t.external_id = $1 AND m.external_id = $2
	`, tid, mid)
	var status string
	var updatedAt time.Time
	if err := row.Scan(&status, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rsvprepo.RSVP{}, rsvprepo.ErrNotFound
		}
		return rsvprepo.RSVP{}, err
	}
	return rsvprepo.RSVP{
		TripID:    tripID,
		MemberID:  memberID,
		Status:    rsvprepo.Status(status),
		UpdatedAt: updatedAt.UTC(),
	}, nil
}

func (r *Repo) Upsert(ctx context.Context, rec rsvprepo.RSVP) error {
	if r.pool == nil {
		return errors.New("nil postgres pool")
	}
	tid, err := uuid.Parse(string(rec.TripID))
	if err != nil {
		return fmt.Errorf("invalid trip id: %w", err)
	}
	mid, err := uuid.Parse(string(rec.MemberID))
	if err != nil {
		return fmt.Errorf("invalid member id: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO trip_rsvps (trip_id, member_id, response, updated_at)
		VALUES (
			(SELECT id FROM trips WHERE external_id = $1),
			(SELECT id FROM members WHERE external_id = $2),
			$3,
			$4
		)
		ON CONFLICT (trip_id, member_id) DO UPDATE
		SET response = EXCLUDED.response,
		    updated_at = EXCLUDED.updated_at
	`, tid, mid, string(rec.Status), rec.UpdatedAt.UTC())
	return err
}

func (r *Repo) ListByTrip(ctx context.Context, tripID domain.TripID) ([]rsvprepo.RSVP, error) {
	if r.pool == nil {
		return nil, errors.New("nil postgres pool")
	}
	tid, err := uuid.Parse(string(tripID))
	if err != nil {
		return []rsvprepo.RSVP{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT m.external_id, r.response, r.updated_at
		FROM trip_rsvps r
		JOIN trips t ON t.id = r.trip_id
		JOIN members m ON m.id = r.member_id
		WHERE t.external_id = $1
		ORDER BY m.external_id ASC, r.updated_at ASC
	`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]rsvprepo.RSVP, 0)
	for rows.Next() {
		var mid uuid.UUID
		var status string
		var updatedAt time.Time
		if err := rows.Scan(&mid, &status, &updatedAt); err != nil {
			return nil, err
		}
		out = append(out, rsvprepo.RSVP{
			TripID:    tripID,
			MemberID:  domain.MemberID(mid.String()),
			Status:    rsvprepo.Status(status),
			UpdatedAt: updatedAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repo) CountYesByTrip(ctx context.Context, tripID domain.TripID) (int, error) {
	if r.pool == nil {
		return 0, errors.New("nil postgres pool")
	}
	tid, err := uuid.Parse(string(tripID))
	if err != nil {
		return 0, nil
	}
	row := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM trip_rsvps r
		JOIN trips t ON t.id = r.trip_id
		WHERE t.external_id = $1 AND r.response = 'YES'
	`, tid)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
