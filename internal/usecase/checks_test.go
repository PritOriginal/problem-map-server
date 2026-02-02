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
		name                  string
		errGetCheckById       error
		errGetPhotosByCheckId error
	}{
		{
			name:                  "Ok",
			errGetCheckById:       nil,
			errGetPhotosByCheckId: nil,
		},
		{
			name:                  "ErrGetCheckById",
			errGetCheckById:       errors.New(""),
			errGetPhotosByCheckId: nil,
		},
		{
			name:                  "ErrGetPhotosByCheckId",
			errGetCheckById:       nil,
			errGetPhotosByCheckId: errors.New(""),
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.checksRepo.On("GetCheckById", mock.Anything, mock.AnythingOfType("int")).Once().
				Return(models.Check{}, tt.errGetCheckById)

			if tt.errGetCheckById == nil {
				suite.photosRepo.On("GetPhotosByCheckId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int")).Once().
					Return([]string{}, tt.errGetPhotosByCheckId)
			}
			_, gotErr := suite.uc.GetCheckById(context.Background(), 1)

			if tt.errGetCheckById == nil && tt.errGetPhotosByCheckId == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByMarkId() {
	tests := []struct {
		name                 string
		errGetChecksByMarkId error
		errGetPhotosByMarkId error
	}{
		{
			name:                 "Ok",
			errGetChecksByMarkId: nil,
			errGetPhotosByMarkId: nil,
		},
		{
			name:                 "ErrGetChecksByMarkId",
			errGetChecksByMarkId: errors.New(""),
			errGetPhotosByMarkId: nil,
		},
		{
			name:                 "ErrGetPhotosByMarkId",
			errGetChecksByMarkId: nil,
			errGetPhotosByMarkId: errors.New(""),
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.checksRepo.On("GetChecksByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
				Return([]models.Check{{}, {}}, tt.errGetChecksByMarkId)

			if tt.errGetChecksByMarkId == nil {
				suite.photosRepo.On("GetPhotosByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(map[int]map[int][]string{}, tt.errGetPhotosByMarkId)
			}

			_, gotErr := suite.uc.GetChecksByMarkId(context.Background(), 1)

			if tt.errGetChecksByMarkId == nil && tt.errGetPhotosByMarkId == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByUserId() {
	tests := []struct {
		name                  string
		errGetChecksByUserId  error
		errGetPhotosByCheckId error
	}{
		{
			name:                  "Ok",
			errGetChecksByUserId:  nil,
			errGetPhotosByCheckId: nil,
		},
		{
			name:                  "ErrGetChecksByUserId",
			errGetChecksByUserId:  errors.New(""),
			errGetPhotosByCheckId: nil,
		},
		{
			name:                  "ErrGetPhotosByCheckId",
			errGetChecksByUserId:  nil,
			errGetPhotosByCheckId: errors.New(""),
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.checksRepo.On("GetChecksByUserId", mock.Anything, mock.AnythingOfType("int")).Once().
				Return([]models.Check{{}}, tt.errGetChecksByUserId)
			if tt.errGetChecksByUserId == nil {

				suite.photosRepo.On("GetPhotosByCheckId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int")).Once().
					Return([]string{}, tt.errGetPhotosByCheckId)
			}

			_, gotErr := suite.uc.GetChecksByUserId(context.Background(), 1)

			if tt.errGetChecksByUserId == nil && tt.errGetPhotosByCheckId == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
