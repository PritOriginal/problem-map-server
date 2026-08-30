package repository

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("exists")
	// ErrInvalidReference is returned when a written row points at a
	// referenced entity (e.g. a mark type) that does not exist.
	ErrInvalidReference = errors.New("invalid reference")
)
