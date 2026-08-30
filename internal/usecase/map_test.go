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

type MapSuite struct {
	suite.Suite
	uc      *usecase.Map
	log     *slog.Logger
	mapRepo *usecase.MockMapRepository
}

func (suite *MapSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.mapRepo = usecase.NewMockMapRepository(suite.T())
	suite.uc = usecase.NewMap(suite.log, usecase.MapRepositories{
		Map: suite.mapRepo,
	})
}

func TestMap(t *testing.T) {
	suite.Run(t, new(MapSuite))
}

func (suite *MapSuite) TestGetAdminBoundariesMarksCount_Validation() {
	from := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := suite.uc.GetAdminBoundariesMarksCount(context.Background(), models.GetAdminBoundaryMarksCountFilters{
		DateRange: models.DateRange{From: from, To: to},
	})

	suite.ErrorIs(err, usecase.ErrInvalidArgument)
	suite.mapRepo.AssertNotCalled(suite.T(), "GetAdminBoundariesMarksCount", mock.Anything, mock.Anything)
}

func (suite *MapSuite) TestGetHeatmap() {
	bbox := models.BBox{MinLon: 41.39, MinLat: 52.69, MaxLon: 41.42, MaxLat: 52.71}

	tests := []struct {
		name       string
		filters    models.HeatmapFilters
		wantCellM  float64
		getHeatmap *method[[]models.HeatmapCell]
		wantErr    error
	}{
		{
			name:       "OkDefaultCell",
			filters:    models.HeatmapFilters{BBox: bbox},
			wantCellM:  models.DefaultHeatmapCellM,
			getHeatmap: &method[[]models.HeatmapCell]{data: []models.HeatmapCell{{Count: 2}}},
		},
		{
			name:       "OkExplicitCell",
			filters:    models.HeatmapFilters{BBox: bbox, CellM: 100, MarkTypeIds: []int{1}, MarkStatusIds: []int{2}},
			wantCellM:  100,
			getHeatmap: &method[[]models.HeatmapCell]{data: []models.HeatmapCell{}},
		},
		{
			name:    "ErrInvalidBBox",
			filters: models.HeatmapFilters{BBox: models.BBox{MinLon: 2, MinLat: 2, MaxLon: 1, MaxLat: 1}},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrCellTooSmall",
			filters: models.HeatmapFilters{BBox: bbox, CellM: models.MinHeatmapCellM - 1},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrCellTooBig",
			filters: models.HeatmapFilters{BBox: bbox, CellM: models.MaxHeatmapCellM + 1},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrTooManyCells",
			filters: models.HeatmapFilters{BBox: models.BBox{MinLon: 30, MinLat: 50, MaxLon: 40, MaxLat: 60}, CellM: 250},
			wantErr: usecase.ErrTooManyHeatmapCells,
		},
		{
			name:       "ErrRepo",
			filters:    models.HeatmapFilters{BBox: bbox, CellM: 500},
			wantCellM:  500,
			getHeatmap: &method[[]models.HeatmapCell]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getHeatmap != nil {
				want := tt.filters
				want.CellM = tt.wantCellM
				suite.mapRepo.On("GetHeatmap", mock.Anything, want).Once().
					Return(tt.getHeatmap.data, tt.getHeatmap.err)
			}

			got, gotErr := suite.uc.GetHeatmap(context.Background(), tt.filters)

			switch {
			case tt.wantErr != nil:
				suite.ErrorIs(gotErr, tt.wantErr)
				// Every heatmap validation failure is a 400 for the client.
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
			case tt.getHeatmap.err != nil:
				assertRepoErr(&suite.Suite, gotErr, tt.getHeatmap.err)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.getHeatmap.data, got)
			}
		})
	}
}

func (suite *MapSuite) TestGetAdminBoundaries() {
	tests := []struct {
		name               string
		getAdminBoundaries method[[]models.AdminBoundary]
	}{
		{
			name: "Ok",
			getAdminBoundaries: method[[]models.AdminBoundary]{
				data: []models.AdminBoundary{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getAdminBoundaries: method[[]models.AdminBoundary]{
				data: nil,
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.mapRepo.On("GetAdminBoundaries", mock.Anything, mock.Anything).Once().
					Return(tt.getAdminBoundaries.data, tt.getAdminBoundaries.err)
				if tt.getAdminBoundaries.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetAdminBoundaries(context.Background(), models.GetAdminBoundaryFilters{})

			if tt.getAdminBoundaries.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getAdminBoundaries.err)
			}
			suite.mapRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MapSuite) TestGetAdminBoundariesMarksCount() {
	tests := []struct {
		name                         string
		getAdminBoundariesMarksCount method[[]models.AdminBoundaryMarksCount]
	}{
		{
			name: "Ok",
			getAdminBoundariesMarksCount: method[[]models.AdminBoundaryMarksCount]{
				data: []models.AdminBoundaryMarksCount{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getAdminBoundariesMarksCount: method[[]models.AdminBoundaryMarksCount]{
				data: nil,
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.mapRepo.On("GetAdminBoundariesMarksCount", mock.Anything, mock.Anything).Once().
					Return(tt.getAdminBoundariesMarksCount.data, tt.getAdminBoundariesMarksCount.err)
				if tt.getAdminBoundariesMarksCount.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetAdminBoundariesMarksCount(context.Background(), models.GetAdminBoundaryMarksCountFilters{})

			if tt.getAdminBoundariesMarksCount.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getAdminBoundariesMarksCount.err)
			}
			suite.mapRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MapSuite) TestGetRegions() {
	tests := []struct {
		name       string
		getRegions method[[]models.Region]
	}{
		{
			name: "Ok",
			getRegions: method[[]models.Region]{
				data: []models.Region{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getRegions: method[[]models.Region]{
				data: nil,
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.mapRepo.On("GetRegions", mock.Anything).Once().
					Return(tt.getRegions.data, tt.getRegions.err)
				if tt.getRegions.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetRegions(context.Background())

			if tt.getRegions.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getRegions.err)
			}
			suite.mapRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MapSuite) TestGetCities() {
	tests := []struct {
		name      string
		getCities method[[]models.City]
	}{
		{
			name: "Ok",
			getCities: method[[]models.City]{
				data: []models.City{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getCities: method[[]models.City]{
				data: nil,
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.mapRepo.On("GetCities", mock.Anything).Once().
					Return(tt.getCities.data, tt.getCities.err)
				if tt.getCities.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetCities(context.Background())

			if tt.getCities.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getCities.err)
			}
			suite.mapRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MapSuite) TestGetDistricts() {
	tests := []struct {
		name         string
		getDistricts method[[]models.District]
	}{
		{
			name: "Ok",
			getDistricts: method[[]models.District]{
				data: []models.District{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getDistricts: method[[]models.District]{
				data: nil,
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.mapRepo.On("GetDistricts", mock.Anything).Once().
					Return(tt.getDistricts.data, tt.getDistricts.err)
				if tt.getDistricts.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetDistricts(context.Background())

			if tt.getDistricts.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getDistricts.err)
			}
			suite.mapRepo.AssertExpectations(suite.T())
		})
	}
}
