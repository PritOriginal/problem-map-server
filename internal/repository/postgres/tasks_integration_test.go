//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

// Seeded tasks (see seed()):
//
//	1: Alice, mark 1, unfulfilled
//	2: Alice, mark 2, completed
//	3: Bob,   mark 3, unfulfilled
const (
	fxTaskAliceMark1 = 1
	fxTaskAliceMark2 = 2
	fxTaskBobMark3   = 3
)

func taskIDs(tasks []models.Task) []int {
	return ids(tasks, func(t models.Task) int { return t.ID })
}

func (s *PostgresSuite) TestTasks_GetTasks() {
	tests := []struct {
		name    string
		filters models.GetTasksFilters
		wantIDs []int
	}{
		{name: "all tasks", wantIDs: []int{fxTaskAliceMark1, fxTaskAliceMark2, fxTaskBobMark3}},
		{name: "unfulfilled only", filters: models.GetTasksFilters{Statuses: []int{int(models.UnfulfilledStatus)}}, wantIDs: []int{fxTaskAliceMark1, fxTaskBobMark3}},
		{name: "completed only", filters: models.GetTasksFilters{Statuses: []int{int(models.CompletedStatus)}}, wantIDs: []int{fxTaskAliceMark2}},
		{name: "both statuses", filters: models.GetTasksFilters{Statuses: []int{1, 2}}, wantIDs: []int{fxTaskAliceMark1, fxTaskAliceMark2, fxTaskBobMark3}},
		{name: "unknown status", filters: models.GetTasksFilters{Statuses: []int{99}}, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.tasks.GetTasks(s.ctx, tt.filters)
			s.Require().NoError(err)
			tasks := page.Items
			s.NotNil(tasks)
			s.Equal(len(tt.wantIDs), page.Total)
			s.ElementsMatch(tt.wantIDs, taskIDs(tasks))
			for _, t := range tasks {
				s.False(t.CreatedAt.IsZero())
				s.False(t.UpdatedAt.IsZero())
			}
		})
	}
}

func (s *PostgresSuite) TestTasks_GetTaskById() {
	tests := []struct {
		name    string
		id      int
		want    models.Task
		wantErr error
	}{
		{
			name: "existing task",
			id:   fxTaskAliceMark2,
			want: models.Task{ID: fxTaskAliceMark2, Name: "Проверить лавку", UserID: fxUserAlice, MarkID: fxMarkInside, StatusID: models.CompletedStatus},
		},
		{name: "missing task", id: 404, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.tasks.GetTaskById(s.ctx, tt.id)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.want.ID, got.ID)
			s.Equal(tt.want.Name, got.Name)
			s.Equal(tt.want.UserID, got.UserID)
			s.Equal(tt.want.MarkID, got.MarkID)
			s.Equal(tt.want.StatusID, got.StatusID)
		})
	}
}

func (s *PostgresSuite) TestTasks_GetTasksByUserId() {
	tests := []struct {
		name    string
		userID  int
		filters models.GetTasksByUserIdFilters
		wantIDs []int
	}{
		{name: "all alice tasks", userID: fxUserAlice, wantIDs: []int{fxTaskAliceMark1, fxTaskAliceMark2}},
		{name: "alice completed", userID: fxUserAlice, filters: models.GetTasksByUserIdFilters{Statuses: []int{int(models.CompletedStatus)}}, wantIDs: []int{fxTaskAliceMark2}},
		{name: "bob unfulfilled", userID: fxUserBob, filters: models.GetTasksByUserIdFilters{Statuses: []int{int(models.UnfulfilledStatus)}}, wantIDs: []int{fxTaskBobMark3}},
		{name: "bob completed is empty", userID: fxUserBob, filters: models.GetTasksByUserIdFilters{Statuses: []int{int(models.CompletedStatus)}}, wantIDs: []int{}},
		{name: "unknown user", userID: 999, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.tasks.GetTasksByUserId(s.ctx, tt.userID, tt.filters)
			s.Require().NoError(err)
			tasks := page.Items
			s.NotNil(tasks)
			s.Equal(len(tt.wantIDs), page.Total)
			s.ElementsMatch(tt.wantIDs, taskIDs(tasks))
		})
	}
}

