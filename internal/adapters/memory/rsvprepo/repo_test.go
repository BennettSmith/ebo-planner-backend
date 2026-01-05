package rsvprepo

import (
	"context"
	"testing"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/rsvprepo"
)

func TestRepo_GetUpsertCountYesList(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	tripID := domain.TripID("t1")

	_, err := r.Get(context.Background(), tripID, "m1")
	if err != rsvprepo.ErrNotFound {
		t.Fatalf("Get(nonexistent) err=%v, want %v", err, rsvprepo.ErrNotFound)
	}

	t1 := time.Unix(10, 0).UTC()
	t2 := time.Unix(20, 0).UTC()

	if err := r.Upsert(context.Background(), rsvprepo.RSVP{TripID: tripID, MemberID: "m2", Status: rsvprepo.StatusNo, UpdatedAt: t2}); err != nil {
		t.Fatalf("Upsert(m2) err=%v", err)
	}
	if err := r.Upsert(context.Background(), rsvprepo.RSVP{TripID: tripID, MemberID: "m1", Status: rsvprepo.StatusYes, UpdatedAt: t1}); err != nil {
		t.Fatalf("Upsert(m1) err=%v", err)
	}

	got, err := r.Get(context.Background(), tripID, "m1")
	if err != nil {
		t.Fatalf("Get(m1) err=%v", err)
	}
	if got.Status != rsvprepo.StatusYes {
		t.Fatalf("Get(m1).Status=%q, want %q", got.Status, rsvprepo.StatusYes)
	}

	nYes, err := r.CountYesByTrip(context.Background(), tripID)
	if err != nil {
		t.Fatalf("CountYesByTrip() err=%v", err)
	}
	if nYes != 1 {
		t.Fatalf("CountYesByTrip()=%d, want 1", nYes)
	}

	list, err := r.ListByTrip(context.Background(), tripID)
	if err != nil {
		t.Fatalf("ListByTrip() err=%v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByTrip() len=%d, want 2", len(list))
	}
	// Ordered by memberID ascending.
	if list[0].MemberID != "m1" || list[1].MemberID != "m2" {
		t.Fatalf("ListByTrip() order=%v, want [m1 m2]", []domain.MemberID{list[0].MemberID, list[1].MemberID})
	}
}
