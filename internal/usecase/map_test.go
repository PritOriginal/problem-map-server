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
	suite.uc = usecase.NewMap(suite.log, suite.mapRepo)
}

func TestMap(t *testing.T) {
	suite.Run(t, new(MapSuite))
}

func (suite *MapSuite) TestGetRegions() {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "Ok",
			wantErr: false,
		},
		{
			name:    "Err",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			marksRepoCall := suite.mapRepo.On("GetRegions", mock.Anything).Once()
			if !tt.wantErr {
				marksRepoCall.Return([]models.Region{}, nil)
			} else {
				marksRepoCall.Return([]models.Region{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetRegions(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *MapSuite) TestGetCities() {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "Ok",
			wantErr: false,
		},
		{
			name:    "Err",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			marksRepoCall := suite.mapRepo.On("GetCities", mock.Anything).Once()
			if !tt.wantErr {
				marksRepoCall.Return([]models.City{}, nil)
			} else {
				marksRepoCall.Return([]models.City{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetCities(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *MapSuite) TestGetDistricts() {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "Ok",
			wantErr: false,
		},
		{
			name:    "Err",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			marksRepoCall := suite.mapRepo.On("GetDistricts", mock.Anything).Once()
			if !tt.wantErr {
				marksRepoCall.Return([]models.District{}, nil)
			} else {
				marksRepoCall.Return([]models.District{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetDistricts(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
