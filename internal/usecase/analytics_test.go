package usecase_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AnalyticsSuite struct {
	suite.Suite
	uc   *usecase.Analytics
	log  *slog.Logger
	repo *usecase.MockAnalyticsRepository
}

func (suite *AnalyticsSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.repo = usecase.NewMockAnalyticsRepository(suite.T())
	suite.uc = usecase.NewAnalytics(suite.log, usecase.AnalyticsRepositories{
		Analytics: suite.repo,
	})
}

func TestAnalytics(t *testing.T) {
	suite.Run(t, new(AnalyticsSuite))
}

var (
	day1 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
)

func (suite *AnalyticsSuite) TestGetKPI() {
	tests := []struct {
		name    string
		filters models.AnalyticsFilters
		getKPI  *method[models.KPI]
	}{
		{
			name:   "Ok",
			getKPI: &method[models.KPI]{data: models.KPI{Total: 3}},
		},
		{
			name:    "OkWithFilters",
			filters: models.AnalyticsFilters{BoundaryID: 1, MarkTypeID: 2, DateRange: models.DateRange{From: day1, To: day2}},
			getKPI:  &method[models.KPI]{data: models.KPI{Total: 1}},
		},
		{
			name:    "ErrInvalidDateRange",
			filters: models.AnalyticsFilters{DateRange: models.DateRange{From: day2, To: day1}},
		},
		{
			name:    "ErrNegativeBoundary",
			filters: models.AnalyticsFilters{BoundaryID: -1},
		},
		{
			name:   "ErrRepo",
			getKPI: &method[models.KPI]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getKPI != nil {
				suite.repo.On("GetKPI", mock.Anything, tt.filters).Once().
					Return(tt.getKPI.data, tt.getKPI.err)
			}

			got, gotErr := suite.uc.GetKPI(context.Background(), tt.filters)

			switch {
			case tt.getKPI == nil:
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
			case tt.getKPI.err != nil:
				assertRepoErr(&suite.Suite, gotErr, tt.getKPI.err)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.getKPI.data, got)
			}
		})
	}
}

func (suite *AnalyticsSuite) TestGetTimeseries() {
	tests := []struct {
		name          string
		filters       models.TimeseriesFilters
		wantStep      models.TimeseriesStep
		wantFrom      time.Time // zero: derived from "now" and the step
		wantTo        time.Time // zero: "now"
		getTimeseries *method[[]models.TimeseriesPoint]
	}{
		{
			name:          "OkDefaultsToDailyLast30Days",
			wantStep:      models.StepDay,
			getTimeseries: &method[[]models.TimeseriesPoint]{data: []models.TimeseriesPoint{}},
		},
		{
			name:          "OkDefaultsWeeklyLast12Weeks",
			filters:       models.TimeseriesFilters{Step: models.StepWeek},
			wantStep:      models.StepWeek,
			getTimeseries: &method[[]models.TimeseriesPoint]{data: []models.TimeseriesPoint{}},
		},
		{
			name:          "OkFromDerivedFromTo",
			filters:       models.TimeseriesFilters{Step: models.StepMonth, AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{To: day2}}},
			wantStep:      models.StepMonth,
			wantFrom:      day2.Add(-12 * models.StepMonth.Duration()),
			wantTo:        day2,
			getTimeseries: &method[[]models.TimeseriesPoint]{data: []models.TimeseriesPoint{}},
		},
		{
			name:          "OkExplicitRange",
			filters:       models.TimeseriesFilters{Step: models.StepDay, AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: day1, To: day2}}},
			wantStep:      models.StepDay,
			wantFrom:      day1,
			wantTo:        day2,
			getTimeseries: &method[[]models.TimeseriesPoint]{data: []models.TimeseriesPoint{{Period: day1, Created: 1}}},
		},
		{
			name:    "ErrInvalidStep",
			filters: models.TimeseriesFilters{Step: "hour"},
		},
		{
			name:    "ErrToBeforeFrom",
			filters: models.TimeseriesFilters{AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: day2, To: day1}}},
		},
		{
			name:    "ErrTooManyPeriods",
			filters: models.TimeseriesFilters{Step: models.StepDay, AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: day1, To: day1.AddDate(0, 0, models.MaxTimeseriesPeriods+1)}}},
		},
		{
			name:          "ErrRepo",
			filters:       models.TimeseriesFilters{AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: day1, To: day2}}},
			wantStep:      models.StepDay,
			wantFrom:      day1,
			wantTo:        day2,
			getTimeseries: &method[[]models.TimeseriesPoint]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			var got models.TimeseriesFilters
			if tt.getTimeseries != nil {
				suite.repo.On("GetTimeseries", mock.Anything, mock.MatchedBy(func(f models.TimeseriesFilters) bool {
					got = f
					return true
				})).Once().Return(tt.getTimeseries.data, tt.getTimeseries.err)
			}

			before := time.Now()
			points, gotErr := suite.uc.GetTimeseries(context.Background(), tt.filters)
			after := time.Now()

			switch {
			case tt.getTimeseries == nil:
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
				return
			case tt.getTimeseries.err != nil:
				assertRepoErr(&suite.Suite, gotErr, tt.getTimeseries.err)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.getTimeseries.data, points)
			}

			suite.Equal(tt.wantStep, got.Step)
			suite.Equal(tt.filters.BoundaryID, got.BoundaryID)
			if tt.wantTo.IsZero() {
				suite.False(got.To.Before(before) || got.To.After(after), "to must default to now")
			} else {
				suite.Equal(tt.wantTo, got.To)
			}
			wantFrom := tt.wantFrom
			if wantFrom.IsZero() {
				wantFrom = got.To.Add(-time.Duration(usecase.DefaultTimeseriesPeriods(tt.wantStep)) * tt.wantStep.Duration())
			}
			suite.Equal(wantFrom, got.From)
		})
	}
}

