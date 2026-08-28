package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type ChecksSuite struct {
	suite.Suite
	uc         *usecase.Checks
	log        *slog.Logger
	trManager  *usecase.MockManager
	updater    *usecase.MockMarkStatusUpdater
	marksRepo  *usecase.MockMarksRepository
	checksRepo *usecase.MockChecksRepository
	tasksRepo  *usecase.MockTasksRepository
	photosRepo *usecase.MockPhotosRepository
}

func (suite *ChecksSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.updater = usecase.NewMockMarkStatusUpdater(suite.T())
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.tasksRepo = usecase.NewMockTasksRepository(suite.T())
	suite.photosRepo = usecase.NewMockPhotosRepository(suite.T())
	suite.uc = usecase.NewChecks(suite.log, suite.trManager, suite.updater, usecase.ChecksRepositories{
		Marks:  suite.marksRepo,
		Checks: suite.checksRepo,
		Tasks:  suite.tasksRepo,
		Photos: suite.photosRepo,
	})
}

func TestChecks(t *testing.T) {
	suite.Run(t, new(ChecksSuite))
}

func (suite *ChecksSuite) TestAddCheck() {
	tests := []struct {
		name                         string
		getLastMarkStatusHistoryItem method[models.MarkStatusHistoryItem]
		getUserMarkCheck             method[models.Check]
		addCheck                     method[int64]
		addPhotos                    method[any]
		update                       method[any]
		getTask                      method[models.Task]
		updateTaskStatus             method[any]
		wantErr                      error
	}{
		{
			name: "Ok",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
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
			getTask: method[models.Task]{
				data: models.Task{ID: 1},
				err:  nil,
			},
			updateTaskStatus: method[any]{
				err: nil,
			},
		},
		{
			name: "OkWithoutTask",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
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
			getTask: method[models.Task]{
				err: repository.ErrNotFound,
			},
		},
		{
			name: "ErrGetTask",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
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
			getTask: method[models.Task]{
				err: errRepo,
			},
		},
		{
			name: "ErrUpdateTaskStatus",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
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
			getTask: method[models.Task]{
				data: models.Task{ID: 1},
				err:  nil,
			},
			updateTaskStatus: method[any]{
				err: errRepo,
			},
		},
		{
			name: "ErrNotFoundMarkStatus",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				err: repository.ErrNotFound,
			},
		},
		{
			name: "ErrGetLastMarkStatusHistoryItem",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: errRepo,
			},
		},
		{
			name: "ErrGetUserMarkCheck",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: errRepo,
			},
		},
		{
			name: "ErrConflictAlreadyChecked",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: nil,
			},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrConflictAddCheckExists",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
			addCheck: method[int64]{
				err: repository.ErrExists,
			},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrAddCheck",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
			addCheck: method[int64]{
				data: int64(0),
				err:  errRepo,
			},
		},
		{
			name: "ErrAddPhotos",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: errRepo,
			},
		},
		{
			name: "ErrUpdate",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					NewMarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getUserMarkCheck: method[models.Check]{
				err: repository.ErrNotFound,
			},
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: nil,
			},
			update: method[any]{
				err: errRepo,
			},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getLastMarkStatusHistoryItem.data, tt.getLastMarkStatusHistoryItem.err)
				if tt.getLastMarkStatusHistoryItem.err != nil {
					return
				}

				// Everything below runs inside a single transaction.
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().
					Return(func(ctx context.Context, fn func(ctx context.Context) error) error {
						return fn(ctx)
					})

				suite.checksRepo.On("GetUserMarkCheck", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int"), mock.Anything).Once().
					Return(tt.getUserMarkCheck.data, tt.getUserMarkCheck.err)
				if !errors.Is(tt.getUserMarkCheck.err, repository.ErrNotFound) {
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

				suite.updater.On("Update", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.update.err)
				if tt.update.err != nil {
					return
				}

				suite.tasksRepo.On("GetTaskByUserIdAndMarkId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int")).Once().
					Return(tt.getTask.data, tt.getTask.err)
				if tt.getTask.err != nil {
					return
				}

				suite.tasksRepo.On("UpdateTaskStatus", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("models.TaskStatusType")).Once().
					Return(tt.updateTaskStatus.err)
			}()

			_, gotErr := suite.uc.AddCheck(context.Background(), models.Check{}, []io.Reader{})

			switch {
			case tt.getLastMarkStatusHistoryItem.err == nil &&
				errors.Is(tt.getUserMarkCheck.err, repository.ErrNotFound) &&
				tt.addCheck.err == nil &&
				tt.addPhotos.err == nil &&
				tt.update.err == nil &&
				(tt.getTask.err == nil || errors.Is(tt.getTask.err, repository.ErrNotFound)) &&
				tt.updateTaskStatus.err == nil:
				suite.NoError(gotErr)
			case tt.wantErr != nil:
				suite.ErrorIs(gotErr, tt.wantErr)
			default:
				suite.Error(gotErr)
			}
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
			suite.updater.AssertExpectations(suite.T())
			suite.tasksRepo.AssertExpectations(suite.T())
			suite.trManager.AssertExpectations(suite.T())
		})
	}
}

