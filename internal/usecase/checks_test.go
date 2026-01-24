package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type ChecksSuite struct {
	suite.Suite
	uc         *usecase.Checks
	log        *slog.Logger
	checksRepo *usecase.MockChecksRepository
	photosRepo *usecase.MockPhotosRepository
}

func (suite *ChecksSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.photosRepo = usecase.NewMockPhotosRepository(suite.T())
	suite.uc = usecase.NewChecks(suite.log, suite.checksRepo, suite.photosRepo)
}

func TestChecks(t *testing.T) {
	suite.Run(t, new(ChecksSuite))
}

func (suite *ChecksSuite) TestAddCheck() {
	tests := []struct {
		name             string
		addCheckWantErr  bool
		addPhotosWantErr bool
	}{
		{
			name:             "Ok",
			addCheckWantErr:  false,
			addPhotosWantErr: false,
		},
		{
			name:             "ErrAddCheck",
			addCheckWantErr:  true,
			addPhotosWantErr: false,
		},
		{
			name:             "ErrAddPhotos",
			addCheckWantErr:  false,
			addPhotosWantErr: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			checksRepoCall := suite.checksRepo.On("AddCheck", mock.Anything, mock.Anything).Once()
			if !tt.addCheckWantErr {
				checksRepoCall.Return(int64(1), nil)

				photosRepoCall := suite.photosRepo.On("AddPhotos", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int"), mock.Anything).Once()
				if !tt.addPhotosWantErr {
					photosRepoCall.Return(nil)
				} else {
					photosRepoCall.Return(errors.New(""))
				}
			} else {
				checksRepoCall.Return(int64(0), errors.New(""))
			}

			_, gotErr := suite.uc.AddCheck(context.Background(), models.Check{}, []io.Reader{})

			if !tt.addCheckWantErr && !tt.addPhotosWantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *ChecksSuite) TestGetCheckById() {
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
			checksRepoCall := suite.checksRepo.On("GetCheckById", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				checksRepoCall.Return(models.Check{}, nil)
			} else {
				checksRepoCall.Return(models.Check{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetCheckById(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByMarkId() {
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
			checksRepoCall := suite.checksRepo.On("GetChecksByMarkId", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				checksRepoCall.Return([]models.Check{}, nil)
			} else {
				checksRepoCall.Return([]models.Check{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetChecksByMarkId(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByUserId() {
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
			checksRepoCall := suite.checksRepo.On("GetChecksByUserId", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				checksRepoCall.Return([]models.Check{}, nil)
			} else {
				checksRepoCall.Return([]models.Check{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetChecksByUserId(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
