package storage

import "errors"

var (
	ErrNotFound = errors.New("Not found")
	ErrExists   = errors.New("Exists")
)
