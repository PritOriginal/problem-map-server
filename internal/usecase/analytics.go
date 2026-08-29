package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type AnalyticsRepository interface {
	GetKPI(ctx context.Context, filters models.AnalyticsFilters) (models.KPI, error)
	GetTimeseries(ctx context.Context, filters models.TimeseriesFilters) ([]models.TimeseriesPoint, error)
	GetTopTypes(ctx context.Context, filters models.TopTypesFilters) ([]models.TopType, error)
}

type Analytics struct {
	log   *slog.Logger
	repos AnalyticsRepositories
	// now is injectable for deterministic defaults in tests.
	now func() time.Time
}

type AnalyticsRepositories struct {
	Analytics AnalyticsRepository
}

func NewAnalytics(log *slog.Logger, repos AnalyticsRepositories) *Analytics {
	return &Analytics{log: log, repos: repos, now: time.Now}
}

// DefaultTimeseriesPeriods is the number of buckets returned when the
// client gives no range: the last 30 days / 12 weeks / 12 months.
func DefaultTimeseriesPeriods(step models.TimeseriesStep) int {
	switch step {
	case models.StepWeek, models.StepMonth:
		return 12
	default:
		return 30
	}
}

func (uc *Analytics) GetKPI(ctx context.Context, filters models.AnalyticsFilters) (models.KPI, error) {
	const op = "usecase.Analytics.GetKPI"

	if err := filters.Validate(); err != nil {
		return models.KPI{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	kpi, err := uc.repos.Analytics.GetKPI(ctx, filters)
	if err != nil {
		return models.KPI{}, mapRepoErr(op, err)
	}
	return kpi, nil
}

// GetTimeseries fills in the defaults (step=day, to=now, from=to minus
// DefaultTimeseriesPeriods buckets) before validating.
func (uc *Analytics) GetTimeseries(ctx context.Context, filters models.TimeseriesFilters) ([]models.TimeseriesPoint, error) {
	const op = "usecase.Analytics.GetTimeseries"

	if filters.Step == "" {
		filters.Step = models.StepDay
	}
	if err := filters.Step.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if filters.To.IsZero() {
		filters.To = uc.now()
	}
	if filters.From.IsZero() {
		filters.From = filters.To.Add(-time.Duration(DefaultTimeseriesPeriods(filters.Step)) * filters.Step.Duration())
	}
	if err := filters.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	points, err := uc.repos.Analytics.GetTimeseries(ctx, filters)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return points, nil
}

func (uc *Analytics) GetTopTypes(ctx context.Context, filters models.TopTypesFilters) ([]models.TopType, error) {
	const op = "usecase.Analytics.GetTopTypes"

	if err := filters.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if filters.Limit == 0 {
		filters.Limit = models.DefaultTopTypesLimit
	}

	types, err := uc.repos.Analytics.GetTopTypes(ctx, filters)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return types, nil
}
