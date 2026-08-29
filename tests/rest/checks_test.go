//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	checksrest "github.com/PritOriginal/problem-map-server/internal/handler/checks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ChecksSuite struct {
	suite.Suite
	Cfg *config.Config
	fx  *fixtures
}

func (st *ChecksSuite) SetupSuite() {
	st.Cfg, st.fx = loadFixtures(st.T())
}

func TestChecksSuite(t *testing.T) {
	suite.Run(t, new(ChecksSuite))
}

func (st *ChecksSuite) TestGetCheckById() {
	checker := addNewUser(st.T(), &st.Cfg.REST)
	addCheckResponse := addNewCheck(st.T(), &st.Cfg.REST, st.fx.markID, checker.Payload.AccessToken)

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
			id:         strconv.Itoa(math.MaxInt32),
			statusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host: st.Cfg.REST.Host,
				port: st.Cfg.REST.Port,
				path: fmt.Sprintf("/checks/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[checksrest.GetCheckByIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Require().NotNil(response.Payload.Check)
				st.Equal(addCheckResponse.Payload.CheckId, response.Payload.Check.ID)
				st.Equal(st.fx.markID, response.Payload.Check.MarkID)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *ChecksSuite) TestGetChecksByMarkId() {
	checker := addNewUser(st.T(), &st.Cfg.REST)
	markID := addNewMark(st.T(), &st.Cfg.REST, st.fx.user.AccessToken, st.fx.markTypeID).Payload.MarkId
	checkID := addNewCheck(st.T(), &st.Cfg.REST, markID, checker.Payload.AccessToken).Payload.CheckId

	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(markID),
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
				path: fmt.Sprintf("/checks/mark/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[checksrest.GetChecksByMarkIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				// The mark author's own check is created together with the
				// mark, so the list holds it plus the checker's one.
				for _, check := range response.Payload.Checks {
					st.Equal(markID, check.MarkID)
				}
				st.Contains(ids(response.Payload.Checks, func(c models.Check) int { return c.ID }), checkID)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *ChecksSuite) TestGetChecksByUserId() {
	checker := addNewUser(st.T(), &st.Cfg.REST)
	checkerID := currentUserId(st.T(), &st.Cfg.REST, checker.Payload.AccessToken)
	markID := addNewMark(st.T(), &st.Cfg.REST, st.fx.user.AccessToken, st.fx.markTypeID).Payload.MarkId
	checkID := addNewCheck(st.T(), &st.Cfg.REST, markID, checker.Payload.AccessToken).Payload.CheckId

	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(checkerID),
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
				path: fmt.Sprintf("/checks/user/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[checksrest.GetChecksByUserIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Require().Len(response.Payload.Checks, 1)
				st.Equal(checkID, response.Payload.Checks[0].ID)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *ChecksSuite) TestAddCheck() {
	signInResponse := addNewUser(st.T(), &st.Cfg.REST)
	markID := addNewMark(st.T(), &st.Cfg.REST, st.fx.user.AccessToken, st.fx.markTypeID).Payload.MarkId

	tests := []struct {
		name       string
		req        checksrest.AddCheckRequest
		statusCode int
	}{
		{
			name: "Ok201",
			req: checksrest.AddCheckRequest{
				MarkID:  markID,
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
				MarkID:  math.MaxInt32,
				Result:  true,
				Comment: "",
			},
			statusCode: 400,
		},
		{
			name: "Err409Conflict",
			req: checksrest.AddCheckRequest{
				MarkID:  markID,
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
				st.NotZero(response.Payload.CheckId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

// addNewCheck leaves a positive check on markID on behalf of accessToken.
func addNewCheck(t *testing.T, cfg *config.RESTConfig, markID int, accessToken string) responses.Response[checksrest.AddCheckResponse] {
	t.Helper()

	return addCheck(
		t,
		cfg,
		checksrest.AddCheckRequest{
			MarkID:  markID,
			Result:  true,
			Comment: "functional test check",
		},
		accessToken, http.StatusCreated,
	)
}

func addCheck(t *testing.T, cfg *config.RESTConfig, request checksrest.AddCheckRequest, accessToken string, expectedStatusCode int) responses.Response[checksrest.AddCheckResponse] {
	b := &bytes.Buffer{}
	mpw := multipart.NewWriter(b)
	require.NoError(t, mpw.WriteField("mark_id", strconv.Itoa(request.MarkID)))
	require.NoError(t, mpw.WriteField("result", strconv.FormatBool(request.Result)))
	require.NoError(t, mpw.WriteField("comment", request.Comment))

	attachPhotos(t, mpw, 2)
	require.NoError(t, mpw.Close())

	req, err := http.NewRequest(
		http.MethodPost,
		makeUrl(makeUrlParams{
			host: cfg.Host,
			port: cfg.Port,
			path: "/checks",
		}),
		b,
	)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", mpw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[checksrest.AddCheckResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}
