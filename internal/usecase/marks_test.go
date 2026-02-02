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

type MarksSuite struct {
	suite.Suite
	uc         *usecase.Marks
	log        *slog.Logger
	marksRepo  *usecase.MockMarksRepository
	checksRepo *usecase.MockChecksRepository
	photosRepo *usecase.MockPhotosRepository
}

func (suite *MarksSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.photosRepo = usecase.NewMockPhotosRepository(suite.T())
	suite.uc = usecase.NewMarks(suite.log, suite.marksRepo, suite.checksRepo, suite.photosRepo)
}

func TestMarks(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func (suite *MarksSuite) TestGetMarks() {
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
			marksRepoCall := suite.marksRepo.On("GetMarks", mock.Anything).Once()
			if !tt.wantErr {
				marksRepoCall.Return([]models.Mark{}, nil)
			} else {
				marksRepoCall.Return([]models.Mark{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetMarks(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkById() {
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
			marksRepoCall := suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				marksRepoCall.Return(models.Mark{}, nil)
			} else {
				marksRepoCall.Return(models.Mark{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetMarkById(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarksByUserId() {
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
			marksRepoCall := suite.marksRepo.On("GetMarksByUserId", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				marksRepoCall.Return([]models.Mark{}, nil)
			} else {
				marksRepoCall.Return([]models.Mark{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetMarksByUserId(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *MarksSuite) TestAddMark() {
	tests := []struct {
		name             string
		addMarkWantErr   bool
		addCheckWantErr  bool
		addPhotosWantErr bool
	}{
		{
			name:             "Ok",
			addMarkWantErr:   false,
			addCheckWantErr:  false,
			addPhotosWantErr: false,
		},
		{
			name:             "ErrAddMark",
			addMarkWantErr:   true,
			addCheckWantErr:  false,
			addPhotosWantErr: false,
		},
		{
			name:             "ErrAddCheck",
			addMarkWantErr:   false,
			addCheckWantErr:  true,
			addPhotosWantErr: false,
		},
		{
			name:             "ErrAddPhotos",
			addMarkWantErr:   false,
			addCheckWantErr:  false,
			addPhotosWantErr: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			marksRepoCall := suite.marksRepo.On("AddMark", mock.Anything, mock.Anything).Once()
			if !tt.addMarkWantErr {
				marksRepoCall.Return(int64(1), nil)

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
			} else {
				marksRepoCall.Return(int64(0), errors.New(""))
			}

			_, gotErr := suite.uc.AddMark(context.Background(), models.Mark{}, []io.Reader{})

			if !tt.addMarkWantErr && !tt.addCheckWantErr && !tt.addPhotosWantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkTypes() {
	tests := []struct {
		name    string
		want    []models.Mark
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
			marksRepoCall := suite.marksRepo.On("GetMarkTypes", mock.Anything).Once()
			if !tt.wantErr {
				marksRepoCall.Return([]models.MarkType{}, nil)
			} else {
				marksRepoCall.Return([]models.MarkType{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetMarkTypes(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatuses() {
	tests := []struct {
		name    string
		want    []models.Mark
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
			marksRepoCall := suite.marksRepo.On("GetMarkStatuses", mock.Anything).Once()
			if !tt.wantErr {
				marksRepoCall.Return([]models.MarkStatus{}, nil)
			} else {
				marksRepoCall.Return([]models.MarkStatus{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetMarkStatuses(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
