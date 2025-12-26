package trips

import (
	"time"

	"ebo-planner-backend/internal/domain"
)

// Optional is a tri-state field used to distinguish:
// - unspecified (omitted)
// - specified as null
// - specified with a value
type Optional[T any] struct {
	specified bool
	isNull    bool
	value     T
}

func Unspecified[T any]() Optional[T] { return Optional[T]{} }
func Null[T any]() Optional[T]        { return Optional[T]{specified: true, isNull: true} }
func Some[T any](v T) Optional[T]     { return Optional[T]{specified: true, value: v} }

func (o Optional[T]) IsSpecified() bool { return o.specified }
func (o Optional[T]) IsNull() bool      { return o.specified && o.isNull }
func (o Optional[T]) Value() T          { return o.value }

type CreateTripDraftInput struct {
	Name string
}

// TripCreated is the minimal response returned when a draft trip is created.
type TripCreated struct {
	ID              domain.TripID
	Status          domain.TripStatus
	DraftVisibility domain.DraftVisibility
}

type LocationPatch struct {
	Label            Optional[string]
	Address          Optional[string]
	Latitude         Optional[float64]
	Longitude        Optional[float64]
	ClearCoordinates bool // when true, clear both latitude+longitude
}

type UpdateTripInput struct {
	// Name is optional and cannot be null.
	Name Optional[string]

	Description  Optional[string]
	StartDate    Optional[time.Time]
	EndDate      Optional[time.Time]
	CapacityRigs Optional[int]

	DifficultyText              Optional[string]
	MeetingLocation             Optional[*LocationPatch] // null clears the location
	CommsRequirementsText       Optional[string]
	RecommendedRequirementsText Optional[string]

	ArtifactIDs Optional[[]string] // null clears all artifacts; value reorders existing artifacts by ID
}
