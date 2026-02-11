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
	updater    *usecase.MockMarkStatusUpdater
	marksRepo  *usecase.MockMarksRepository
	checksRepo *usecase.MockChecksRepository
	photosRepo *usecase.MockPhotosRepository
}

func (suite *ChecksSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.updater = usecase.NewMockMarkStatusUpdater(suite.T())
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.photosRepo = usecase.NewMockPhotosRepository(suite.T())
	suite.uc = usecase.NewChecks(suite.log, suite.updater, usecase.ChecksRepositories{
		Marks:  suite.marksRepo,
		Checks: suite.checksRepo,
		Photos: suite.photosRepo,
	})
}

func TestChecks(t *testing.T) {
	suite.Run(t, new(ChecksSuite))
}

func (suite *ChecksSuite) TestAddCheck() {
	type method[T any] struct {
		data T
		err  error
	}

	tests := []struct {
		name      string
		addCheck  method[int64]
		addPhotos method[any]
		update    method[any]
	}{
		{
			name: "Ok",
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: nil,
			},
			update: method[any]{
				err: nil,
			},
		},
		{
			name: "ErrAddCheck",
			addCheck: method[int64]{
				data: int64(0),
				err:  errors.New(""),
			},
			addPhotos: method[any]{
				err: nil,
			},
			update: method[any]{
				err: nil,
			},
		},
		{
			name: "ErrAddPhotos",
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: errors.New(""),
			},
			update: method[any]{
				err: nil,
			},
		},
		{
			name: "ErrUpdate",
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: nil,
			},
			update: method[any]{
				err: errors.New(""),
			},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
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

				suite.updater.On("Update", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.update.err)
				if tt.update.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.AddCheck(context.Background(), models.Check{}, []io.Reader{})

			if tt.addCheck.err == nil && tt.addPhotos.err == nil && tt.update.err == nil {
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

type MarkStatusUpdaterSuite struct {
	suite.Suite
	u          *usecase.Updater
	log        *slog.Logger
	marksRepo  *usecase.MockMarksRepository
	checksRepo *usecase.MockChecksRepository
}

func (suite *MarkStatusUpdaterSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.u = usecase.NewUpdater(suite.log, usecase.UpdaterRepositories{
		Marks:  suite.marksRepo,
		Checks: suite.checksRepo,
	})
}

func TestMarkStatusUpdater(t *testing.T) {
	suite.Run(t, new(MarkStatusUpdaterSuite))
}

func (suite *MarkStatusUpdaterSuite) TestUpdateMarkStatus() {
	type method[T any] struct {
		data T
		err  error
	}

	tests := []struct {
		name              string
		getMarkById       method[models.Mark]
		getChecksByMarkId method[[]models.Check]
		wantUpdated       bool
		updateMarkStatus  method[any]
	}{
		{
			name: "Ok",
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: int(models.UnconfirmedStatus),
				},
				err: nil,
			},
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{
						Result: true,
					},
					{
						Result: false,
					},
				},
				err: nil,
			},
		},
		{
			name: "Ok-ConfirmedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: int(models.UnconfirmedStatus),
				},
				err: nil,
			},
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{
						Result: true,
					},
					{
						Result: true,
					},
					{
						Result: true,
					},
				},
				err: nil,
			},
			wantUpdated: true,
			updateMarkStatus: method[any]{
				err: nil,
			},
		},
		{
			name: "Err-ConfirmedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: int(models.UnconfirmedStatus),
				},
				err: nil,
			},
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{
						Result: true,
					},
					{
						Result: true,
					},
					{
						Result: true,
					},
				},
				err: nil,
			},
			wantUpdated: true,
			updateMarkStatus: method[any]{
				err: errors.New(""),
			},
		},
		{
			name: "Ok-RefutedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: int(models.UnconfirmedStatus),
				},
				err: nil,
			},
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{
						Result: false,
					},
					{
						Result: false,
					},
					{
						Result: false,
					},
				},
				err: nil,
			},
			wantUpdated: true,
			updateMarkStatus: method[any]{
				err: nil,
			},
		},
		{
			name: "Ok-RefutedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: int(models.UnconfirmedStatus),
				},
				err: nil,
			},
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{
						Result: false,
					},
					{
						Result: false,
					},
					{
						Result: false,
					},
				},
				err: nil,
			},
			wantUpdated: true,
			updateMarkStatus: method[any]{
				err: errors.New(""),
			},
		},
		{
			name: "Err-GetMarkById",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  errors.New(""),
			},
		},
		{
			name: "Err-GetChecksByMarkId",
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: int(models.UnconfirmedStatus),
				},
				err: nil,
			},
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{
						Result: true,
					},
					{
						Result: false,
					},
				},
				err: errors.New(""),
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

				if tt.getMarkById.data.MarkStatusID == int(models.UnconfirmedStatus) {
					suite.checksRepo.On("GetChecksByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
						Return(tt.getChecksByMarkId.data, tt.getChecksByMarkId.err)
					if tt.getChecksByMarkId.err != nil {
						return
					}

					if tt.wantUpdated {
						suite.marksRepo.On("UpdateMarkStatus", mock.Anything, mock.AnythingOfType("int"), mock.Anything).Once().
							Return(tt.updateMarkStatus.err)
						if tt.updateMarkStatus.err != nil {
							return
						}
					}
				}
			}()

			gotErr := suite.u.Update(context.Background(), 1)

			if tt.getMarkById.err == nil && tt.getChecksByMarkId.err == nil && tt.updateMarkStatus.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
