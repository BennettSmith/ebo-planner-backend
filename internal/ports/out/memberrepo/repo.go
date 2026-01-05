package memberrepo

import (
	"context"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
)

// Member is the persistence shape used by the member repository.
//
// Note: This is intentionally minimal and will likely evolve as we implement
// domain models and use-cases (Milestones 3+). It's used as an internal record,
// not an HTTP DTO.
type Member struct {
	ID      domain.MemberID
	Subject domain.SubjectID
	// DisplayName is the member's preferred display name.
	DisplayName string
	// Email is stored for the member profile, but is not safe to return in directory/search.
	Email string
	// GroupAliasEmail is an optional email address used for group aliasing; nil means unset.
	GroupAliasEmail *string
	// VehicleProfile is optional informational member metadata; nil means unset.
	VehicleProfile *domain.VehicleProfile

	IsActive bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository provides access to persisted members.
//
// Result ordering expectations:
// - List/Search methods should return results ordered by DisplayName ascending to keep behavior deterministic.
type Repository interface {
	Create(ctx context.Context, m Member) error
	Update(ctx context.Context, m Member) error

	GetByID(ctx context.Context, id domain.MemberID) (Member, error)
	GetBySubject(ctx context.Context, subject domain.SubjectID) (Member, error)

	List(ctx context.Context, includeInactive bool) ([]Member, error)

	// SearchActiveByDisplayName searches active members by a tokenized, case-insensitive match on DisplayName.
	// The query validation (e.g. minimum length) is enforced at the application layer.
	SearchActiveByDisplayName(ctx context.Context, query string, limit int) ([]Member, error)
}
