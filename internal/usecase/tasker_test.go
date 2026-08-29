package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type TaskerSuite struct {
	suite.Suite
}

func TestTasker(t *testing.T) {
	suite.Run(t, new(TaskerSuite))
}

func testTaskerConfig() config.TaskerConfig {
	return config.TaskerConfig{
		Interval:          15 * time.Minute,
		TaskTTL:           72 * time.Hour,
		MaxTasksPerUser:   3,
		RequiredChecks:    2,
		TargetProbability: 0.8,
		MaxRadiusMeters:   5000,
		DistanceLambda:    0.05,
		LoadDelta:         0.3,
		FatigueBeta:       0.2,
	}
}

type taskerMocks struct {
	tasks *usecase.MockTaskerTasksRepository
	marks *usecase.MockMarksRepository
	users *usecase.MockUsersRepository
	trm   *usecase.MockManager
}

func (suite *TaskerSuite) newTasker(cfg config.TaskerConfig) (*usecase.Tasker, taskerMocks) {
	m := taskerMocks{
		tasks: usecase.NewMockTaskerTasksRepository(suite.T()),
		marks: usecase.NewMockMarksRepository(suite.T()),
		users: usecase.NewMockUsersRepository(suite.T()),
		trm:   usecase.NewMockManager(suite.T()),
	}
	uc := usecase.NewTasker(slogdiscard.NewDiscardLogger(), cfg, m.trm, usecase.TaskerRepositories{
		Tasks: m.tasks,
		Marks: m.marks,
		Users: m.users,
	})
	return uc, m
}

// assigned is a (user, mark) pair passed to AddTask.
type assigned struct {
	userId, markId int
}