func (suite *ChecksSuite) TestGetCheckById() {
	tests := []struct {
		name               string
		getCheckById       method[models.Check]
		getPhotosByCheckId method[[]string]
	}{
		{
			name:               "Ok",
			getCheckById:       method[models.Check]{},
			getPhotosByCheckId: method[[]string]{},
		},
		{
			name: "ErrGetCheckById",
			getCheckById: method[models.Check]{
				err: errRepo,
			},
			getPhotosByCheckId: method[[]string]{},
		},
		{
			name: "ErrGetCheckByIdNotFound",
			getCheckById: method[models.Check]{
				err: repository.ErrNotFound,
			},
			getPhotosByCheckId: method[[]string]{},
		},
		{
			name:         "ErrGetPhotosByCheckId",
			getCheckById: method[models.Check]{},
			getPhotosByCheckId: method[[]string]{
				err: errRepo,
			},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.checksRepo.On("GetCheckById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getCheckById.data, tt.getCheckById.err)
				if tt.getCheckById.err != nil {
					return
				}

				suite.photosRepo.On("GetPhotosByCheckId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int")).Once().
					Return(tt.getPhotosByCheckId.data, tt.getPhotosByCheckId.err)
				if tt.getPhotosByCheckId.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetCheckById(context.Background(), 1)

			if tt.getCheckById.err == nil && tt.getPhotosByCheckId.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getCheckById.err, tt.getPhotosByCheckId.err)
			}
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByMarkId() {
	tests := []struct {
		name              string
		getChecksByMarkId method[[]models.Check]
		getPhotosByMarkId method[map[int]map[int][]string]
	}{
		{
			name: "Ok",
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{{}, {}},
				err:  nil,
			},
			getPhotosByMarkId: method[map[int]map[int][]string]{
				data: map[int]map[int][]string{},
				err:  nil,
			},
		},
		{
			name: "ErrGetChecksByMarkId",
			getChecksByMarkId: method[[]models.Check]{
				data: nil,
				err:  errRepo,
			},
			getPhotosByMarkId: method[map[int]map[int][]string]{
				data: nil,
				err:  nil,
			},
		},
		{
			name: "ErrGetPhotosByMarkId",
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{{}, {}},
				err:  nil,
			},
			getPhotosByMarkId: method[map[int]map[int][]string]{
				data: nil,
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
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
			}()

			_, gotErr := suite.uc.GetChecksByMarkId(context.Background(), 1)

			if tt.getChecksByMarkId.err == nil && tt.getPhotosByMarkId.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getChecksByMarkId.err, tt.getPhotosByMarkId.err)
			}
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByUserId() {
	tests := []struct {
		name               string
		getChecksByUserId  method[[]models.Check]
		getPhotosByCheckId method[[]string]
	}{
		{
			name: "Ok",
			getChecksByUserId: method[[]models.Check]{
				data: []models.Check{{}},
				err:  nil,
			},
			getPhotosByCheckId: method[[]string]{
				data: []string{},
				err:  nil,
			},
		},
		{
			name: "ErrGetChecksByUserId",
			getChecksByUserId: method[[]models.Check]{
				data: nil,
				err:  errRepo,
			},
			getPhotosByCheckId: method[[]string]{
				data: nil,
				err:  nil,
			},
		},
		{
			name: "ErrGetPhotosByCheckId",
			getChecksByUserId: method[[]models.Check]{
				data: []models.Check{{}},
				err:  nil,
			},
			getPhotosByCheckId: method[[]string]{
				data: nil,
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.checksRepo.On("GetChecksByUserId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getChecksByUserId.data, tt.getChecksByUserId.err)
				if tt.getChecksByUserId.err != nil {
					return
				}

				suite.photosRepo.On("GetPhotosByCheckId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int")).Once().
					Return(tt.getPhotosByCheckId.data, tt.getPhotosByCheckId.err)
				if tt.getPhotosByCheckId.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetChecksByUserId(context.Background(), 1)

			if tt.getChecksByUserId.err == nil && tt.getPhotosByCheckId.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getChecksByUserId.err, tt.getPhotosByCheckId.err)
			}
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
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

func (suite *MarkStatusUpdaterSuite) SetupTest() {
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
	tests := []struct {
		name                         string
		getMarkById                  method[models.Mark]
		getLastMarkStatusHistoryItem method[models.MarkStatusHistoryItem]
		getChecksByMarkHistoryId     method[[]models.Check]
		wantUpdated                  bool
		updateMarkStatus             method[any]
	}{
		{
			name: "Ok",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					ID: 1,
				},
			},
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getChecksByMarkHistoryId: method[[]models.Check]{
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
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					ID: 1,
				},
			},
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getChecksByMarkHistoryId: method[[]models.Check]{
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
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					ID: 1,
				},
			},
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getChecksByMarkHistoryId: method[[]models.Check]{
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
				err: errRepo,
			},
		},
		{
			name: "Ok-RefutedStatus",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					ID: 1,
				},
			},
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getChecksByMarkHistoryId: method[[]models.Check]{
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
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					ID: 1,
				},
			},
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getChecksByMarkHistoryId: method[[]models.Check]{
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
				err: errRepo,
			},
		},
		{
			name: "Err-GetMarkStatusHistoryByMarkId",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				err: errRepo,
			},
		},
		{
			name: "Err-GetMarkById",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					ID: 1,
				},
			},
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  errRepo,
			},
		},
		{
			name: "Err-GetChecksByMarkId",
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				data: models.MarkStatusHistoryItem{
					ID: 1,
				},
			},
			getMarkById: method[models.Mark]{
				data: models.Mark{
					MarkStatusID: models.UnconfirmedStatus,
				},
				err: nil,
			},
			getChecksByMarkHistoryId: method[[]models.Check]{
				data: []models.Check{
					{
						Result: true,
					},
					{
						Result: false,
					},
				},
				err: errRepo,
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

				suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getLastMarkStatusHistoryItem.data, tt.getLastMarkStatusHistoryItem.err)
				if tt.getLastMarkStatusHistoryItem.err != nil {
					return
				}

				if tt.getMarkById.data.MarkStatusID == models.UnconfirmedStatus {
					suite.checksRepo.On("GetChecksByMarkHistoryId", mock.Anything, mock.AnythingOfType("int")).Once().
						Return(tt.getChecksByMarkHistoryId.data, tt.getChecksByMarkHistoryId.err)
					if tt.getChecksByMarkHistoryId.err != nil {
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

			if tt.getLastMarkStatusHistoryItem.err == nil &&
				tt.getMarkById.err == nil &&
				tt.getChecksByMarkHistoryId.err == nil &&
				tt.updateMarkStatus.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getLastMarkStatusHistoryItem.err, tt.getMarkById.err, tt.getChecksByMarkHistoryId.err, tt.updateMarkStatus.err)
			}
			suite.marksRepo.AssertExpectations(suite.T())
			suite.checksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarkStatusUpdaterSuite) TestConfirm() {
	tests := []struct {
		name             string
		getMarkById      method[models.Mark]
		updateMarkStatus method[any]
		want             models.MarkStatusType
		err              error
	}{
		{
			name: "Ok-UnconfirmedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.UnconfirmedStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.ConfirmedStatus,
		},
		{
			name: "Ok-ConfirmedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.ConfirmedStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.UnderReviewStatus,
		},
		{
			name: "Ok-RediscoveredStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.RediscoveredStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.UnderReviewStatus,
		},
		{
			name: "Ok-UnderReviewStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.UnderReviewStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.ClosedStatus,
		},
		{
			name: "Err-Conflict",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.ClosedStatus},
				err:  nil,
			},
			err: usecase.ErrConflict,
		},
		{
			name: "Err-GetMarkById",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  errRepo,
			},
		},
		{
			name: "Err-GetMarkByIdNotFound",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  repository.ErrNotFound,
			},
		},
		{
			name: "Err-UpdateMarkStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.UnconfirmedStatus},
			},
			updateMarkStatus: method[any]{
				err: errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkById.data, tt.getMarkById.err)
				if tt.getMarkById.err != nil || errors.Is(tt.err, usecase.ErrConflict) {
					return
				}

				suite.marksRepo.On("UpdateMarkStatus", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("models.MarkStatusType")).Once().
					Return(tt.updateMarkStatus.err)
				if tt.updateMarkStatus.err != nil {
					return
				}
			}()

			got, gotErr := suite.u.Confirm(context.Background(), 1)

			if tt.getMarkById.err == nil && tt.updateMarkStatus.err == nil && tt.err == nil {
				suite.Equal(tt.want, got)
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getMarkById.err, tt.updateMarkStatus.err, tt.err)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarkStatusUpdaterSuite) TestReject() {
	tests := []struct {
		name             string
		getMarkById      method[models.Mark]
		updateMarkStatus method[any]
		want             models.MarkStatusType
		err              error
	}{
		{
			name: "Ok-UnconfirmedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.UnconfirmedStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.RefutedStatus,
		},
		{
			name: "Ok-ConfirmedStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.ConfirmedStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.RefutedStatus,
		},
		{
			name: "Ok-RediscoveredStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.RediscoveredStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.ClosedStatus,
		},
		{
			name: "Ok-UnderReviewStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.UnderReviewStatus},
				err:  nil,
			},
			updateMarkStatus: method[any]{},
			want:             models.RediscoveredStatus,
		},
		{
			name: "Err-Conflict",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.ClosedStatus},
				err:  nil,
			},
			err: usecase.ErrConflict,
		},
		{
			name: "Err-GetMarkById",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  errRepo,
			},
		},
		{
			name: "Err-GetMarkByIdNotFound",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  repository.ErrNotFound,
			},
		},
		{
			name: "Err-UpdateMarkStatus",
			getMarkById: method[models.Mark]{
				data: models.Mark{MarkStatusID: models.UnconfirmedStatus},
			},
			updateMarkStatus: method[any]{
				err: errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkById.data, tt.getMarkById.err)
				if tt.getMarkById.err != nil || errors.Is(tt.err, usecase.ErrConflict) {
					return
				}

				suite.marksRepo.On("UpdateMarkStatus", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("models.MarkStatusType")).Once().
					Return(tt.updateMarkStatus.err)
				if tt.updateMarkStatus.err != nil {
					return
				}
			}()

			got, gotErr := suite.u.Reject(context.Background(), 1)

			if tt.getMarkById.err == nil && tt.updateMarkStatus.err == nil && tt.err == nil {
				suite.Equal(tt.want, got)
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getMarkById.err, tt.updateMarkStatus.err, tt.err)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}
