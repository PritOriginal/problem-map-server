package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// EventsSuite covers the domain events raised by the use cases: what is
// published, on which subject, and that nothing leaves a rolled back
// transaction.
type EventsSuite struct {
	suite.Suite
	publisher *events.MockPublisher
}

func (suite *EventsSuite) SetupTest() {
	suite.publisher = events.NewMockPublisher(suite.T())
}

func TestUsecaseEvents(t *testing.T) {
	suite.Run(t, new(EventsSuite))
}

// runTx runs the callback as if inside a transaction and then fails the
// commit with commitErr (nil commits).
func runTx(commitErr error) func(ctx context.Context, fn func(context.Context) error) error {
	return func(ctx context.Context, fn func(context.Context) error) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return commitErr
	}
}

func (suite *EventsSuite) TestUpdaterConfirmPublishes() {
	trm := usecase.NewMockManager(suite.T())
	marks := usecase.NewMockMarksRepository(suite.T())
	checks := usecase.NewMockChecksRepository(suite.T())
	users := usecase.NewMockUsersRepository(suite.T())
	updater := usecase.NewUpdater(slogdiscard.NewDiscardLogger(), ratingCfg, trm, usecase.UpdaterRepositories{
		Marks:  marks,
		Checks: checks,
		Users:  users,
	}).WithEvents(suite.publisher)

	tests := []struct {
		name      string
		run       func(ctx context.Context) (models.MarkStatusType, error)
		mark      models.Mark
		wantNew   models.MarkStatusType
		updateErr error
	}{
		{
			name:    "ConfirmPublishes",
			run:     func(ctx context.Context) (models.MarkStatusType, error) { return updater.Confirm(ctx, 5) },
			mark:    models.Mark{ID: 5, UserID: 3, MarkStatusID: models.UnconfirmedStatus},
			wantNew: models.ConfirmedStatus,
		},
		{
			name:    "RejectPublishes",
			run:     func(ctx context.Context) (models.MarkStatusType, error) { return updater.Reject(ctx, 5) },
			mark:    models.Mark{ID: 5, UserID: 3, MarkStatusID: models.UnconfirmedStatus},
			wantNew: models.RefutedStatus,
		},
		{
			name:      "UpdateErrorDoesNotPublish",
			run:       func(ctx context.Context) (models.MarkStatusType, error) { return updater.Confirm(ctx, 5) },
			mark:      models.Mark{ID: 5, UserID: 3, MarkStatusID: models.UnconfirmedStatus},
			wantNew:   models.ConfirmedStatus,
			updateErr: errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			trm.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
			marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
			marks.On("GetMarkById", mock.Anything, 5).Once().Return(tt.mark, nil)
			marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 5).Once().Return(models.MarkStatusHistoryItem{ID: 8}, nil)
			checks.On("GetChecksByMarkHistoryId", mock.Anything, 8).Once().Return([]models.Check{}, nil)
			marks.On("UpdateMarkStatus", mock.Anything, 5, tt.wantNew).Once().Return(tt.updateErr)
			if tt.updateErr == nil {
				// The author is rated on the first decision about the mark.
				users.On("AddRatingEvent", mock.Anything, mock.Anything).Once().Return(int64(1), nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkStatusChanged, mock.MatchedBy(func(ev events.MarkStatusChanged) bool {
					return ev.EventID != "" && ev.MarkID == 5 && ev.AuthorID == 3 &&
						ev.OldStatus == tt.mark.MarkStatusID && ev.NewStatus == tt.wantNew
				})).Once().Return(nil)
			}

			got, err := tt.run(context.Background())
			if tt.updateErr != nil {
				suite.Error(err)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.wantNew, got)
		})
	}
}

