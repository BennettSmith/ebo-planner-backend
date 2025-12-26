package members

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

type VehicleProfilePatch struct {
	Make             Optional[string]
	Model            Optional[string]
	TireSize         Optional[string]
	LiftLockers      Optional[string]
	FuelRange        Optional[string]
	RecoveryGear     Optional[string]
	HamRadioCallSign Optional[string]
	Notes            Optional[string]
}

type UpdateMyMemberProfileInput struct {
	DisplayName     Optional[string]
	Email           Optional[string] // cannot be null
	GroupAliasEmail Optional[string] // may be null
	VehicleProfile  Optional[VehicleProfilePatch]
}

type CreateMyMemberInput struct {
	DisplayName     string
	Email           string
	GroupAliasEmail *string
	VehicleProfile  *VehicleProfilePatch // treated as a full object on create
}
