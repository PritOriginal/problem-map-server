package tasksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type TasksSuite struct {
	suite.Suite
	r  *chi.Mux
	uc *tasksrest.MockTasks
}

func (suite *TasksSuite) SetupSuite() {
	suite.uc = tasksrest.NewMockTasks(suite.T())

	log := slogdiscard.NewDiscardLogger()
	validate := validator.New()
	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	suite.r = chi.NewRouter()

	tasksrest.Register(suite.r, suite.uc, baseHandler)
}

func TestUsers(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}

func (suite *TasksSuite) TestGetTasks() {
	tests := []struct {
		name        string
		errGetTasks error
		statusCode  int
	}{
		{
			name:        "Ok200",
			errGetTasks: nil,
			statusCode:  200,
		},
		{
			name:        "Err500",
			errGetTasks: errors.New(""),
			statusCode:  500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetTasks", mock.Anything).Once().
				Return([]models.Task{}, tt.errGetTasks)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/tasks", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *TasksSuite) TestGetTaskById() {
	tests := []struct {
		name           string
		id             string
		wantErrParseId bool
		errGetTaskById error
		statusCode     int
	}{
		{
			name:           "Ok200",
			id:             "1",
			wantErrParseId: false,
			errGetTaskById: nil,
			statusCode:     200,
		},
		{
			name:           "Err400",
			id:             "a",
			wantErrParseId: true,
			errGetTaskById: nil,
			statusCode:     400,
		},
		{
			name:           "Err404",
			id:             "1",
			wantErrParseId: false,
			errGetTaskById: storage.ErrNotFound,
			statusCode:     404,
		},
		{
			name:           "Err500",
			id:             "1",
			wantErrParseId: false,
			errGetTaskById: errors.New(""),
			statusCode:     500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetTaskById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.Task{}, tt.errGetTaskById)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/tasks/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *TasksSuite) TestGetTasksByUserId() {
	tests := []struct {
		name                string
		id                  string
		wantErrParseId      bool
		errGetTasksByUserId error
		statusCode          int
	}{
		{
			name:                "Ok200",
			id:                  "1",
			wantErrParseId:      false,
			errGetTasksByUserId: nil,
			statusCode:          200,
		},
		{
			name:                "Err400",
			id:                  "a",
			wantErrParseId:      true,
			errGetTasksByUserId: nil,
			statusCode:          400,
		},
		{
			name:                "Err500",
			id:                  "1",
			wantErrParseId:      false,
			errGetTasksByUserId: errors.New(""),
			statusCode:          500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetTasksByUserId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return([]models.Task{}, tt.errGetTasksByUserId)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/tasks/user/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *TasksSuite) TestAddTask() {
	tests := []struct {
		name            string
		rawReq          string
		req             tasksrest.AddTaskRequest
		wantErrParseReq bool
		errAddTask      error
		statusCode      int
	}{
		{
			name: "Ok201",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 1,
				MarkID: 1,
			},
			wantErrParseReq: false,
			errAddTask:      nil,
			statusCode:      201,
		},
		{
			name:            "Err400InvalidJSON",
			rawReq:          "{",
			wantErrParseReq: true,
			errAddTask:      nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq",
			req: tasksrest.AddTaskRequest{
				Name: "test",
			},
			wantErrParseReq: true,
			errAddTask:      nil,
			statusCode:      400,
		},
		{
			name: "Err500",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 1,
				MarkID: 1,
			},
			wantErrParseReq: false,
			errAddTask:      errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseReq {
				suite.uc.On("AddTask", mock.Anything, mock.Anything).Once().
					Return(int64(1), tt.errAddTask)
			}

			w := httptest.NewRecorder()

			var buf *bytes.Buffer
			if tt.rawReq == "" {
				body, err := json.Marshal(tt.req)
				suite.NoError(err)
				buf = bytes.NewBuffer(body)
			} else {
				buf = bytes.NewBuffer([]byte(tt.rawReq))
			}

			req := httptest.NewRequest("POST", "/tasks", buf)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}
