package postgres

import (
	"errors"

	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/lib/pq"
)

// uniqueViolationCode is the PostgreSQL SQLSTATE for unique_violation.
const uniqueViolationCode = "23505"

// foreignKeyViolationCode is the PostgreSQL SQLSTATE for foreign_key_violation.
const foreignKeyViolationCode = "23503"

// isForeignKeyViolation reports whether err is a foreign-key violation, i.e.
// a referenced row does not exist.
func isForeignKeyViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == foreignKeyViolationCode
}

// wrapUniqueViolation maps a PostgreSQL unique-constraint violation to
// repository.ErrExists; any other error is returned as is.
func wrapUniqueViolation(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == uniqueViolationCode {
		return repository.ErrExists
	}
	return err
}
