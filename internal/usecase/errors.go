package usecase

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	// ErrUnavailable is returned when a required dependency is not reachable.
	ErrUnavailable = errors.New("unavailable")
	// ErrInvalidArgument is returned when request filters or pagination are
	// out of range; handlers map it to 400.
	ErrInvalidArgument = errors.New("invalid argument")
)