func (suite *EventsSuite) TestUpdaterCollectsPendingInsideTransaction() {
	marks := usecase.NewMockMarksRepository(suite.T())
	checks := usecase.NewMockChecksRepository(suite.T())
	users := usecase.NewMockUsersRepository(suite.T())
	updater := usecase.NewUpdater(slogdiscard.NewDiscardLogger(), ratingCfg, usecase.NewMockManager(suite.T()), usecase.UpdaterRepositories{
		Marks:  marks,
		Checks: checks,
		Users:  users,
	}).WithEvents(suite.publisher)

	marks.On("GetMarkById", mock.Anything, 5).Once().
		Return(models.Mark{ID: 5, UserID: 3, MarkStatusID: models.UnconfirmedStatus}, nil)
	marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 5).Once().
		Return(models.MarkStatusHistoryItem{ID: 8}, nil)
	checks.On("GetChecksByMarkHistoryId", mock.Anything, 8).Once().
		Return([]models.Check{{Result: true}, {Result: true}, {Result: true}}, nil)
	marks.On("UpdateMarkStatus", mock.Anything, 5, models.ConfirmedStatus).Once().Return(nil)
	// Three checkers and the author are rated inside the transaction.
	users.On("AddRatingEvent", mock.Anything, mock.Anything).Times(4).Return(int64(1), nil)

	var pending events.Pending
	ctx := events.WithPending(context.Background(), &pending)
	suite.NoError(updater.Update(ctx, 5))

	// Nothing published yet (the publisher mock would fail on an unexpected
	// call); the event is waiting for the commit.
	suite.Require().Len(pending.Events(), 1)
	ev, ok := pending.Events()[0].(events.MarkStatusChanged)
	suite.Require().True(ok)
	suite.Equal(5, ev.MarkID)
	suite.Equal(models.ConfirmedStatus, ev.NewStatus)
}

// checksMocks wires a Checks use case whose status updater is the real
// Updater, so AddCheck exercises the full pending-events path.
type checksMocks struct {
	trm    *usecase.MockManager
	marks  *usecase.MockMarksRepository
	checks *usecase.MockChecksRepository
	tasks  *usecase.MockTasksRepository
	photos *usecase.MockPhotosRepository
	users  *usecase.MockUsersRepository
}

func (suite *EventsSuite) newChecks() (*usecase.Checks, checksMocks) {
	m := checksMocks{
		trm:    usecase.NewMockManager(suite.T()),
		marks:  usecase.NewMockMarksRepository(suite.T()),
		checks: usecase.NewMockChecksRepository(suite.T()),
		tasks:  usecase.NewMockTasksRepository(suite.T()),
		photos: usecase.NewMockPhotosRepository(suite.T()),
		users:  usecase.NewMockUsersRepository(suite.T()),
	}
	log := slogdiscard.NewDiscardLogger()
	updater := usecase.NewUpdater(log, ratingCfg, m.trm, usecase.UpdaterRepositories{Marks: m.marks, Checks: m.checks, Users: m.users}).WithEvents(suite.publisher)
	uc := usecase.NewChecks(log, ratingCfg, m.trm, updater, usecase.ChecksRepositories{
		Marks:  m.marks,
		Checks: m.checks,
		Tasks:  m.tasks,
		Photos: m.photos,
		Users:  m.users,
	}).WithEvents(suite.publisher)
	return uc, m
}

func (suite *EventsSuite) TestAddCheckPublishesAfterCommit() {
	tests := []struct {
		name       string
		commitErr  error
		wantEvents bool
	}{
		{name: "CommittedPublishesBoth", wantEvents: true},
		{name: "RolledBackPublishesNothing", commitErr: errors.New("commit failed")},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			uc, m := suite.newChecks()
			check := models.Check{MarkID: 5, UserID: 2}

			m.marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 5).
				Return(models.MarkStatusHistoryItem{ID: 8, NewMarkStatusID: models.UnconfirmedStatus}, nil)
			m.trm.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(tt.commitErr))
			m.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
			// AddCheck reads the mark for the own-mark rule, Updater.Update reads it again.
			m.marks.On("GetMarkById", mock.Anything, 5).Twice().
				Return(models.Mark{ID: 5, UserID: 3, MarkStatusID: models.UnconfirmedStatus}, nil)
			m.checks.On("CountChecksByUserIdSince", mock.Anything, 2, mock.Anything).Once().Return(0, nil)
			m.checks.On("GetUserMarkCheck", mock.Anything, 2, 8).Once().Return(models.Check{}, repository.ErrNotFound)
			m.checks.On("AddCheck", mock.Anything, mock.Anything).Once().Return(int64(77), nil)
			m.photos.On("AddPhotos", mock.Anything, 5, 77, mock.Anything).Once().Return(nil)
			// Updater.Update: three positive checks confirm the mark.
			m.checks.On("GetChecksByMarkHistoryId", mock.Anything, 8).Once().
				Return([]models.Check{{Result: true}, {Result: true}, {Result: true}}, nil)
			m.marks.On("UpdateMarkStatus", mock.Anything, 5, models.ConfirmedStatus).Once().Return(nil)
			m.users.On("AddRatingEvent", mock.Anything, mock.Anything).Times(4).Return(int64(1), nil)
			m.tasks.On("GetTaskByUserIdAndMarkId", mock.Anything, 2, 5, models.UnfulfilledStatus).Once().
				Return(models.Task{}, repository.ErrNotFound)

			if tt.wantEvents {
				var order []string
				suite.publisher.On("Publish", mock.Anything, events.SubjectCheckAdded, mock.MatchedBy(func(ev events.CheckAdded) bool {
					return ev.CheckID == 77 && ev.MarkID == 5 && ev.UserID == 2 && ev.EventID != ""
				})).Once().Run(func(mock.Arguments) { order = append(order, events.SubjectCheckAdded) }).Return(nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkStatusChanged, mock.MatchedBy(func(ev events.MarkStatusChanged) bool {
					return ev.MarkID == 5 && ev.AuthorID == 3 && ev.NewStatus == models.ConfirmedStatus
				})).Once().Run(func(mock.Arguments) { order = append(order, events.SubjectMarkStatusChanged) }).Return(nil)
				defer func() {
					suite.Equal([]string{events.SubjectCheckAdded, events.SubjectMarkStatusChanged}, order)
				}()
			}

			id, err := uc.AddCheck(context.Background(), check, nil)
			if tt.commitErr != nil {
				suite.ErrorIs(err, tt.commitErr)
				return
			}
			suite.NoError(err)
			suite.Equal(int64(77), id)
		})
	}
}

