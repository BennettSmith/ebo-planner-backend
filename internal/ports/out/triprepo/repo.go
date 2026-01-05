package triprepo

import (
	"context"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
)

type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusPublished Status = "PUBLISHED"
	StatusCanceled  Status = "CANCELED"
)

type DraftVisibility string

const (
	DraftVisibilityPrivate DraftVisibility = "PRIVATE"
	DraftVisibilityPublic  DraftVisibility = "PUBLIC"
)

// Trip is the persistence shape used by the trip repository.
// It is not an HTTP DTO.
type Trip struct {
	ID domain.TripID

	Status Status

	// Core planning fields (nullable in v1 responses).
	Name        *string
	Description *string

	CreatorMemberID    domain.MemberID
	OrganizerMemberIDs []domain.MemberID

	DraftVisibility DraftVisibility

	// StartDate is used for sorting; nil means "unknown".
	StartDate *time.Time
	EndDate   *time.Time

	CapacityRigs *int
	// AttendingRigs is a read model field populated for published trips (later milestones).
	AttendingRigs *int

	DifficultyText              *string
	MeetingLocation             *domain.Location
	CommsRequirementsText       *string
	RecommendedRequirementsText *string

	Artifacts []domain.TripArtifact

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository provides access to persisted trips.
//
// Result ordering expectations:
// - List methods should return results deterministically ordered (see Milestone 4 sorting rules).
type Repository interface {
	Create(ctx context.Context, t Trip) error
	Save(ctx context.Context, t Trip) error

	GetByID(ctx context.Context, id domain.TripID) (Trip, error)

	// ListPublishedAndCanceled returns trips with status in (PUBLISHED, CANCELED).
	ListPublishedAndCanceled(ctx context.Context) ([]Trip, error)

	// ListDraftsVisibleTo returns draft trips visible to the caller using v1 visibility rules:
	// - PUBLIC drafts are visible to organizers (caller must be in OrganizerMemberIDs)
	// - PRIVATE drafts are visible only to the creator (caller must equal CreatorMemberID)
	ListDraftsVisibleTo(ctx context.Context, caller domain.MemberID) ([]Trip, error)
}
