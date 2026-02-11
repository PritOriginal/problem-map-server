package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

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

func (suite *MapSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.mapRepo = usecase.NewMockMapRepository(suite.T())
	suite.uc = usecase.NewMap(suite.log, usecase.MapRepositories{
		Map: suite.mapRepo,
	})
}

func TestMap(t *testing.T) {
	suite.Run(t, new(MapSuite))
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
				err:  errors.New(""),
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
				suite.NotNil(gotErr)
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
				err:  errors.New(""),
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
				suite.NotNil(gotErr)
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
				err:  errors.New(""),
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
				suite.NotNil(gotErr)
			}
			suite.mapRepo.AssertExpectations(suite.T())
		})
	}
}
