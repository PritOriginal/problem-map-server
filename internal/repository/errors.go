package repository

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("exists")
)
