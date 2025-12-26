package domain

// SubjectID is the authenticated subject extracted from JWT claims (typically "sub").
// We model it as an opaque identifier: its format is controlled by the IdP.
type SubjectID string

// MemberID is an internal identifier for a member record.
type MemberID string

// TripID is an internal identifier for a trip record.
type TripID string
