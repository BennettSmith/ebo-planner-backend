package rsvprepo

import (
	"context"
	"sort"
	"sync"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/rsvprepo"
)

type key struct {
	tripID   domain.TripID
	memberID domain.MemberID
}

// Repo is an in-memory implementation of rsvprepo.Repository.
// It is safe for concurrent use.
type Repo struct {
	mu sync.RWMutex
	m  map[key]rsvprepo.RSVP
}

func NewRepo() *Repo {
	return &Repo{m: make(map[key]rsvprepo.RSVP)}
}

func (r *Repo) Get(ctx context.Context, tripID domain.TripID, memberID domain.MemberID) (rsvprepo.RSVP, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.m[key{tripID: tripID, memberID: memberID}]
	if !ok {
		return rsvprepo.RSVP{}, rsvprepo.ErrNotFound
	}
	return v, nil
}

func (r *Repo) Upsert(ctx context.Context, rec rsvprepo.RSVP) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[key{tripID: rec.TripID, memberID: rec.MemberID}] = rec
	return nil
}

func (r *Repo) ListByTrip(ctx context.Context, tripID domain.TripID) ([]rsvprepo.RSVP, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]rsvprepo.RSVP, 0)
	for k, v := range r.m {
		if k.tripID == tripID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MemberID == out[j].MemberID {
			return out[i].UpdatedAt.Before(out[j].UpdatedAt)
		}
		return string(out[i].MemberID) < string(out[j].MemberID)
	})
	return out, nil
}

func (r *Repo) CountYesByTrip(ctx context.Context, tripID domain.TripID) (int, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for k, v := range r.m {
		if k.tripID != tripID {
			continue
		}
		if v.Status == rsvprepo.StatusYes {
			n++
		}
	}
	return n, nil
}
