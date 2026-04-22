//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
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
}

func (st *TasksSuite) SetupSuite() {
	st.Cfg = config.MustLoadPath("../../configs/config.yaml")
}

func TestTasksSuite(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}

func (st *TasksSuite) TestGetTasks() {
	response := getTasks(st.T(), &st.Cfg.REST, http.StatusOK)
	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Tasks)
}

func getTasks(t *testing.T, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[tasksrest.GetTasksResponse] {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/tasks", cfg.Host, cfg.Port))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var response responses.Response[tasksrest.GetTasksResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *TasksSuite) TestGetTaskById() {
	getTasksResponse := getTasks(st.T(), &st.Cfg.REST, http.StatusOK)

	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(getTasksResponse.Payload.Tasks[0].ID),
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
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/tasks/%s",
				st.Cfg.REST.Host,
				st.Cfg.REST.Port,
				tt.id,
			))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[tasksrest.GetTaskByIdResponse]

			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.Task)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *TasksSuite) TestGetTasksByUserId() {
	getUsersResponse := getUsers(st.T(), &st.Cfg.REST, http.StatusOK)

	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(getUsersResponse.Payload.Users[0].Id),
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
			resp, err := http.Get(
				fmt.Sprintf("http://%s:%d/tasks/user/%s",
					st.Cfg.REST.Host,
					st.Cfg.REST.Port,
					tt.id,
				))
			st.NoError(err)
			defer resp.Body.Close()

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
	getUsersResponse := getUsers(st.T(), &st.Cfg.REST, http.StatusOK)
	userIndex := rand.Intn(len(getUsersResponse.Payload.Users))
	user := getUsersResponse.Payload.Users[userIndex]

	getMarksResponse := getMarks(st.T(), &st.Cfg.REST, http.StatusOK)
	markIndex := rand.Intn(len(getMarksResponse.Payload.Marks))
	mark := getMarksResponse.Payload.Marks[markIndex]

	tests := []struct {
		name       string
		rawReq     string
		req        tasksrest.AddTaskRequest
		statusCode int
	}{
		{
			name: "Ok201",
			req: tasksrest.AddTaskRequest{
				Name:   "test",
				UserID: user.Id,
				MarkID: mark.ID,
			},
			statusCode: http.StatusCreated,
		},
		{
			name:       "Err400InvalidJSON",
			rawReq:     "{",
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err400InvalidReq",
			req: tasksrest.AddTaskRequest{
				Name: "test",
			},
			statusCode: http.StatusBadRequest,
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

			resp, err := http.Post(
				fmt.Sprintf("http://%s:%d/tasks", st.Cfg.REST.Host, st.Cfg.REST.Port),
				"application/json",
				request,
			)
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[tasksrest.AddTaskResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.TaskId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}
