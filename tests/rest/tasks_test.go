//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/tests/rest/suite"
	"github.com/stretchr/testify/require"
)

func TestGetTasks(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/tasks", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[tasksrest.GetTasksResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Tasks)
}

func TestGetTaskById(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/tasks/1000", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[tasksrest.GetTaskByIdResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Task)
}

func TestGetTasksByUserId(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/tasks/user/1", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[tasksrest.GetTasksByUserIdResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Tasks)
}

func TestAddTask(t *testing.T) {
	st := suite.New(t)

	task := models.Task{
		Name:   "",
		UserID: 1,
		MarkID: 1,
	}

	reqJSON, err := json.Marshal(task)
	require.NoError(t, err)

	resp, err := http.Post(
		fmt.Sprintf("http://%s:%d/tasks", st.Cfg.REST.Host, st.Cfg.REST.Port),
		"application/json",
		bytes.NewBuffer(reqJSON),
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	defer resp.Body.Close()
}