func (suite *TaskerSuite) TestUpdate() {
	marks := []models.Mark{{ID: 1}, {ID: 2}}
	users := []models.User{
		{Id: 1, Rating: 5},
		{Id: 2, Rating: 5},
		{Id: 3, Rating: 5},
	}
	distances := []models.DistanceFromMarkToPoint{
		{MarkId: 1, UserId: 1, Distance: 1},
		{MarkId: 1, UserId: 2, Distance: 2},
		{MarkId: 1, UserId: 3, Distance: 3},
		{MarkId: 2, UserId: 1, Distance: 1},
		{MarkId: 2, UserId: 2, Distance: 2},
		{MarkId: 2, UserId: 3, Distance: 3},
	}

	tests := []struct {
		name      string
		cfg       func(*config.TaskerConfig)
		marks     method[[]models.Mark]
		users     method[[]models.User]
		tasks     method[[]models.Task]
		distances method[[]models.DistanceFromMarkToPoint]
		addTask   method[int64]
		trmErr    error
		// wantAssigned is the exact set of AddTask calls (order-insensitive).
		wantAssigned []assigned
		wantStats    usecase.TaskerStats
		wantErr      bool
	}{
		{
			name:      "assigns every free user while marks are not covered",
			marks:     method[[]models.Mark]{data: marks},
			users:     method[[]models.User]{data: users},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances},
			addTask:   method[int64]{data: 1},
			wantAssigned: []assigned{
				{1, 1}, {2, 1}, {3, 1},
				{1, 2}, {2, 2}, {3, 2},
			},
			wantStats: usecase.TaskerStats{Marks: 2, Users: 3, Candidates: 6, Assigned: 6, Covered: 0, Iterations: 4},
		},
		{
			name:      "respects the per-user limit",
			cfg:       func(c *config.TaskerConfig) { c.MaxTasksPerUser = 1 },
			marks:     method[[]models.Mark]{data: marks},
			users:     method[[]models.User]{data: users},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances},
			addTask:   method[int64]{data: 1},
			// The closest user takes mark 1, the next closest mark 2, the
			// third one goes to mark 1 again on the second round.
			wantAssigned: []assigned{{1, 1}, {2, 2}, {3, 1}},
			wantStats:    usecase.TaskerStats{Marks: 2, Users: 3, Candidates: 6, Assigned: 3, Covered: 0, Iterations: 3},
		},
		{
			name:  "counts issued tasks towards the limit and skips taken pairs",
			cfg:   func(c *config.TaskerConfig) { c.MaxTasksPerUser = 1 },
			marks: method[[]models.Mark]{data: marks},
			users: method[[]models.User]{data: users},
			tasks: method[[]models.Task]{data: []models.Task{
				{ID: 1, UserID: 1, MarkID: 1, StatusID: models.UnfulfilledStatus},
				{ID: 2, UserID: 2, MarkID: 2, StatusID: models.CompletedStatus},
				{ID: 3, UserID: 3, MarkID: 2, StatusID: models.OverdueStatus},
			}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances},
			addTask:   method[int64]{data: 1},
			// User 1 is at the limit; user 2 already did mark 2, user 3 let
			// mark 2 expire — neither gets mark 2 again.
			wantAssigned: []assigned{{2, 1}, {3, 1}},
			wantStats:    usecase.TaskerStats{Marks: 2, Users: 3, Candidates: 3, Assigned: 2, Covered: 0, Iterations: 3},
		},
		{
			name:      "does nothing when marks are already covered",
			cfg:       func(c *config.TaskerConfig) { c.TargetProbability = 0 },
			marks:     method[[]models.Mark]{data: marks},
			users:     method[[]models.User]{data: users},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances},
			wantStats: usecase.TaskerStats{Marks: 2, Users: 3, Candidates: 6, Assigned: 0, Covered: 2, Iterations: 1},
		},
		{
			name:      "ignores users outside the radius",
			marks:     method[[]models.Mark]{data: marks[:1]},
			users:     method[[]models.User]{data: users},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances[:1]},
			addTask:   method[int64]{data: 1},
			wantAssigned: []assigned{
				{1, 1},
			},
			wantStats: usecase.TaskerStats{Marks: 1, Users: 3, Candidates: 1, Assigned: 1, Covered: 0, Iterations: 2},
		},
		{
			name: "never assigns a mark to its author",
			// User 1 authored mark 1: they are the closest candidate but must
			// not be asked to verify their own mark.
			marks:     method[[]models.Mark]{data: []models.Mark{{ID: 1, UserID: 1}}},
			users:     method[[]models.User]{data: users},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances[:3]},
			addTask:   method[int64]{data: 1},
			wantAssigned: []assigned{
				{2, 1}, {3, 1},
			},
			wantStats: usecase.TaskerStats{Marks: 1, Users: 3, Candidates: 2, Assigned: 2, Covered: 0, Iterations: 3},
		},
		{
			name:  "skips users present in tasks or distances but missing from users",
			marks: method[[]models.Mark]{data: marks[:1]},
			// Only user 1 is registered; users 2 and 3 still have rows in
			// tasks/distances (read outside a single transaction).
			users: method[[]models.User]{data: users[:1]},
			tasks: method[[]models.Task]{data: []models.Task{
				{ID: 1, UserID: 2, MarkID: 1, StatusID: models.UnfulfilledStatus},
				{ID: 2, UserID: 3, MarkID: 1, StatusID: models.OverdueStatus},
			}},
			distances:    method[[]models.DistanceFromMarkToPoint]{data: distances[:3]},
			addTask:      method[int64]{data: 1},
			wantAssigned: []assigned{{1, 1}},
			wantStats:    usecase.TaskerStats{Marks: 1, Users: 1, Candidates: 1, Assigned: 1, Covered: 0, Iterations: 2},
		},
		{
			name:    "marks error",
			marks:   method[[]models.Mark]{err: errors.New("db")},
			wantErr: true,
		},
		{
			name:    "users error",
			marks:   method[[]models.Mark]{data: marks},
			users:   method[[]models.User]{err: errors.New("db")},
			wantErr: true,
		},
		{
			name:    "tasks error",
			marks:   method[[]models.Mark]{data: marks},
			users:   method[[]models.User]{data: users},
			tasks:   method[[]models.Task]{err: errors.New("db")},
			wantErr: true,
		},
		{
			name:      "distances error",
			marks:     method[[]models.Mark]{data: marks},
			users:     method[[]models.User]{data: users},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{err: errors.New("db")},
			wantErr:   true,
		},
		{
			name:      "add task error aborts the transaction",
			marks:     method[[]models.Mark]{data: marks[:1]},
			users:     method[[]models.User]{data: users[:1]},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances[:1]},
			addTask:   method[int64]{err: errors.New("db")},
			wantErr:   true,
		},
		{
			name:      "transaction error",
			marks:     method[[]models.Mark]{data: marks[:1]},
			users:     method[[]models.User]{data: users[:1]},
			tasks:     method[[]models.Task]{data: []models.Task{}},
			distances: method[[]models.DistanceFromMarkToPoint]{data: distances[:1]},
			trmErr:    errors.New("tx"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			cfg := testTaskerConfig()
			if tt.cfg != nil {
				tt.cfg(&cfg)
			}
			uc, m := suite.newTasker(cfg)

			m.marks.On("GetMarks", mock.Anything, models.GetMarksFilters{
				MarkStatusIds: []int{int(models.UnconfirmedStatus), int(models.UnderReviewStatus)},
			}).Once().Return(models.Page[models.Mark]{Items: tt.marks.data, Total: len(tt.marks.data)}, tt.marks.err)

			if tt.marks.err == nil {
				m.users.On("GetUsers", mock.Anything, models.Pagination{}).Once().Return(models.Page[models.User]{Items: tt.users.data, Total: len(tt.users.data)}, tt.users.err)
			}
			if tt.marks.err == nil && tt.users.err == nil {
				m.tasks.On("GetTasks", mock.Anything, models.GetTasksFilters{
					Statuses: []int{int(models.UnfulfilledStatus), int(models.CompletedStatus), int(models.OverdueStatus)},
				}).Once().Return(models.Page[models.Task]{Items: tt.tasks.data, Total: len(tt.tasks.data)}, tt.tasks.err)
			}
			if tt.marks.err == nil && tt.users.err == nil && tt.tasks.err == nil {
				m.marks.On("GetDistancesFromMarkToPoint", mock.Anything, models.GetDistanceFromMarkToPointFilters{
					MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus, models.UnderReviewStatus},
					MaxRadius:     cfg.MaxRadiusMeters,
				}).Once().Return(tt.distances.data, tt.distances.err)
			}

			var got []assigned
			if tt.distances.err == nil && tt.marks.err == nil && tt.users.err == nil && tt.tasks.err == nil {
				m.trm.On("Do", mock.Anything, mock.Anything).Once().
					Return(func(ctx context.Context, fn func(context.Context) error) error {
						if tt.trmErr != nil {
							return tt.trmErr
						}
						return fn(ctx)
					})
				if tt.trmErr == nil && (len(tt.wantAssigned) > 0 || tt.addTask.err != nil) {
					before := time.Now()
					m.tasks.On("AddTask", mock.Anything, mock.MatchedBy(func(task models.Task) bool {
						due := task.DueAt
						return due.Valid &&
							!due.Time.Before(before.Add(cfg.TaskTTL)) &&
							due.Time.Before(time.Now().Add(cfg.TaskTTL+time.Minute))
					})).Run(func(args mock.Arguments) {
						task := args.Get(1).(models.Task)
						got = append(got, assigned{userId: task.UserID, markId: task.MarkID})
					}).Return(tt.addTask.data, tt.addTask.err)
				}
			}

			stats, err := uc.Update(context.Background())

			if tt.wantErr {
				suite.Error(err)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.wantStats, stats)
			suite.ElementsMatch(tt.wantAssigned, got)
		})
	}
}

func (suite *TaskerSuite) TestExpireOverdue() {
	tests := []struct {
		name   string
		expire method[int64]
	}{
		{name: "Ok", expire: method[int64]{data: 7}},
		{name: "Err", expire: method[int64]{err: errors.New("db")}},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			uc, m := suite.newTasker(testTaskerConfig())

			before := time.Now()
			m.tasks.On("ExpireOverdueTasks", mock.Anything, mock.MatchedBy(func(now time.Time) bool {
				return !now.Before(before) && !now.After(time.Now())
			})).Once().Return(tt.expire.data, tt.expire.err)

			got, err := uc.ExpireOverdue(context.Background())

			if tt.expire.err != nil {
				suite.Error(err)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.expire.data, got)
		})
	}
}
