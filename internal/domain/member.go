package domain

import "time"

// VehicleProfile is optional informational metadata about a member's rig/setup.
// All fields are optional.
type VehicleProfile struct {
	Make             *string
	Model            *string
	TireSize         *string
	LiftLockers      *string
	FuelRange        *string
	RecoveryGear     *string
	HamRadioCallSign *string
	Notes            *string
}

// Member is the domain representation of a member profile.
type Member struct {
	ID      MemberID
	Subject SubjectID

	DisplayName     string
	Email           string
	GroupAliasEmail *string
	VehicleProfile  *VehicleProfile

	IsActive bool

	CreatedAt time.Time
	UpdatedAt time.Time
}
