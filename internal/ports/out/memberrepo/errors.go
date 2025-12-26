package memberrepo

import "errors"

var (
	// ErrNotFound indicates the requested member does not exist.
	ErrNotFound = errors.New("member not found")

	// ErrSubjectAlreadyBound indicates a member already exists for the provided subject.
	ErrSubjectAlreadyBound = errors.New("member subject already bound")

	// ErrAlreadyExists indicates a member already exists with the provided ID.
	ErrAlreadyExists = errors.New("member already exists")
)
