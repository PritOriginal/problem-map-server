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
	// ErrTooManyHeatmapCells is a specific invalid-argument case: the heatmap
	// grid would exceed models.MaxHeatmapCells, the client must raise cell_m.
	ErrTooManyHeatmapCells = fmt.Errorf("%w: too many heatmap cells, increase cell_m", ErrInvalidArgument)
)

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
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}