func (suite *AnalyticsSuite) TestGetTopTypes() {
	tests := []struct {
		name        string
		filters     models.TopTypesFilters
		wantLimit   int
		getTopTypes *method[[]models.TopType]
	}{
		{
			name:        "OkDefaultLimit",
			wantLimit:   models.DefaultTopTypesLimit,
			getTopTypes: &method[[]models.TopType]{data: []models.TopType{{MarkTypeID: 1, Count: 2, Share: 1}}},
		},
		{
			name:        "OkExplicitLimit",
			filters:     models.TopTypesFilters{BoundaryID: 3, Limit: 5, DateRange: models.DateRange{From: day1}},
			wantLimit:   5,
			getTopTypes: &method[[]models.TopType]{data: []models.TopType{}},
		},
		{
			name:    "ErrLimitTooBig",
			filters: models.TopTypesFilters{Limit: models.MaxTopTypesLimit + 1},
		},
		{
			name:    "ErrNegativeLimit",
			filters: models.TopTypesFilters{Limit: -1},
		},
		{
			name:    "ErrToBeforeFrom",
			filters: models.TopTypesFilters{DateRange: models.DateRange{From: day2, To: day1}},
		},
		{
			name:        "ErrRepo",
			wantLimit:   models.DefaultTopTypesLimit,
			getTopTypes: &method[[]models.TopType]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getTopTypes != nil {
				want := tt.filters
				want.Limit = tt.wantLimit
				suite.repo.On("GetTopTypes", mock.Anything, want).Once().
					Return(tt.getTopTypes.data, tt.getTopTypes.err)
			}

			got, gotErr := suite.uc.GetTopTypes(context.Background(), tt.filters)

			switch {
			case tt.getTopTypes == nil:
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
			case tt.getTopTypes.err != nil:
				assertRepoErr(&suite.Suite, gotErr, tt.getTopTypes.err)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.getTopTypes.data, got)
			}
		})
	}
}

func (suite *AnalyticsSuite) TestGetOpenStats() {
	tests := []struct {
		name       string
		boundaryID int
		repo       *method[models.OpenStats]
	}{
		{name: "Ok", repo: &method[models.OpenStats]{data: models.OpenStats{MarksTotal: 3}}},
		{name: "OkBoundary", boundaryID: 1, repo: &method[models.OpenStats]{data: models.OpenStats{MarksTotal: 1}}},
		{name: "ErrNegativeBoundary", boundaryID: -1},
		{name: "ErrRepo", repo: &method[models.OpenStats]{err: errRepo}},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.repo != nil {
				suite.repo.On("GetOpenStats", mock.Anything, tt.boundaryID).Once().
					Return(tt.repo.data, tt.repo.err)
			}

			got, gotErr := suite.uc.GetOpenStats(context.Background(), tt.boundaryID)

			switch {
			case tt.repo == nil:
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
			case tt.repo.err != nil:
				suite.ErrorIs(gotErr, tt.repo.err)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.repo.data, got)
			}
		})
	}
}
