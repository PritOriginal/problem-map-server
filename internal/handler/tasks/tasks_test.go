package tasksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type TasksSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *tasksrest.MockTasks
}

func (suite *TasksSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Key: []byte("1234"),
	})
	if err != nil {
		panic(err)
	}
	if err := authMiddleware.MiddlewareInit(); err != nil {
		panic(err)
	}

	suite.uc = tasksrest.NewMockTasks(suite.T())

	log := slogdiscard.NewDiscardLogger()

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	tasksrest.Register(suite.r, log, authMiddleware, suite.uc)
}

func TestUsers(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}

func (suite *TasksSuite) TestGetTasks() {
	tests := []struct {
		name                 string
		query                string
		wantErrParseStatuses bool
		errGetTasks          error
		statusCode           int
	}{
		{
			name:        "Ok200",
			errGetTasks: nil,
			statusCode:  200,
		},
		{
			name:        "Ok200",
			query:       "?statuses=1",
			errGetTasks: nil,
			statusCode:  200,
		},
		{
			name:        "Ok200",
			query:       "?statuses=1,2",
			errGetTasks: nil,
			statusCode:  200,
		},
		{
			name:                 "Err400",
			query:                "?statuses=a",
			wantErrParseStatuses: true,
			statusCode:           400,
		},
		{
			name:       "Ok200Pagination",
			query:      "?limit=10&offset=20",
			statusCode: 200,
		},
		{
			name:                 "Err400LimitTooBig",
			query:                "?limit=501",
			wantErrParseStatuses: true,
			statusCode:           400,
		},
		{
			name:        "Err500",
			errGetTasks: errors.New(""),
			statusCode:  500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseStatuses {
				suite.uc.On("ListTasks", mock.Anything, mock.AnythingOfType("models.GetTasksFilters")).Once().
					Return(models.Page[models.Task]{Items: []models.Task{}}, tt.errGetTasks)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/tasks"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
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
			errGetTaskById: usecase.ErrNotFound,
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *TasksSuite) TestGetTasksByUserId() {
	tests := []struct {
		name                 string
		id                   string
		query                string
		wantErrParseId       bool
		wantErrParseStatuses bool
		errGetTasksByUserId  error
		statusCode           int
	}{
		{
			name:                "Ok200",
			id:                  "1",
			errGetTasksByUserId: nil,
			statusCode:          200,
		},
		{
			name:       "Ok200",
			id:         "1",
			query:      "?statuses=1",
			statusCode: 200,
		},
		{
			name:       "Ok200",
			id:         "1",
			query:      "?statuses=1,2",
			statusCode: 200,
		},
		{
			name:                 "Err400",
			id:                   "1",
			query:                "?statuses=a",
			wantErrParseStatuses: true,
			statusCode:           400,
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
			if !tt.wantErrParseId && !tt.wantErrParseStatuses {
				suite.uc.On("ListTasksByUserId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("models.GetTasksByUserIdFilters")).Once().
					Return(models.Page[models.Task]{Items: []models.Task{}}, tt.errGetTasksByUserId)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/tasks/user/"+tt.id+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *TasksSuite) TestAddTask() {
	tests := []struct {
		name            string
		rawReq          string
		req             tasksrest.AddTaskRequest
		role            models.Role
		noToken         bool
		wantErrParseReq bool
		errAddTask      error
		statusCode      int
	}{
		{
			name: "Ok201Moderator",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 42,
				MarkID: 1,
			},
			role:            models.RoleModerator,
			wantErrParseReq: false,
			errAddTask:      nil,
			statusCode:      201,
		},
		{
			name: "Ok201Admin",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 42,
				MarkID: 1,
			},
			role:            models.RoleAdmin,
			wantErrParseReq: false,
			errAddTask:      nil,
			statusCode:      201,
		},
		{
			name: "Err401NoToken",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 42,
				MarkID: 1,
			},
			noToken:         true,
			wantErrParseReq: true,
			statusCode:      401,
		},
		{
			name: "Err403User",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 42,
				MarkID: 1,
			},
			role:            models.RoleUser,
			wantErrParseReq: true,
			statusCode:      403,
		},
		{
			name:            "Err400InvalidJSON",
			rawReq:          "{",
			wantErrParseReq: true,
			errAddTask:      nil,
			statusCode:      400,
		},
		{
			name: "Err400NoMarkId",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 42,
			},
			role:            models.RoleModerator,
			wantErrParseReq: true,
			statusCode:      400,
		},
		{
			name: "Err400NoUserId",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				MarkID: 1,
			},
			role:            models.RoleModerator,
			wantErrParseReq: true,
			statusCode:      400,
		},
		{
			name: "Err500",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: 42,
				MarkID: 1,
			},
			wantErrParseReq: false,
			errAddTask:      errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// moderatorId is the caller from the JWT; the assignee must come
			// from the request body, not from the moderator's claims.
			const moderatorId = 7
			if !tt.wantErrParseReq {
				suite.uc.On("AddTask", mock.Anything, mock.MatchedBy(func(task models.Task) bool {
					return task.UserID == tt.req.UserID && task.MarkID == tt.req.MarkID && task.Name == tt.req.Name
				})).Once().
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
			if !tt.noToken {
				role := tt.role
				if role == "" {
					role = models.RoleModerator
				}
				accessToken, err := token.CreateToken(1*time.Minute, moderatorId, string(role), "1234")
				suite.NoError(err)
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}