func (suite *EventsSuite) TestAddCheckPublishesTaskCompleted() {
	tests := []struct {
		name      string
		task      models.Task
		taskErr   error
		wantEvent bool
	}{
		{name: "IssuedTaskClosed", task: models.Task{ID: 9, UserID: 2, MarkID: 5}, wantEvent: true},
		{name: "NoIssuedTask", taskErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			uc, m := suite.newChecks()
			check := models.Check{MarkID: 5, UserID: 2}

			m.marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 5).
				Return(models.MarkStatusHistoryItem{ID: 8, NewMarkStatusID: models.UnconfirmedStatus}, nil)
			m.trm.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
			m.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
			m.marks.On("GetMarkById", mock.Anything, 5).Twice().
				Return(models.Mark{ID: 5, UserID: 3, MarkStatusID: models.UnconfirmedStatus}, nil)
			m.checks.On("CountChecksByUserIdSince", mock.Anything, 2, mock.Anything).Once().Return(0, nil)
			m.checks.On("GetUserMarkCheck", mock.Anything, 2, 8).Once().Return(models.Check{}, repository.ErrNotFound)
			m.checks.On("AddCheck", mock.Anything, mock.Anything).Once().Return(int64(77), nil)
			m.photos.On("AddPhotos", mock.Anything, 5, 77, mock.Anything).Once().Return(nil)
			// One check does not resolve the stage: no status change.
			m.checks.On("GetChecksByMarkHistoryId", mock.Anything, 8).Once().Return([]models.Check{{Result: true}}, nil)
			m.tasks.On("GetTaskByUserIdAndMarkId", mock.Anything, 2, 5, models.UnfulfilledStatus).Once().Return(tt.task, tt.taskErr)
			if tt.wantEvent {
				m.tasks.On("UpdateTaskStatus", mock.Anything, 9, models.CompletedStatus).Once().Return(nil)
				m.users.On("AddRatingEvent", mock.Anything, mock.Anything).Once().Return(int64(1), nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectTaskCompleted, mock.MatchedBy(func(ev events.TaskCompleted) bool {
					return ev.TaskID == 9 && ev.UserID == 2 && ev.MarkID == 5 && ev.CheckID == 77 && ev.EventID != ""
				})).Once().Return(nil)
			}
			suite.publisher.On("Publish", mock.Anything, events.SubjectCheckAdded, mock.Anything).Once().Return(nil)

			_, err := uc.AddCheck(context.Background(), check, nil)
			suite.NoError(err)
		})
	}
}

