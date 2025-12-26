package rsvprepo

import (
	"context"
	"time"

	"ebo-planner-backend/internal/domain"
)

type Status string

const (
	StatusYes   Status = "YES"
	StatusNo    Status = "NO"
	StatusUnset Status = "UNSET"
)

type RSVP struct {
	TripID   domain.TripID
	MemberID domain.MemberID

	Status    Status
	UpdatedAt time.Time
}

type Repository interface {
	// Get returns the RSVP for (trip, member). If it does not exist, ErrNotFound is returned.
	Get(ctx context.Context, tripID domain.TripID, memberID domain.MemberID) (RSVP, error)

	// Upsert writes the RSVP for (trip, member) using last-write-wins semantics.
	Upsert(ctx context.Context, r RSVP) error

	// ListByTrip returns all RSVP records for a trip.
	ListByTrip(ctx context.Context, tripID domain.TripID) ([]RSVP, error)

	// CountYesByTrip counts RSVP=YES for the specified trip.
	CountYesByTrip(ctx context.Context, tripID domain.TripID) (int, error)
}
