package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// ratingCfg mirrors the config defaults so that delta assertions read naturally.
var ratingCfg = config.RatingConfig{
	CheckCorrect:    2,
	CheckWrong:      -1,
	MarkConfirmed:   3,
	MarkRefuted:     -2,
	TaskCompleted:   1,
	MaxChecksPerDay: 50,
}

// Fixed identities used by AddCheck tests: the checker never owns the mark.
const (
	testCheckerID = 1
	testAuthorID  = 2
)

// runInTx makes the transaction manager mock run the callback directly.
func runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

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
	usersRepo  *usecase.MockUsersRepository
}

func (suite *ChecksSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.updater = usecase.NewMockMarkStatusUpdater(suite.T())
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.tasksRepo = usecase.NewMockTasksRepository(suite.T())
	suite.photosRepo = usecase.NewMockPhotosRepository(suite.T())
	suite.usersRepo = usecase.NewMockUsersRepository(suite.T())
	suite.uc = usecase.NewChecks(suite.log, ratingCfg, suite.trManager, suite.updater, usecase.ChecksRepositories{
		Marks:  suite.marksRepo,
		Checks: suite.checksRepo,
		Tasks:  suite.tasksRepo,
		Photos: suite.photosRepo,
		Users:  suite.usersRepo,
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
				suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.Mark{ID: 1, UserID: testAuthorID}, nil)

				suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getLastMarkStatusHistoryItem.data, tt.getLastMarkStatusHistoryItem.err)
				if tt.getLastMarkStatusHistoryItem.err != nil {
					return
				}

				// Everything below runs inside a single transaction.
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)

				suite.checksRepo.On("CountChecksByUserIdSince", mock.Anything, testCheckerID, mock.AnythingOfType("time.Time")).Once().
					Return(0, nil)

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

				suite.tasksRepo.On("GetTaskByUserIdAndMarkId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int"), models.UnfulfilledStatus).Once().
					Return(tt.getTask.data, tt.getTask.err)
				if tt.getTask.err != nil {
					return
				}

				suite.tasksRepo.On("UpdateTaskStatus", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("models.TaskStatusType")).Once().
					Return(tt.updateTaskStatus.err)
				if tt.updateTaskStatus.err != nil {
					return
				}

				suite.usersRepo.On("AddRatingEvent", mock.Anything, models.RatingEvent{
					UserID:  testCheckerID,
					Delta:   ratingCfg.TaskCompleted,
					Reason:  models.RatingReasonTaskCompleted,
					MarkID:  null.IntFrom(1),
					CheckID: null.IntFrom(tt.addCheck.data),
				}).Once().Return(int64(1), nil)
			}()

			_, gotErr := suite.uc.AddCheck(context.Background(), models.Check{UserID: testCheckerID, MarkID: 1}, []io.Reader{})

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
			suite.usersRepo.AssertExpectations(suite.T())
			suite.trManager.AssertExpectations(suite.T())
		})
	}
}

// TestAddCheckOwnMark: the author may not vote on their own mark; nothing
// is written and the transaction is never opened.
func (suite *ChecksSuite) TestAddCheckOwnMark() {
	suite.marksRepo.On("GetMarkById", mock.Anything, 1).Once().
		Return(models.Mark{ID: 1, UserID: testCheckerID}, nil)

	_, err := suite.uc.AddCheck(context.Background(), models.Check{UserID: testCheckerID, MarkID: 1}, nil)

	suite.ErrorIs(err, usecase.ErrForbidden)
	suite.marksRepo.AssertNotCalled(suite.T(), "GetLastMarkStatusHistoryItem", mock.Anything, mock.Anything)
	suite.trManager.AssertNotCalled(suite.T(), "Do", mock.Anything, mock.Anything)
	suite.checksRepo.AssertNotCalled(suite.T(), "AddCheck", mock.Anything, mock.Anything)
}

// TestAddCheckMarkNotFound: an unknown mark is reported before any other lookup.
func (suite *ChecksSuite) TestAddCheckMarkNotFound() {
	suite.marksRepo.On("GetMarkById", mock.Anything, 1).Once().
		Return(models.Mark{}, repository.ErrNotFound)

	_, err := suite.uc.AddCheck(context.Background(), models.Check{UserID: testCheckerID, MarkID: 1}, nil)

	suite.ErrorIs(err, usecase.ErrNotFound)
}

