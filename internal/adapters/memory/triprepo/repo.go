package triprepo

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/triprepo"
)

// Repo is an in-memory implementation of triprepo.Repository.
// It is safe for concurrent use.
type Repo struct {
	mu   sync.RWMutex
	byID map[domain.TripID]triprepo.Trip
}

func NewRepo() *Repo {
	return &Repo{
		byID: make(map[domain.TripID]triprepo.Trip),
	}
}

func (r *Repo) Create(ctx context.Context, t triprepo.Trip) error {
	_ = ctx
	if t.ID == "" {
		return triprepo.ErrAlreadyExists // treat empty ID as invalid for now
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[t.ID]; ok {
		return triprepo.ErrAlreadyExists
	}
	r.byID[t.ID] = cloneTrip(t)
	return nil
}

func (r *Repo) Save(ctx context.Context, t triprepo.Trip) error {
	_ = ctx
	if t.ID == "" {
		return triprepo.ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[t.ID] = cloneTrip(t)
	return nil
}

func (r *Repo) GetByID(ctx context.Context, id domain.TripID) (triprepo.Trip, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[id]
	if !ok {
		return triprepo.Trip{}, triprepo.ErrNotFound
	}
	return cloneTrip(t), nil
}

func (r *Repo) ListPublishedAndCanceled(ctx context.Context) ([]triprepo.Trip, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]triprepo.Trip, 0)
	for _, t := range r.byID {
		if t.Status == triprepo.StatusPublished || t.Status == triprepo.StatusCanceled {
			out = append(out, cloneTrip(t))
		}
	}
	sortTrips(out)
	return out, nil
}

func (r *Repo) ListDraftsVisibleTo(ctx context.Context, caller domain.MemberID) ([]triprepo.Trip, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]triprepo.Trip, 0)
	for _, t := range r.byID {
		if t.Status != triprepo.StatusDraft {
			continue
		}
		if isDraftVisibleTo(t, caller) {
			out = append(out, cloneTrip(t))
		}
	}
	sortTrips(out)
	return out, nil
}

func cloneTrip(t triprepo.Trip) triprepo.Trip {
	cp := t
	if t.OrganizerMemberIDs != nil {
		cp.OrganizerMemberIDs = append([]domain.MemberID(nil), t.OrganizerMemberIDs...)
	}
	cp.Name = cloneStringPtr(t.Name)
	cp.Description = cloneStringPtr(t.Description)
	cp.EndDate = cloneTimePtr(t.EndDate)
	cp.CapacityRigs = cloneIntPtr(t.CapacityRigs)
	cp.AttendingRigs = cloneIntPtr(t.AttendingRigs)
	cp.DifficultyText = cloneStringPtr(t.DifficultyText)
	cp.CommsRequirementsText = cloneStringPtr(t.CommsRequirementsText)
	cp.RecommendedRequirementsText = cloneStringPtr(t.RecommendedRequirementsText)
	cp.MeetingLocation = cloneLocation(t.MeetingLocation)
	if t.Artifacts != nil {
		cp.Artifacts = append([]domain.TripArtifact(nil), t.Artifacts...)
	}
	if t.StartDate != nil {
		sd := *t.StartDate
		cp.StartDate = &sd
	}
	return cp
}

func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneTimePtr(p *time.Time) *time.Time {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneLocation(l *domain.Location) *domain.Location {
	if l == nil {
		return nil
	}
	cp := *l
	cp.Address = cloneStringPtr(l.Address)
	if l.Latitude != nil {
		v := *l.Latitude
		cp.Latitude = &v
	}
	if l.Longitude != nil {
		v := *l.Longitude
		cp.Longitude = &v
	}
	return &cp
}

func isDraftVisibleTo(t triprepo.Trip, caller domain.MemberID) bool {
	switch t.DraftVisibility {
	case triprepo.DraftVisibilityPublic:
		for _, id := range t.OrganizerMemberIDs {
			if id == caller {
				return true
			}
		}
		return false
	case triprepo.DraftVisibilityPrivate:
		return t.CreatorMemberID == caller
	default:
		return false
	}
}

func sortTrips(ts []triprepo.Trip) {
	// Sorting rule (v1): by startDate ascending; if startDate missing, place after dated trips and sort by createdAt ascending.
	sort.Slice(ts, func(i, j int) bool {
		a := ts[i]
		b := ts[j]
		ad, bd := a.StartDate, b.StartDate

		if ad != nil && bd != nil {
			if !ad.Equal(*bd) {
				return ad.Before(*bd)
			}
			// Tie-breaker: createdAt, then ID.
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.Before(b.CreatedAt)
			}
			return string(a.ID) < string(b.ID)
		}
		if ad != nil && bd == nil {
			return true
		}
		if ad == nil && bd != nil {
			return false
		}
		// Both missing startDate => createdAt, then ID.
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return string(a.ID) < string(b.ID)
	})
}
