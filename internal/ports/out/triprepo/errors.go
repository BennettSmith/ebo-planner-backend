package triprepo

import "errors"

var (
	ErrNotFound      = errors.New("trip not found")
	ErrAlreadyExists = errors.New("trip already exists")
)
