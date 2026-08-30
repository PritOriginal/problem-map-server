package usecase

import (
	"errors"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/repository"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	// ErrUnavailable is returned when a required dependency is not reachable.
	ErrUnavailable = errors.New("unavailable")
	// ErrInvalidArgument is returned when request filters or pagination are
	// out of range; handlers map it to 400.
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrTooManyRequests is returned when a per-user quota (e.g. checks per
	// day) is exhausted; handlers map it to 429.
	ErrTooManyRequests = errors.New("too many requests")
	// ErrTooManyHeatmapCells is a specific invalid-argument case: the heatmap
	// grid would exceed models.MaxHeatmapCells, the client must raise cell_m.
	ErrTooManyHeatmapCells = fmt.Errorf("%w: too many heatmap cells, increase cell_m", ErrInvalidArgument)
)

// ErrorKind classifies a usecase error for the transport layers: REST maps
// it to an HTTP status, gRPC to a status code. Keeping the classification
// here guarantees both report the same class for the same domain failure.
type ErrorKind int

const (
	// KindInternal is any error that is not a known domain failure; it is
	// logged and reported without details.
	KindInternal ErrorKind = iota
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
	KindUnavailable
	KindInvalidArgument
	KindTooManyRequests
)

// Kind returns the ErrorKind of err (errors.Is on the sentinel errors).
// A nil error is KindInternal too: callers check err != nil first.
func Kind(err error) ErrorKind {
	switch {
	case errors.Is(err, ErrNotFound):
		return KindNotFound
	case errors.Is(err, ErrConflict):
		return KindConflict
	case errors.Is(err, ErrUnauthorized):
		return KindUnauthorized
	case errors.Is(err, ErrForbidden):
		return KindForbidden
	case errors.Is(err, ErrUnavailable):
		return KindUnavailable
	case errors.Is(err, ErrInvalidArgument):
		return KindInvalidArgument
	case errors.Is(err, ErrTooManyRequests):
		return KindTooManyRequests
	default:
		return KindInternal
	}
}

// mapRepoErr translates repository errors into usecase errors so that callers
// never depend on the repository package. repository.ErrNotFound becomes
// ErrNotFound, repository.ErrExists becomes ErrConflict; anything else is
// wrapped with the operation name.
func mapRepoErr(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrNotFound):
		return fmt.Errorf("%s: %w", op, ErrNotFound)
	case errors.Is(err, repository.ErrExists):
		return fmt.Errorf("%s: %w", op, ErrConflict)
	case errors.Is(err, repository.ErrInvalidReference):
		return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}
