//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	checksrest "github.com/PritOriginal/problem-map-server/internal/handler/checks"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ChecksSuite struct {
	suite.Suite
	Cfg *config.Config
}

func (st *ChecksSuite) SetupSuite() {
	st.Cfg = config.MustLoadPath("../../configs/config.yaml")
}

func TestChecksSuite(t *testing.T) {
	suite.Run(t, new(ChecksSuite))
}

func (st *ChecksSuite) TestGetCheckById() {
	signInResponse := addNewUser(st.T(), &st.Cfg.REST)
	addCheckResponse := addNewCheck(st.T(), &st.Cfg.REST, signInResponse.Payload.AccessToken)

	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(addCheckResponse.Payload.CheckId),
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Err404",
			id:         "1",
			statusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/checks/%s", st.Cfg.REST.Host, st.Cfg.REST.Port, tt.id))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[checksrest.GetCheckByIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.Check)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *ChecksSuite) TestGetChecksByMarkId() {
	getMarksResponse := getMarks(st.T(), &st.Cfg.REST, "", http.StatusOK)

	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(getMarksResponse.Payload.Marks[0].ID),
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
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/checks/mark/%s", st.Cfg.REST.Host, st.Cfg.REST.Port, tt.id))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[checksrest.GetChecksByMarkIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.NotNil(response.Payload.Checks)
			} else {
			}
		})
	}
}

func (st *ChecksSuite) TestGetChecksByUserId() {
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
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/checks/user/%s", st.Cfg.REST.Host, st.Cfg.REST.Port, tt.id))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[checksrest.GetChecksByUserIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.Checks)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *ChecksSuite) TestAddCheck() {
	signInResponse := addNewUser(st.T(), &st.Cfg.REST)
	getMarksResponse := getMarks(st.T(), &st.Cfg.REST, "", http.StatusOK)
	randomMarkIndex := rand.Intn(len(getMarksResponse.Payload.Marks))
	randomMark := getMarksResponse.Payload.Marks[randomMarkIndex]

	tests := []struct {
		name       string
		req        checksrest.AddCheckRequest
		statusCode int
	}{
		{
			name: "Ok201",
			req: checksrest.AddCheckRequest{
				MarkID:  randomMark.ID,
				Result:  true,
				Comment: "",
			},
			statusCode: 201,
		},
		{
			name: "Err400InvalidReq",
			req: checksrest.AddCheckRequest{
				Result:  true,
				Comment: "",
			},
			statusCode: 400,
		},
		{
			name: "Err400NotFoundMark",
			req: checksrest.AddCheckRequest{
				MarkID:  1,
				Result:  true,
				Comment: "",
			},
			statusCode: 400,
		},
		{
			name: "Err409Conflict",
			req: checksrest.AddCheckRequest{
				MarkID:  randomMark.ID,
				Result:  true,
				Comment: "",
			},
			statusCode: 409,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			response := addCheck(
				st.T(),
				&st.Cfg.REST,
				tt.req,
				signInResponse.Payload.AccessToken,
				tt.statusCode,
			)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.CheckId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func addNewCheck(t *testing.T, cfg *config.RESTConfig, accessToken string) responses.Response[checksrest.AddCheckResponse] {
	getMarksResponse := getMarks(t, cfg, "", http.StatusOK)
	randomMarkIndex := rand.Intn(len(getMarksResponse.Payload.Marks))
	randomMark := getMarksResponse.Payload.Marks[randomMarkIndex]

	return addCheck(
		t,
		cfg,
		checksrest.AddCheckRequest{
			MarkID:  randomMark.ID,
			Result:  gofakeit.Bool(),
			Comment: "",
		},
		accessToken, http.StatusCreated,
	)
}

func addCheck(t *testing.T, cfg *config.RESTConfig, request checksrest.AddCheckRequest, accessToken string, expectedStatusCode int) responses.Response[checksrest.AddCheckResponse] {
	b := &bytes.Buffer{}
	mpw := multipart.NewWriter(b)
	mpw.WriteField("mark_id", strconv.Itoa(request.MarkID))
	mpw.WriteField("result", strconv.FormatBool(request.Result))
	mpw.WriteField("comment", request.Comment)

	image := gofakeit.ImageJpeg(10, 10)
	fw, err := mpw.CreateFormFile("photos", "test.jpg")
	require.NoError(t, err)
	io.Copy(fw, bytes.NewBuffer(image))

	mpw.Close()

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/checks", cfg.Host, cfg.Port), b)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", mpw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[checksrest.AddCheckResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}
