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
	suite.uc = usecase.NewMarks(suite.log, usecase.MarksRepositories{
		Marks:  suite.marksRepo,
		Checks: suite.checksRepo,
		Photos: suite.photosRepo,
	})
}

func TestMarks(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func (suite *MarksSuite) TestGetMarks() {
	tests := []struct {
		name     string
		getMarks method[[]models.Mark]
	}{
		{
			name: "Ok",
			getMarks: method[[]models.Mark]{
				data: []models.Mark{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarks: method[[]models.Mark]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarks", mock.Anything).Once().
					Return(tt.getMarks.data, tt.getMarks.err)
				if tt.getMarks.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarks(context.Background())

			if tt.getMarks.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkById() {
	tests := []struct {
		name        string
		getMarkById method[models.Mark]
	}{
		{
			name: "Ok",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkById.data, tt.getMarkById.err)
				if tt.getMarkById.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkById(context.Background(), 1)

			if tt.getMarkById.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarksByUserId() {
	tests := []struct {
		name             string
		getMarksByUserId method[[]models.Mark]
	}{
		{
			name: "Ok",
			getMarksByUserId: method[[]models.Mark]{
				data: []models.Mark{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarksByUserId: method[[]models.Mark]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarksByUserId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarksByUserId.data, tt.getMarksByUserId.err)
				if tt.getMarksByUserId.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarksByUserId(context.Background(), 1)

			if tt.getMarksByUserId.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestAddMark() {
	tests := []struct {
		name      string
		addMark   method[int64]
		addCheck  method[int64]
		addPhotos method[any]
	}{
		{
			name: "Ok",
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: nil,
			},
		},
		{
			name: "ErrAddMark",
			addMark: method[int64]{
				data: int64(0),
				err:  errors.New(""),
			},
			addCheck: method[int64]{
				data: int64(0),
				err:  nil,
			},
			addPhotos: method[any]{
				err: nil,
			},
		},
		{
			name: "ErrAddCheck",
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addCheck: method[int64]{
				data: int64(0),
				err:  errors.New(""),
			},
			addPhotos: method[any]{
				err: nil,
			},
		},
		{
			name: "ErrAddPhotos",
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("AddMark", mock.Anything, mock.Anything).Once().
					Return(tt.addMark.data, tt.addMark.err)
				if tt.addMark.err != nil {
					return
				}

				suite.checksRepo.On("AddCheck", mock.Anything, mock.Anything).Once().
					Return(tt.addCheck.data, tt.addCheck.err)
				if tt.addCheck.err != nil {
					return
				}

				suite.photosRepo.On("AddPhotos", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int"), mock.Anything).Once().
					Return(tt.addPhotos.err)
				if tt.addPhotos.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.AddMark(context.Background(), models.Mark{}, []io.Reader{})

			if tt.addMark.err == nil && tt.addCheck.err == nil && tt.addPhotos.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkTypes() {
	tests := []struct {
		name         string
		getMarkTypes method[[]models.MarkType]
	}{
		{
			name: "Ok",
			getMarkTypes: method[[]models.MarkType]{
				data: []models.MarkType{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarkTypes: method[[]models.MarkType]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkTypes", mock.Anything).Once().
					Return(tt.getMarkTypes.data, tt.getMarkTypes.err)
				if tt.getMarkTypes.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkTypes(context.Background())

			if tt.getMarkTypes.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatuses() {
	tests := []struct {
		name            string
		getMarkStatuses method[[]models.MarkStatus]
	}{
		{
			name: "Ok",
			getMarkStatuses: method[[]models.MarkStatus]{
				data: []models.MarkStatus{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarkStatuses: method[[]models.MarkStatus]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkStatuses", mock.Anything).Once().
					Return(tt.getMarkStatuses.data, tt.getMarkStatuses.err)
				if tt.getMarkStatuses.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkStatuses(context.Background())

			if tt.getMarkStatuses.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatusHistoryByMarkId() {
	tests := []struct {
		name                         string
		getMarkStatusHistoryByMarkId method[[]models.MarkStatusHistoryItem]
		withChecks                   bool
		getChecksByMarkId            method[[]models.Check]
		getPhotosByMarkId            method[map[int]map[int][]string]
	}{
		{
			name: "Ok",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: nil,
			},
			withChecks: false,
		},
		{
			name: "OkWithChecks",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				data: []models.MarkStatusHistoryItem{
					{ID: 1},
					{ID: 2},
				},
				err: nil,
			},
			withChecks: true,
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{ID: 1, MarkStatusHistoryItemId: 1},
					{ID: 2, MarkStatusHistoryItemId: 1},
				},
				err: nil,
			},
			getPhotosByMarkId: method[map[int]map[int][]string]{
				data: map[int]map[int][]string{
					1: {
						1: []string{"1", "2"},
					},
				},
				err: nil,
			},
		},
		{
			name: "ErrGetMarkStatusHistoryByMarkId",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: errors.New(""),
			},
		},
		{
			name: "ErrGetChecksByMarkId",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: nil,
			},
			withChecks: true,
			getChecksByMarkId: method[[]models.Check]{
				err: errors.New(""),
			},
		},
		{
			name: "ErrGetPhotosByMarkId",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: nil,
			},
			withChecks: true,
			getChecksByMarkId: method[[]models.Check]{
				err: nil,
			},
			getPhotosByMarkId: method[map[int]map[int][]string]{
				err: errors.New(""),
			},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkStatusHistoryByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkStatusHistoryByMarkId.data, tt.getMarkStatusHistoryByMarkId.err)
				if tt.getMarkStatusHistoryByMarkId.err != nil {
					return
				}

				if tt.withChecks {
					suite.checksRepo.On("GetChecksByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
						Return(tt.getChecksByMarkId.data, tt.getChecksByMarkId.err)
					if tt.getChecksByMarkId.err != nil {
						return
					}

					suite.photosRepo.On("GetPhotosByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
						Return(tt.getPhotosByMarkId.data, tt.getPhotosByMarkId.err)
					if tt.getPhotosByMarkId.err != nil {
						return
					}
				}
			}()

			_, gotErr := suite.uc.GetMarkStatusHistoryByMarkId(context.Background(), 1, tt.withChecks)

			if tt.getMarkStatusHistoryByMarkId.err == nil &&
				tt.getChecksByMarkId.err == nil &&
				tt.getPhotosByMarkId.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}
