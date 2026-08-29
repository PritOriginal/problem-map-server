package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/lib/pq"
)

// PostgreSQL SQLSTATE codes mapped to repository errors.
const (
	uniqueViolationCode     = "23505"
	foreignKeyViolationCode = "23503"
)

// wrapPgError translates a database error into the repository vocabulary
// and prefixes it with op: sql.ErrNoRows becomes repository.ErrNotFound, a
// unique violation repository.ErrExists and a foreign-key violation (the
// written row points at a missing user, mark, type...)
// repository.ErrInvalidReference. Any other error is wrapped as is. A nil
// error stays nil.
func wrapPgError(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%s: %w", op, repository.ErrNotFound)
	case isPgCode(err, uniqueViolationCode):
		return fmt.Errorf("%s: %w", op, repository.ErrExists)
	case isPgCode(err, foreignKeyViolationCode):
		return fmt.Errorf("%s: %w: %w", op, repository.ErrInvalidReference, err)
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

// isForeignKeyViolation reports whether err is a foreign-key violation, i.e.
// a referenced row does not exist.
func isForeignKeyViolation(err error) bool {
	return isPgCode(err, foreignKeyViolationCode)
}

func isPgCode(err error, code pq.ErrorCode) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == code
}
