package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type TasksSuite struct {
	suite.Suite
	uc        *usecase.Tasks
	log       *slog.Logger
	tasksRepo *usecase.MockTasksRepository
}

func (suite *TasksSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.tasksRepo = usecase.NewMockTasksRepository(suite.T())
	suite.uc = usecase.NewTasks(suite.log, usecase.TasksRepositories{
		Tasks: suite.tasksRepo,
	})
}

func TestTasks(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}

func (suite *TasksSuite) TestGetTasks() {
	tests := []struct {
		name     string
		getTasks method[[]models.Task]
	}{
		{
			name: "Ok",
			getTasks: method[[]models.Task]{
				data: []models.Task{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getTasks: method[[]models.Task]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.tasksRepo.On("GetTasks", mock.Anything).Once().
					Return(tt.getTasks.data, tt.getTasks.err)
				if tt.getTasks.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetTasks(context.Background())

			if tt.getTasks.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.tasksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *TasksSuite) TestGetTaskById() {
	tests := []struct {
		name        string
		getTaskById method[models.Task]
	}{
		{
			name: "Ok",
			getTaskById: method[models.Task]{
				data: models.Task{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getTaskById: method[models.Task]{
				data: models.Task{},
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.tasksRepo.On("GetTaskById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getTaskById.data, tt.getTaskById.err)
				if tt.getTaskById.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetTaskById(context.Background(), 1)

			if tt.getTaskById.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.tasksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *TasksSuite) TestGetTasksByUserId() {
	tests := []struct {
		name             string
		getTasksByUserId method[[]models.Task]
	}{
		{
			name: "Ok",
			getTasksByUserId: method[[]models.Task]{
				data: []models.Task{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getTasksByUserId: method[[]models.Task]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.tasksRepo.On("GetTasksByUserId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getTasksByUserId.data, tt.getTasksByUserId.err)
				if tt.getTasksByUserId.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetTasksByUserId(context.Background(), 1)

			if tt.getTasksByUserId.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.tasksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *TasksSuite) TestAddTask() {
	tests := []struct {
		name    string
		addTask method[int64]
	}{
		{
			name: "Ok",
			addTask: method[int64]{
				data: int64(1),
				err:  nil,
			},
		},
		{
			name: "Err",
			addTask: method[int64]{
				data: int64(0),
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.tasksRepo.On("AddTask", mock.Anything, mock.Anything).Once().
					Return(tt.addTask.data, tt.addTask.err)
				if tt.addTask.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.AddTask(context.Background(), models.Task{})

			if tt.addTask.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.tasksRepo.AssertExpectations(suite.T())
		})
	}
}
