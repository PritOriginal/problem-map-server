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
	suite.uc = usecase.NewTasks(suite.log, suite.tasksRepo)
}

func TestTasks(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}

func (suite *TasksSuite) TestGetTasks() {
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
			tasksRepoCall := suite.tasksRepo.On("GetTasks", mock.Anything).Once()
			if !tt.wantErr {
				tasksRepoCall.Return([]models.Task{}, nil)
			} else {
				tasksRepoCall.Return([]models.Task{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetTasks(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *TasksSuite) TestGetTaskById() {
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
			tasksRepoCall := suite.tasksRepo.On("GetTaskById", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				tasksRepoCall.Return(models.Task{}, nil)
			} else {
				tasksRepoCall.Return(models.Task{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetTaskById(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *TasksSuite) TestGetTasksByUserId() {
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
			tasksRepoCall := suite.tasksRepo.On("GetTasksByUserId", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				tasksRepoCall.Return([]models.Task{}, nil)
			} else {
				tasksRepoCall.Return([]models.Task{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetTasksByUserId(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *TasksSuite) TestAddTask() {
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
			tasksRepoCall := suite.tasksRepo.On("AddTask", mock.Anything, mock.Anything).Once()
			if !tt.wantErr {
				tasksRepoCall.Return(int64(1), nil)
			} else {
				tasksRepoCall.Return(int64(0), errors.New(""))
			}

			_, gotErr := suite.uc.AddTask(context.Background(), models.Task{})

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