func (s *PostgresSuite) TestTasks_GetTaskByUserIdAndMarkId() {
	tests := []struct {
		name    string
		userID  int
		markID  int
		status  models.TaskStatusType
		wantID  int
		wantErr error
	}{
		{name: "match", userID: fxUserBob, markID: fxMarkFar, status: models.UnfulfilledStatus, wantID: fxTaskBobMark3},
		{name: "completed task", userID: fxUserAlice, markID: fxMarkInside, status: models.CompletedStatus, wantID: fxTaskAliceMark2},
		{name: "other status", userID: fxUserBob, markID: fxMarkFar, status: models.CompletedStatus, wantErr: repository.ErrNotFound},
		{name: "user has no task for mark", userID: fxUserBob, markID: fxMarkNear, status: models.UnfulfilledStatus, wantErr: repository.ErrNotFound},
		{name: "unknown mark", userID: fxUserAlice, markID: 404, status: models.UnfulfilledStatus, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.tasks.GetTaskByUserIdAndMarkId(s.ctx, tt.userID, tt.markID, tt.status)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.wantID, got.ID)
		})
	}
}

func (s *PostgresSuite) TestTasks_AddTask() {
	tests := []struct {
		name    string
		task    models.Task
		wantErr bool
	}{
		{name: "task gets default unfulfilled status", task: models.Task{Name: "Сходить", UserID: fxUserBob, MarkID: fxMarkNear}},
		{name: "unknown user violates foreign key", task: models.Task{Name: "x", UserID: 999, MarkID: fxMarkNear}, wantErr: true},
		{name: "unknown mark violates foreign key", task: models.Task{Name: "x", UserID: fxUserBob, MarkID: 999}, wantErr: true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			id, err := s.tasks.AddTask(s.ctx, tt.task)
			if tt.wantErr {
				s.Error(err)
				return
			}
			s.Require().NoError(err)
			s.Greater(id, int64(fxTaskBobMark3))

			got, err := s.tasks.GetTaskById(s.ctx, int(id))
			s.Require().NoError(err)
			s.Equal(tt.task.Name, got.Name)
			s.Equal(tt.task.UserID, got.UserID)
			s.Equal(tt.task.MarkID, got.MarkID)
			s.Equal(models.UnfulfilledStatus, got.StatusID)
			s.WithinDuration(time.Now(), got.CreatedAt, time.Minute)
		})
	}
}

func (s *PostgresSuite) TestTasks_UpdateTaskStatus() {
	tests := []struct {
		name    string
		taskID  int
		status  models.TaskStatusType
		wantErr bool
	}{
		{name: "complete a task", taskID: fxTaskAliceMark1, status: models.CompletedStatus},
		{name: "reopen a task", taskID: fxTaskAliceMark2, status: models.UnfulfilledStatus},
		{name: "unknown status violates foreign key", taskID: fxTaskBobMark3, status: models.TaskStatusType(99), wantErr: true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := s.tasks.UpdateTaskStatus(s.ctx, tt.taskID, tt.status)
			if tt.wantErr {
				s.Error(err)
				return
			}
			s.Require().NoError(err)

			got, err := s.tasks.GetTaskById(s.ctx, tt.taskID)
			s.Require().NoError(err)
			s.Equal(tt.status, got.StatusID)
		})
	}
}

func (s *PostgresSuite) TestTasks_UpdateTaskStatus_UnknownTask() {
	// UPDATE of a missing row is a no-op for the repository.
	s.NoError(s.tasks.UpdateTaskStatus(s.ctx, 404, models.CompletedStatus))
	_, err := s.tasks.GetTaskById(s.ctx, 404)
	s.ErrorIs(err, repository.ErrNotFound)
}