func (suite *EventsSuite) TestTasksAddTaskPublishes() {
	tasks := usecase.NewMockTasksRepository(suite.T())
	uc := usecase.NewTasks(slogdiscard.NewDiscardLogger(), usecase.TasksRepositories{Tasks: tasks}).WithEvents(suite.publisher)

	tests := []struct {
		name   string
		addErr error
	}{
		{name: "OkPublishes"},
		{name: "ErrDoesNotPublish", addErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			task := models.Task{UserID: 2, MarkID: 5, DueAt: null.Time{}}
			tasks.On("AddTask", mock.Anything, task).Once().Return(int64(9), tt.addErr)
			if tt.addErr == nil {
				suite.publisher.On("Publish", mock.Anything, events.SubjectTaskAssigned, mock.MatchedBy(func(ev events.TaskAssigned) bool {
					return ev.TaskID == 9 && ev.UserID == 2 && ev.MarkID == 5 && ev.DueAt == nil && ev.EventID != ""
				})).Once().Return(nil)
			}

			_, err := uc.AddTask(context.Background(), task)
			if tt.addErr != nil {
				assertRepoErr(&suite.Suite, err, tt.addErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *EventsSuite) TestTaskerUpdatePublishesAfterCommit() {
	cfg := config.TaskerConfig{
		Interval: 15 * time.Minute, TaskTTL: 72 * time.Hour, MaxTasksPerUser: 3, RequiredChecks: 1,
		TargetProbability: 0.01, MaxRadiusMeters: 5000, DistanceLambda: 0.05, LoadDelta: 0.3, FatigueBeta: 0.2,
	}

	tests := []struct {
		name      string
		commitErr error
	}{
		{name: "CommittedPublishes"},
		{name: "RolledBackPublishesNothing", commitErr: errors.New("commit failed")},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			tasks := usecase.NewMockTaskerTasksRepository(suite.T())
			marks := usecase.NewMockMarksRepository(suite.T())
			users := usecase.NewMockUsersRepository(suite.T())
			trm := usecase.NewMockManager(suite.T())
			uc := usecase.NewTasker(slogdiscard.NewDiscardLogger(), cfg, trm, usecase.TaskerRepositories{
				Tasks: tasks, Marks: marks, Users: users,
			}).WithEvents(suite.publisher)

			marks.On("GetMarks", mock.Anything, mock.Anything).Once().
				Return(models.Page[models.Mark]{Items: []models.Mark{{ID: 5}}}, nil)
			users.On("GetUsers", mock.Anything, mock.Anything).Once().
				Return(models.Page[models.User]{Items: []models.User{{Id: 2, Rating: 5}}}, nil)
			tasks.On("GetTasks", mock.Anything, mock.Anything).Once().
				Return(models.Page[models.Task]{}, nil)
			marks.On("GetDistancesFromMarkToPoint", mock.Anything, mock.Anything).Once().
				Return([]models.DistanceFromMarkToPoint{{MarkId: 5, UserId: 2, Distance: 0.1}}, nil)
			trm.On("Do", mock.Anything, mock.Anything).Once().
				Return(func(ctx context.Context, fn func(context.Context) error) error {
					if err := fn(ctx); err != nil {
						return err
					}
					return tt.commitErr
				})
			tasks.On("AddTask", mock.Anything, mock.MatchedBy(func(t models.Task) bool {
				return t.UserID == 2 && t.MarkID == 5
			})).Once().Return(int64(9), nil)
			if tt.commitErr == nil {
				suite.publisher.On("Publish", mock.Anything, events.SubjectTaskAssigned, mock.MatchedBy(func(ev events.TaskAssigned) bool {
					return ev.TaskID == 9 && ev.UserID == 2 && ev.MarkID == 5 && ev.DueAt != nil && ev.EventID != ""
				})).Once().Return(nil)
			}

			stats, err := uc.Update(context.Background())
			if tt.commitErr != nil {
				suite.ErrorIs(err, tt.commitErr)
				return
			}
			suite.NoError(err)
			suite.Equal(1, stats.Assigned)
		})
	}
}

func (suite *EventsSuite) TestWithEventsNilKeepsNoop() {
	tasks := usecase.NewMockTasksRepository(suite.T())
	uc := usecase.NewTasks(slogdiscard.NewDiscardLogger(), usecase.TasksRepositories{Tasks: tasks}).WithEvents(nil)
	tasks.On("AddTask", mock.Anything, mock.Anything).Once().Return(int64(1), nil)

	_, err := uc.AddTask(context.Background(), models.Task{})
	suite.NoError(err)
}
