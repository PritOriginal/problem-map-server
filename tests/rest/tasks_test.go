//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TasksSuite struct {
	suite.Suite
	Cfg *config.Config
	fx  *fixtures
}

func (st *TasksSuite) SetupSuite() {
	st.Cfg, st.fx = loadFixtures(st.T())
}

func TestTasksSuite(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}

func (st *TasksSuite) TestGetTasks() {
	response := getTasks(st.T(), &st.Cfg.REST, http.StatusOK)
	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Tasks)

	ids := make([]int, 0, len(response.Payload.Tasks))
	for _, task := range response.Payload.Tasks {
		ids = append(ids, task.ID)
	}
	st.Contains(ids, st.fx.taskID, "fixture task must be listed")
}

func getTasks(t *testing.T, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[tasksrest.GetTasksResponse] {
	resp, err := http.Get(makeUrl(makeUrlParams{
		host: cfg.Host,
		port: cfg.Port,
		path: "/tasks",
	}))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[tasksrest.GetTasksResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *TasksSuite) TestGetTaskById() {
	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(st.fx.taskID),
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Err404",
			id:         strconv.Itoa(math.MaxInt32),
			statusCode: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host: st.Cfg.REST.Host,
				port: st.Cfg.REST.Port,
				path: fmt.Sprintf("/tasks/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[tasksrest.GetTaskByIdResponse]

			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Require().NotNil(response.Payload.Task)
				st.Equal(st.fx.taskID, response.Payload.Task.ID)
				st.Equal(st.fx.markID, response.Payload.Task.MarkID)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *TasksSuite) TestGetTasksByUserId() {
	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(st.fx.user.ID),
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host: st.Cfg.REST.Host,
				port: st.Cfg.REST.Port,
				path: fmt.Sprintf("/tasks/user/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[tasksrest.GetTasksByUserIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.Tasks)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *TasksSuite) TestAddTask() {
	moderatorAccessToken := st.fx.moderatorToken
	userAccessToken := st.fx.user.AccessToken

	// A fresh mark so the fixture task on fx.markID is left untouched.
	markID := addNewMark(st.T(), &st.Cfg.REST, userAccessToken).Payload.MarkId

	tests := []struct {
		name        string
		rawReq      string
		req         tasksrest.AddTaskRequest
		accessToken string
		statusCode  int
	}{
		{
			name: "Ok201",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				MarkID: markID,
			},
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusCreated,
		},
		{
			name: "Err401NoToken",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				MarkID: markID,
			},
			statusCode: http.StatusUnauthorized,
		},
		{
			name: "Err403User",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				MarkID: markID,
			},
			accessToken: userAccessToken,
			statusCode:  http.StatusForbidden,
		},
		{
			name:        "Err400InvalidJSON",
			rawReq:      "{",
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusBadRequest,
		},
		{
			name: "Err400InvalidReq",
			req: tasksrest.AddTaskRequest{
				Name: "test",
			},
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			var request *bytes.Buffer
			if tt.rawReq == "" {
				reqJSON, err := json.Marshal(tt.req)
				st.NoError(err)
				request = bytes.NewBuffer(reqJSON)
			} else {
				request = bytes.NewBuffer([]byte(tt.rawReq))
			}

			response := addTask(st.T(), &st.Cfg.REST, request, tt.accessToken, tt.statusCode)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotZero(response.Payload.TaskId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}
