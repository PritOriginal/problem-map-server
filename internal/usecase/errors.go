package usecase

import "errors"

var (
	ErrNotFound     = errors.New("Not found")
	ErrConflict     = errors.New("Conflict")
	ErrUnauthorized = errors.New("Unauthorized")
)
