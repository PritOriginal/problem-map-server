package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// ExportMarksRepository is the part of the marks storage the export needs:
// a count to enforce the row cap and a streaming iterator.
type ExportMarksRepository interface {
	CountMarks(ctx context.Context, filters models.GetMarksFilters) (int, error)
	// IterateMarks calls fn for every matching mark in order; an error of fn
	// stops the iteration and is returned unchanged.
	IterateMarks(ctx context.Context, filters models.GetMarksFilters, fn func(models.Mark) error) error
}

type ExportRepositories struct {
	Marks ExportMarksRepository
}

// ErrExportTooLarge is returned when more rows match than export.max-rows
// allows; handlers map it to 400.
var ErrExportTooLarge = fmt.Errorf("%w: too many rows to export, narrow the filters", ErrInvalidArgument)

// Export streams filtered marks to an encoder without materialising them.
type Export struct {
	log   *slog.Logger
	cfg   config.ExportConfig
	repos ExportRepositories
}

func NewExport(log *slog.Logger, cfg config.ExportConfig, repos ExportRepositories) *Export {
	if cfg.MaxRows <= 0 {
		cfg.MaxRows = 50_000
	}
	return &Export{log: log, cfg: cfg, repos: repos}
}

// MaxRows is the row cap of one export.
func (uc *Export) MaxRows() int { return uc.cfg.MaxRows }

// ExportMarks validates the filters, checks the number of matching rows
// against the cap (ErrExportTooLarge) and then streams every mark to fn.
// Pagination in filters is ignored: an export is never paginated. fn is
// not called before the checks passed, so a caller may start writing the
// response body from inside fn.
func (uc *Export) ExportMarks(ctx context.Context, filters models.GetMarksFilters, fn func(models.Mark) error) error {
	const op = "usecase.Export.ExportMarks"

	filters.Pagination = models.Pagination{}
	if err := filters.Validate(); err != nil {
		return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	count, err := uc.repos.Marks.CountMarks(ctx, filters)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if count > uc.cfg.MaxRows {
		return fmt.Errorf("%s: %w (%d rows, max %d)", op, ErrExportTooLarge, count, uc.cfg.MaxRows)
	}

	// The cap is enforced by the query too: rows inserted between the count
	// and the scan never push the export past it.
	filters.Pagination.Limit = uc.cfg.MaxRows
	if err := uc.repos.Marks.IterateMarks(ctx, filters, fn); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
