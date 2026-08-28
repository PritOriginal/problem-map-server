package postgres

import (
	"errors"

	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/lib/pq"
)

// uniqueViolationCode is the PostgreSQL SQLSTATE for unique_violation.
const uniqueViolationCode = "23505"

// wrapUniqueViolation maps a PostgreSQL unique-constraint violation to
// repository.ErrExists; any other error is returned as is.
func wrapUniqueViolation(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == uniqueViolationCode {
		return repository.ErrExists
	}
	return err
}