// TestAddCheckDailyLimit covers the rolling 24h quota.
func (suite *ChecksSuite) TestAddCheckDailyLimit() {
	tests := []struct {
		name    string
		count   int
		countEr error
		wantErr error
	}{
		{name: "BelowLimit", count: ratingCfg.MaxChecksPerDay - 1},
		{name: "AtLimit", count: ratingCfg.MaxChecksPerDay, wantErr: usecase.ErrTooManyRequests},
		{name: "AboveLimit", count: ratingCfg.MaxChecksPerDay + 10, wantErr: usecase.ErrTooManyRequests},
		{name: "ErrCount", countEr: errRepo, wantErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.marksRepo.On("GetMarkById", mock.Anything, 1).Once().
				Return(models.Mark{ID: 1, UserID: testAuthorID}, nil)
			suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, 1).Once().
				Return(models.MarkStatusHistoryItem{ID: 7, NewMarkStatusID: models.UnconfirmedStatus}, nil)
			suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
			suite.checksRepo.On("CountChecksByUserIdSince", mock.Anything, testCheckerID, mock.AnythingOfType("time.Time")).Once().
				Run(func(args mock.Arguments) {
					since := args.Get(2).(time.Time)
					suite.WithinDuration(time.Now().Add(-24*time.Hour), since, time.Minute)
				}).
				Return(tt.count, tt.countEr)

			if tt.wantErr == nil {
				suite.checksRepo.On("GetUserMarkCheck", mock.Anything, testCheckerID, 7).Once().
					Return(models.Check{}, repository.ErrNotFound)
				suite.checksRepo.On("AddCheck", mock.Anything, mock.Anything).Once().Return(int64(5), nil)
				suite.photosRepo.On("AddPhotos", mock.Anything, 1, 5, mock.Anything).Once().Return(nil)
				suite.updater.On("Update", mock.Anything, 1).Once().Return(nil)
				suite.tasksRepo.On("GetTaskByUserIdAndMarkId", mock.Anything, testCheckerID, 1, models.UnfulfilledStatus).Once().
					Return(models.Task{}, repository.ErrNotFound)
			}

			id, err := suite.uc.AddCheck(context.Background(), models.Check{UserID: testCheckerID, MarkID: 1}, nil)

			// Mocks are strict: a rejected check that still reached AddCheck
			// would fail on the missing expectation.
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
				suite.Equal(int64(5), id)
			}
			// No task was closed, so no rating is awarded here.
			suite.usersRepo.AssertNotCalled(suite.T(), "AddRatingEvent", mock.Anything, mock.Anything)
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
				suite.checksRepo.On("GetChecksByMarkId", mock.Anything, mock.AnythingOfType("int"), models.Pagination{Limit: 10}).Once().
					Return(models.Page[models.Check]{Items: tt.getChecksByMarkId.data, Total: len(tt.getChecksByMarkId.data)}, tt.getChecksByMarkId.err)
				if tt.getChecksByMarkId.err != nil {
					return
				}

				suite.photosRepo.On("GetPhotosByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getPhotosByMarkId.data, tt.getPhotosByMarkId.err)
				if tt.getPhotosByMarkId.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.ListChecksByMarkId(context.Background(), 1, models.Pagination{Limit: 10})

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
	// Two checks on mark 1 and one on mark 2: photos must be listed once per mark.
	checks := []models.Check{{ID: 10, MarkID: 1}, {ID: 11, MarkID: 1}, {ID: 12, MarkID: 2}}
	photos := map[int]map[int][]string{
		1: {10: {"a.jpg"}, 11: {"b.jpg"}},
		2: {12: {"c.jpg"}},
	}

	tests := []struct {
		name              string
		getChecksByUserId method[[]models.Check]
		getPhotosByMarkId method[map[int]map[int][]string]
		wantPhotos        [][]string
	}{
		{
			name:              "Ok",
			getChecksByUserId: method[[]models.Check]{data: checks},
			getPhotosByMarkId: method[map[int]map[int][]string]{data: photos},
			wantPhotos:        [][]string{{"a.jpg"}, {"b.jpg"}, {"c.jpg"}},
		},
		{
			name:              "ErrGetChecksByUserId",
			getChecksByUserId: method[[]models.Check]{err: errRepo},
		},
		{
			name:              "ErrGetPhotosByMarkId",
			getChecksByUserId: method[[]models.Check]{data: checks},
			getPhotosByMarkId: method[map[int]map[int][]string]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.checksRepo.On("GetChecksByUserId", mock.Anything, mock.AnythingOfType("int"), models.Pagination{Limit: 10}).Once().
				Return(models.Page[models.Check]{Items: tt.getChecksByUserId.data, Total: len(tt.getChecksByUserId.data)}, tt.getChecksByUserId.err)
			if tt.getChecksByUserId.err == nil {
				if tt.getPhotosByMarkId.err != nil {
					suite.photosRepo.On("GetPhotosByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
						Return(nil, tt.getPhotosByMarkId.err)
				} else {
					for markId := range photos {
						suite.photosRepo.On("GetPhotosByMarkId", mock.Anything, markId).Once().
							Return(tt.getPhotosByMarkId.data, nil)
					}
				}
			}

			got, gotErr := suite.uc.ListChecksByUserId(context.Background(), 1, models.Pagination{Limit: 10})

			if tt.getChecksByUserId.err == nil && tt.getPhotosByMarkId.err == nil {
				suite.NoError(gotErr)
				for i, want := range tt.wantPhotos {
					suite.Equal(want, got.Items[i].Photos)
				}
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getChecksByUserId.err, tt.getPhotosByMarkId.err)
			}
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *ChecksSuite) TestListChecksInvalidPagination() {
	bad := models.Pagination{Limit: models.MaxLimit + 1}

	_, err := suite.uc.ListChecksByMarkId(context.Background(), 1, bad)
	suite.ErrorIs(err, usecase.ErrInvalidArgument)

	_, err = suite.uc.ListChecksByUserId(context.Background(), 1, bad)
	suite.ErrorIs(err, usecase.ErrInvalidArgument)

	suite.checksRepo.AssertNotCalled(suite.T(), "GetChecksByMarkId", mock.Anything, mock.Anything, bad)
	suite.checksRepo.AssertNotCalled(suite.T(), "GetChecksByUserId", mock.Anything, mock.Anything, bad)
}

type MarkStatusUpdaterSuite struct {
	suite.Suite
	u          *usecase.Updater
	log        *slog.Logger
	trManager  *usecase.MockManager
	marksRepo  *usecase.MockMarksRepository
	checksRepo *usecase.MockChecksRepository
	usersRepo  *usecase.MockUsersRepository
}

func (suite *MarkStatusUpdaterSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.usersRepo = usecase.NewMockUsersRepository(suite.T())
	suite.u = usecase.NewUpdater(suite.log, ratingCfg, suite.trManager, usecase.UpdaterRepositories{
		Marks:  suite.marksRepo,
		Checks: suite.checksRepo,
		Users:  suite.usersRepo,
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
						// One event per check plus one for the author (first decision).
						suite.usersRepo.On("AddRatingEvent", mock.Anything, mock.AnythingOfType("models.RatingEvent")).
							Times(len(tt.getChecksByMarkHistoryId.data)+1).Return(int64(1), nil)
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
			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}

// TestRatingDeltas pins the rating awarded on every resolved voting stage:
// checkers are rated by whether their vote matched the outcome, the author
// only on the first decision about the mark.
func (suite *MarkStatusUpdaterSuite) TestRatingDeltas() {
	const (
		markId   = 10
		authorId = 42
	)
	checks := []models.Check{
		{ID: 1, UserID: 101, Result: true},
		{ID: 2, UserID: 102, Result: false},
		{ID: 3, UserID: 103, Result: true},
	}
	checkerEvent := func(check models.Check, correct bool) models.RatingEvent {
		event := models.RatingEvent{
			UserID:  check.UserID,
			Delta:   ratingCfg.CheckWrong,
			Reason:  models.RatingReasonCheckWrong,
			MarkID:  null.IntFrom(markId),
			CheckID: null.IntFrom(int64(check.ID)),
		}
		if correct {
			event.Delta = ratingCfg.CheckCorrect
			event.Reason = models.RatingReasonCheckCorrect
		}
		return event
	}
	authorEvent := func(confirmed bool) models.RatingEvent {
		event := models.RatingEvent{
			UserID: authorId,
			Delta:  ratingCfg.MarkRefuted,
			Reason: models.RatingReasonMarkRefuted,
			MarkID: null.IntFrom(markId),
		}
		if confirmed {
			event.Delta = ratingCfg.MarkConfirmed
			event.Reason = models.RatingReasonMarkConfirmed
		}
		return event
	}

	tests := []struct {
		name       string
		status     models.MarkStatusType
		confirm    bool
		wantStatus models.MarkStatusType
		wantEvents []models.RatingEvent
	}{
		{
			name: "Unconfirmed->Confirmed", status: models.UnconfirmedStatus, confirm: true,
			wantStatus: models.ConfirmedStatus,
			wantEvents: []models.RatingEvent{
				checkerEvent(checks[0], true), checkerEvent(checks[1], false), checkerEvent(checks[2], true),
				authorEvent(true),
			},
		},
		{
			name: "Unconfirmed->Refuted", status: models.UnconfirmedStatus, confirm: false,
			wantStatus: models.RefutedStatus,
			wantEvents: []models.RatingEvent{
				checkerEvent(checks[0], false), checkerEvent(checks[1], true), checkerEvent(checks[2], false),
				authorEvent(false),
			},
		},
		{
			name: "UnderReview->Closed (no author event)", status: models.UnderReviewStatus, confirm: true,
			wantStatus: models.ClosedStatus,
			wantEvents: []models.RatingEvent{
				checkerEvent(checks[0], true), checkerEvent(checks[1], false), checkerEvent(checks[2], true),
			},
		},
		{
			name: "UnderReview->Rediscovered (no author event)", status: models.UnderReviewStatus, confirm: false,
			wantStatus: models.RediscoveredStatus,
			wantEvents: []models.RatingEvent{
				checkerEvent(checks[0], false), checkerEvent(checks[1], true), checkerEvent(checks[2], false),
			},
		},
		{
			name: "Confirmed->Refuted by moderator (no author event)", status: models.ConfirmedStatus, confirm: false,
			wantStatus: models.RefutedStatus,
			wantEvents: []models.RatingEvent{
				checkerEvent(checks[0], false), checkerEvent(checks[1], true), checkerEvent(checks[2], false),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
			suite.marksRepo.On("GetMarkById", mock.Anything, markId).Once().
				Return(models.Mark{ID: markId, UserID: authorId, MarkStatusID: tt.status}, nil)
			suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, markId).Once().
				Return(models.MarkStatusHistoryItem{ID: 3}, nil)
			suite.checksRepo.On("GetChecksByMarkHistoryId", mock.Anything, 3).Once().Return(checks, nil)
			suite.marksRepo.On("UpdateMarkStatus", mock.Anything, markId, tt.wantStatus).Once().Return(nil)

			var got []models.RatingEvent
			suite.usersRepo.On("AddRatingEvent", mock.Anything, mock.AnythingOfType("models.RatingEvent")).
				Times(len(tt.wantEvents)).
				Run(func(args mock.Arguments) {
					got = append(got, args.Get(1).(models.RatingEvent))
				}).
				Return(int64(1), nil)

			var (
				status models.MarkStatusType
				err    error
			)
			if tt.confirm {
				status, err = suite.u.Confirm(context.Background(), markId)
			} else {
				status, err = suite.u.Reject(context.Background(), markId)
			}

			suite.NoError(err)
			suite.Equal(tt.wantStatus, status)
			suite.Equal(tt.wantEvents, got)
		})
	}
}

// TestRatingEventFailureAborts: a failed rating write is propagated so the
// surrounding transaction rolls back the status change.
func (suite *MarkStatusUpdaterSuite) TestRatingEventFailureAborts() {
	suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
	suite.marksRepo.On("GetMarkById", mock.Anything, 1).Once().
		Return(models.Mark{ID: 1, UserID: 2, MarkStatusID: models.UnconfirmedStatus}, nil)
	suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, 1).Once().
		Return(models.MarkStatusHistoryItem{ID: 1}, nil)
	suite.checksRepo.On("GetChecksByMarkHistoryId", mock.Anything, 1).Once().
		Return([]models.Check{{ID: 1, UserID: 3, Result: true}}, nil)
	suite.marksRepo.On("UpdateMarkStatus", mock.Anything, 1, models.ConfirmedStatus).Once().Return(nil)
	suite.usersRepo.On("AddRatingEvent", mock.Anything, mock.Anything).Once().Return(int64(0), errRepo)

	_, err := suite.u.Confirm(context.Background(), 1)

	suite.ErrorIs(err, errRepo)
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
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)

				suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkById.data, tt.getMarkById.err)
				if tt.getMarkById.err != nil {
					return
				}

				suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.MarkStatusHistoryItem{ID: 1}, nil)
				suite.checksRepo.On("GetChecksByMarkHistoryId", mock.Anything, 1).Once().
					Return([]models.Check{{ID: 1, UserID: 3, Result: true}}, nil)
				if errors.Is(tt.err, usecase.ErrConflict) {
					return
				}

				suite.marksRepo.On("UpdateMarkStatus", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("models.MarkStatusType")).Once().
					Return(tt.updateMarkStatus.err)
				if tt.updateMarkStatus.err != nil {
					return
				}

				suite.usersRepo.On("AddRatingEvent", mock.Anything, mock.AnythingOfType("models.RatingEvent")).
					Return(int64(1), nil)
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
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)

				suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkById.data, tt.getMarkById.err)
				if tt.getMarkById.err != nil {
					return
				}

				suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.MarkStatusHistoryItem{ID: 1}, nil)
				suite.checksRepo.On("GetChecksByMarkHistoryId", mock.Anything, 1).Once().
					Return([]models.Check{{ID: 1, UserID: 3, Result: true}}, nil)
				if errors.Is(tt.err, usecase.ErrConflict) {
					return
				}

				suite.marksRepo.On("UpdateMarkStatus", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("models.MarkStatusType")).Once().
					Return(tt.updateMarkStatus.err)
				if tt.updateMarkStatus.err != nil {
					return
				}

				suite.usersRepo.On("AddRatingEvent", mock.Anything, mock.AnythingOfType("models.RatingEvent")).
					Return(int64(1), nil)
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
