package models_test

import (
	"errors"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/stretchr/testify/suite"
)

type ModelsSuite struct {
	suite.Suite
}

func TestModels(t *testing.T) {
	suite.Run(t, new(ModelsSuite))
}

// errAny marks a case that must fail without a specific sentinel.
var errAny = errors.New("any error")

func (suite *ModelsSuite) TestTimeseriesStep_Validate() {
	suite.NoError(models.StepDay.Validate())
	suite.NoError(models.StepWeek.Validate())
	suite.NoError(models.StepMonth.Validate())
	suite.Error(models.TimeseriesStep("").Validate())
	suite.Error(models.TimeseriesStep("hour").Validate())
}

func (suite *ModelsSuite) TestTimeseriesFilters_Validate() {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		filters models.TimeseriesFilters
		wantErr bool
	}{
		{name: "ok", filters: models.TimeseriesFilters{Step: models.StepDay, AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: from, To: from.AddDate(0, 0, 30)}}}},
		{name: "missing range", filters: models.TimeseriesFilters{Step: models.StepDay}, wantErr: true},
		{name: "to before from", filters: models.TimeseriesFilters{Step: models.StepDay, AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: from, To: from.AddDate(0, 0, -1)}}}, wantErr: true},
		{name: "too many days", filters: models.TimeseriesFilters{Step: models.StepDay, AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: from, To: from.AddDate(0, 0, models.MaxTimeseriesPeriods+1)}}}, wantErr: true},
		{name: "same span in months is fine", filters: models.TimeseriesFilters{Step: models.StepMonth, AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: from, To: from.AddDate(0, 0, models.MaxTimeseriesPeriods+1)}}}},
		{name: "negative boundary", filters: models.TimeseriesFilters{Step: models.StepDay, AnalyticsFilters: models.AnalyticsFilters{BoundaryID: -1, DateRange: models.DateRange{From: from, To: from.AddDate(0, 0, 1)}}}, wantErr: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := tt.filters.Validate()
			if tt.wantErr {
				suite.Error(err)
			} else {
				suite.NoError(err)
			}
		})
	}
}

func (suite *ModelsSuite) TestHeatmapFilters_Validate() {
	city := models.BBox{MinLon: 41.39, MinLat: 52.69, MaxLon: 41.42, MaxLat: 52.71}
	country := models.BBox{MinLon: 30, MinLat: 50, MaxLon: 40, MaxLat: 60}

	tests := []struct {
		name    string
		filters models.HeatmapFilters
		wantErr error
	}{
		{name: "city at 250 m", filters: models.HeatmapFilters{BBox: city, CellM: 250}},
		{name: "city at 10 m is too fine", filters: models.HeatmapFilters{BBox: city, CellM: 10}, wantErr: models.ErrTooManyHeatmapCells},
		{name: "country at 250 m is too fine", filters: models.HeatmapFilters{BBox: country, CellM: 250}, wantErr: models.ErrTooManyHeatmapCells},
		{name: "country at 50 km", filters: models.HeatmapFilters{BBox: country, CellM: 50_000}},
		{name: "cell below minimum", filters: models.HeatmapFilters{BBox: city, CellM: 5}, wantErr: errAny},
		{name: "cell above maximum", filters: models.HeatmapFilters{BBox: city, CellM: models.MaxHeatmapCellM + 1}, wantErr: errAny},
		{name: "invalid bbox", filters: models.HeatmapFilters{BBox: models.BBox{MinLon: 1, MinLat: 1, MaxLon: 0, MaxLat: 0}, CellM: 250}, wantErr: models.ErrInvalidBBox},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := tt.filters.Validate()
			switch {
			case tt.wantErr == nil:
				suite.NoError(err)
			case errors.Is(tt.wantErr, errAny):
				suite.Error(err)
			default:
				suite.ErrorIs(err, tt.wantErr)
			}
		})
	}
}

func (suite *ModelsSuite) TestHeatmapFilters_EstimateCells() {
	// A 1.5 km x 1.7 km square in EPSG:3857 at the equator (where the
	// projection is metric): hexagons of size 100 m tile it in columns every
	// 150 m and rows every ~173 m, so roughly 11 x 11 cells plus the edges.
	bbox := models.BBox{MinLon: 0, MinLat: 0, MaxLon: 1500.0 / 111_319.49, MaxLat: 1700.0 / 111_319.49}
	got := models.HeatmapFilters{BBox: bbox, CellM: 100}.EstimateCells()
	suite.InDelta(12*12, got, 24)

	// Doubling the cell size divides the estimate by about four.
	coarse := models.HeatmapFilters{BBox: bbox, CellM: 200}.EstimateCells()
	suite.Less(coarse, got/2)
}

func (suite *ModelsSuite) TestTopTypesFilters_Validate() {
	suite.NoError(models.TopTypesFilters{}.Validate())
	suite.NoError(models.TopTypesFilters{Limit: models.MaxTopTypesLimit}.Validate())
	suite.Error(models.TopTypesFilters{Limit: models.MaxTopTypesLimit + 1}.Validate())
	suite.Error(models.TopTypesFilters{Limit: -1}.Validate())
	suite.Error(models.TopTypesFilters{BoundaryID: -1}.Validate())
	now := time.Now()
	suite.ErrorIs(models.TopTypesFilters{DateRange: models.DateRange{From: now, To: now.Add(-time.Hour)}}.Validate(), models.ErrInvalidDateRange)
}
