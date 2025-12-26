package domain

import "time"

type TripStatus string

const (
	TripStatusDraft     TripStatus = "DRAFT"
	TripStatusPublished TripStatus = "PUBLISHED"
	TripStatusCanceled  TripStatus = "CANCELED"
)

type DraftVisibility string

const (
	DraftVisibilityPrivate DraftVisibility = "PRIVATE"
	DraftVisibilityPublic  DraftVisibility = "PUBLIC"
)

type ArtifactType string

const (
	ArtifactTypeGPX      ArtifactType = "GPX"
	ArtifactTypeSchedule ArtifactType = "SCHEDULE"
	ArtifactTypeDocument ArtifactType = "DOCUMENT"
	ArtifactTypeOther    ArtifactType = "OTHER"
)

type Location struct {
	Label   string
	Address *string

	Latitude  *float64
	Longitude *float64
}

type TripArtifact struct {
	ArtifactID string
	Type       ArtifactType
	Title      string
	URL        string
}

type TripSummary struct {
	ID              TripID
	Name            *string
	StartDate       *time.Time // date-only semantics at the edges
	EndDate         *time.Time // date-only semantics at the edges
	Status          TripStatus
	DraftVisibility *DraftVisibility

	CapacityRigs  *int
	AttendingRigs *int
}

type MemberSummary struct {
	ID              MemberID
	DisplayName     string
	Email           string
	GroupAliasEmail *string
}

// TripDetails is the domain read model used by Milestone 4 endpoints.
type TripDetails struct {
	TripSummary

	Description                 *string
	DifficultyText              *string
	MeetingLocation             *Location
	CommsRequirementsText       *string
	RecommendedRequirementsText *string

	Organizers []MemberSummary
	Artifacts  []TripArtifact

	// RSVP-related fields are introduced in later milestones; nil means "omitted".
	RSVPSummary        *TripRSVPSummary
	MyRSVP             *MyRSVP
	RSVPActionsEnabled bool
}

type TripRSVPSummary struct {
	CapacityRigs *int

	AttendingRigs int

	AttendingMembers    []MemberSummary
	NotAttendingMembers []MemberSummary
}

type RSVPResponse string

const (
	RSVPResponseYes   RSVPResponse = "YES"
	RSVPResponseNo    RSVPResponse = "NO"
	RSVPResponseUnset RSVPResponse = "UNSET"
)

type MyRSVP struct {
	TripID    TripID
	MemberID  MemberID
	Response  RSVPResponse
	UpdatedAt time.Time
}
