//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MarksSuite struct {
	suite.Suite
	Cfg *config.Config
	fx  *fixtures
}

func (st *MarksSuite) SetupSuite() {
	st.Cfg, st.fx = loadFixtures(st.T())
}

func TestMarksSuite(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func (st *MarksSuite) TestGetMarks() {
	tests := []struct {
		name       string
		query      string
		statusCode int
	}{
		{
			name:       "Ok200",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "mark_type_ids=1",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "mark_type_ids=1,2",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "mark_type_ids=1,2&mark_status_ids=1",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "mark_type_ids=1,2&mark_status_ids=1,2",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok400",
			query:      "mark_type_ids=a",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Ok400",
			query:      "mark_status_ids=a",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			response := getMarks(st.T(), &st.Cfg.REST, tt.query, tt.statusCode)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.Marks)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func getMarks(t *testing.T, cfg *config.RESTConfig, query string, expectedStatusCode int) responses.Response[marksrest.GetMarksResponse] {
	resp, err := http.Get(makeUrl(makeUrlParams{
		host:  cfg.Host,
		port:  cfg.Port,
		path:  "/marks",
		query: query,
	}))

	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[marksrest.GetMarksResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *MarksSuite) TestGetMarkById() {
	tests := []struct {
		name       string
		id         string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(st.fx.markID),
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok400",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Ok404",
			id:         strconv.Itoa(math.MaxInt32),
			statusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host: st.Cfg.REST.Host,
				port: st.Cfg.REST.Port,
				path: fmt.Sprintf("/marks/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.GetMarkByIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Require().NotNil(response.Payload.Mark)
				st.Equal(st.fx.markID, response.Payload.Mark.ID)
				st.Equal(st.fx.user.ID, response.Payload.Mark.UserID)
				st.Equal(st.fx.markTypeID, response.Payload.Mark.MarkTypeID)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MarksSuite) TestGetMarkByUserId() {
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
			name:       "Ok400",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host: st.Cfg.REST.Host,
				port: st.Cfg.REST.Port,
				path: fmt.Sprintf("/marks/user/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.GetMarksByUserIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				ids := make([]int, 0, len(response.Payload.Marks))
				for _, m := range response.Payload.Marks {
					ids = append(ids, m.ID)
					st.Equal(st.fx.user.ID, m.UserID)
				}
				st.Contains(ids, st.fx.markID)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MarksSuite) TestAddMark() {
	accessToken := st.fx.user.AccessToken
	markTypeID := st.fx.markTypeID
	long, lat := fixtureMarkPoint.X(), fixtureMarkPoint.Y()

	tests := []struct {
		name       string
		req        marksrest.AddMarkRequest
		statusCode int
	}{
		{
			name: "Ok201",
			req: marksrest.AddMarkRequest{
				Longitude:   long,
				Latitude:    lat,
				MarkTypeID:  markTypeID,
				Description: "functional test mark",
			},
			statusCode: http.StatusCreated,
		},
		{
			name: "Err400InvalidReq-1",
			req: marksrest.AddMarkRequest{
				Longitude: 42,
				Latitude:  52,
			},
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err400InvalidReq-2",
			req: marksrest.AddMarkRequest{
				Longitude:   42,
				MarkTypeID:  1,
				Description: "",
			},
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err400InvalidReq-3",
			req: marksrest.AddMarkRequest{
				Longitude:   42,
				Latitude:    52,
				MarkTypeID:  1,
				Description: strings.Repeat("A", 257),
			},
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			b := &bytes.Buffer{}
			mpw := multipart.NewWriter(b)
			st.Require().NoError(mpw.WriteField("longitude", strconv.FormatFloat(tt.req.Longitude, 'f', -1, 64)))
			st.Require().NoError(mpw.WriteField("latitude", strconv.FormatFloat(tt.req.Latitude, 'f', -1, 64)))
			st.Require().NoError(mpw.WriteField("mark_type_id", strconv.Itoa(tt.req.MarkTypeID)))
			st.Require().NoError(mpw.WriteField("description", tt.req.Description))

			for _, image := range getImages(2) {
				fw, err := mpw.CreateFormFile("photos", "test.jpg")
				st.Require().NoError(err)
				_, err = io.Copy(fw, bytes.NewBuffer(image))
				st.Require().NoError(err)
			}

			st.Require().NoError(mpw.Close())

			response := addMark(
				st.T(),
				&st.Cfg.REST,
				b,
				mpw.FormDataContentType(),
				accessToken,
				tt.statusCode,
			)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotZero(response.Payload.MarkId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

// addNewMark creates an unconfirmed mark of the fixture type at the fixture
// point on behalf of accessToken.
func addNewMark(t *testing.T, cfg *config.RESTConfig, accessToken string) responses.Response[marksrest.AddMarkResponse] {
	t.Helper()

	markTypeID := fixture.markTypeID
	if markTypeID == 0 {
		markTypesResponse := getMarkTypes(t, cfg, http.StatusOK)
		require.NotEmpty(t, markTypesResponse.Payload.MarkTypes)
		markTypeID = markTypesResponse.Payload.MarkTypes[0].ID
	}

	b := &bytes.Buffer{}
	mpw := multipart.NewWriter(b)
	require.NoError(t, mpw.WriteField("longitude", strconv.FormatFloat(fixtureMarkPoint.X(), 'f', -1, 64)))
	require.NoError(t, mpw.WriteField("latitude", strconv.FormatFloat(fixtureMarkPoint.Y(), 'f', -1, 64)))
	require.NoError(t, mpw.WriteField("mark_type_id", strconv.Itoa(markTypeID)))
	require.NoError(t, mpw.WriteField("description", "functional test mark"))

	for _, image := range getImages(2) {
		fw, err := mpw.CreateFormFile("photos", "test.jpg")
		require.NoError(t, err)
		_, err = io.Copy(fw, bytes.NewBuffer(image))
		require.NoError(t, err)
	}

	require.NoError(t, mpw.Close())

	return addMark(t, cfg, b, mpw.FormDataContentType(), accessToken, http.StatusCreated)
}

func addMark(t *testing.T, cfg *config.RESTConfig, request io.Reader, contentType string, accessToken string, expectedStatusCode int) responses.Response[marksrest.AddMarkResponse] {
	req, err := http.NewRequest(
		http.MethodPost,
		makeUrl(makeUrlParams{
			host: cfg.Host,
			port: cfg.Port,
			path: "/marks",
		}),
		request,
	)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[marksrest.AddMarkResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *MarksSuite) TestGetMarkTypes() {
	tests := []struct {
		name       string
		statusCode int
	}{
		{
			name:       "Ok200",
			statusCode: http.StatusOK,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			response := getMarkTypes(st.T(), &st.Cfg.REST, tt.statusCode)
			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.MarkTypes)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func getMarkTypes(t *testing.T, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[marksrest.GetMarkTypesResponse] {
	resp, err := http.Get(makeUrl(makeUrlParams{
		host: cfg.Host,
		port: cfg.Port,
		path: "/marks/types",
	}))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[marksrest.GetMarkTypesResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *MarksSuite) TestGetMarkStatuses() {
	tests := []struct {
		name       string
		statusCode int
	}{
		{
			name:       "Ok200",
			statusCode: http.StatusOK,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host: st.Cfg.REST.Host,
				port: st.Cfg.REST.Port,
				path: "/marks/statuses",
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.GetMarkStatusesResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.MarkStatuses)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MarksSuite) TestGetMarkStatusHistoryByMarkId() {
	markId := strconv.Itoa(st.fx.markID)

	tests := []struct {
		name       string
		id         string
		query      string
		statusCode int
	}{
		{
			name:       "Ok200",
			id:         markId,
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			id:         markId,
			query:      "withChecks=false",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			id:         markId,
			query:      "withChecks=true",
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400-id",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Err400-withChecks",
			id:         markId,
			query:      "withChecks=a",
			statusCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host:  st.Cfg.REST.Host,
				port:  st.Cfg.REST.Port,
				path:  fmt.Sprintf("/marks/%s/status-history", tt.id),
				query: tt.query,
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.GetMarkStatusHistoryByMarkIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Require().NotEmpty(response.Payload.HistoryItems, "a mark always has its creation history row")
				st.Equal(models.UnconfirmedStatus, response.Payload.HistoryItems[0].NewMarkStatusID)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MarksSuite) TestConfirm() {
	userAccessToken := st.fx.user.AccessToken
	moderatorAccessToken := st.fx.moderatorToken

	// Fresh unconfirmed mark: confirm moves it to "confirmed".
	markId := addNewMark(st.T(), &st.Cfg.REST, userAccessToken).Payload.MarkId

	// A refuted mark cannot be confirmed any more.
	markForRejectId := addNewMark(st.T(), &st.Cfg.REST, userAccessToken).Payload.MarkId
	rejectResponse := reject(st.T(), &st.Cfg.REST, strconv.Itoa(markForRejectId), moderatorAccessToken, http.StatusOK)
	st.Require().Equal(models.RefutedStatus, rejectResponse.Payload.NewMarkStausId)

	tests := []struct {
		name        string
		id          string
		accessToken string
		statusCode  int
		wantStatus  models.MarkStatusType
	}{
		{
			name:        "Ok200Unconfirmed",
			id:          strconv.Itoa(markId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusOK,
			wantStatus:  models.ConfirmedStatus,
		},
		{
			name:        "Ok200Confirmed",
			id:          strconv.Itoa(markId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusOK,
			wantStatus:  models.UnderReviewStatus,
		},
		{
			name:        "Ok200UnderReview",
			id:          strconv.Itoa(markId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusOK,
			wantStatus:  models.ClosedStatus,
		},
		{
			name:        "Err409Closed",
			id:          strconv.Itoa(markId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusConflict,
		},
		{
			name:        "Err400",
			id:          "a",
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusBadRequest,
		},
		{
			name:        "Err409Refuted",
			id:          strconv.Itoa(markForRejectId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusConflict,
		},
		{
			name:        "Err403User",
			id:          strconv.Itoa(markId),
			accessToken: userAccessToken,
			statusCode:  http.StatusForbidden,
		},
		{
			name:       "Err401NoToken",
			id:         strconv.Itoa(markId),
			statusCode: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			response := confirm(
				st.T(),
				&st.Cfg.REST,
				tt.id,
				tt.accessToken,
				tt.statusCode,
			)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Equal(tt.wantStatus, response.Payload.NewMarkStausId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MarksSuite) TestReject() {
	userAccessToken := st.fx.user.AccessToken
	moderatorAccessToken := st.fx.moderatorToken

	// Fresh unconfirmed mark: reject moves it to "refuted".
	markId := addNewMark(st.T(), &st.Cfg.REST, userAccessToken).Payload.MarkId

	// Mark under review: reject re-opens it, a second reject closes it.
	underReviewId := addNewMark(st.T(), &st.Cfg.REST, userAccessToken).Payload.MarkId
	confirmResponse := confirm(st.T(), &st.Cfg.REST, strconv.Itoa(underReviewId), moderatorAccessToken, http.StatusOK)
	st.Require().Equal(models.ConfirmedStatus, confirmResponse.Payload.NewMarkStausId)
	confirmResponse = confirm(st.T(), &st.Cfg.REST, strconv.Itoa(underReviewId), moderatorAccessToken, http.StatusOK)
	st.Require().Equal(models.UnderReviewStatus, confirmResponse.Payload.NewMarkStausId)

	tests := []struct {
		name        string
		id          string
		accessToken string
		statusCode  int
		wantStatus  models.MarkStatusType
	}{
		{
			name:        "Ok200Unconfirmed",
			id:          strconv.Itoa(markId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusOK,
			wantStatus:  models.RefutedStatus,
		},
		{
			name:        "Err409Refuted",
			id:          strconv.Itoa(markId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusConflict,
		},
		{
			name:        "Ok200UnderReview",
			id:          strconv.Itoa(underReviewId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusOK,
			wantStatus:  models.RediscoveredStatus,
		},
		{
			name:        "Ok200Rediscovered",
			id:          strconv.Itoa(underReviewId),
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusOK,
			wantStatus:  models.ClosedStatus,
		},
		{
			name:        "Err400",
			id:          "a",
			accessToken: moderatorAccessToken,
			statusCode:  http.StatusBadRequest,
		},
		{
			name:        "Err403User",
			id:          strconv.Itoa(markId),
			accessToken: userAccessToken,
			statusCode:  http.StatusForbidden,
		},
		{
			name:       "Err401NoToken",
			id:         strconv.Itoa(markId),
			statusCode: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			response := reject(
				st.T(),
				&st.Cfg.REST,
				tt.id,
				tt.accessToken,
				tt.statusCode,
			)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Equal(tt.wantStatus, response.Payload.NewMarkStausId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func confirm(t *testing.T, cfg *config.RESTConfig, id string, accessToken string, expectedStatusCode int) responses.Response[marksrest.ConfirmResponse] {
	req, err := http.NewRequest(
		http.MethodPost,
		makeUrl(makeUrlParams{
			host: cfg.Host,
			port: cfg.Port,
			path: fmt.Sprintf("/marks/%s/confirm", id),
		}),
		nil,
	)
	require.NoError(t, err)
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[marksrest.ConfirmResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func reject(t *testing.T, cfg *config.RESTConfig, id string, accessToken string, expectedStatusCode int) responses.Response[marksrest.RejectResponse] {
	req, err := http.NewRequest(
		http.MethodPost,
		makeUrl(makeUrlParams{
			host: cfg.Host,
			port: cfg.Port,
			path: fmt.Sprintf("/marks/%s/reject", id),
		}),
		nil,
	)
	require.NoError(t, err)
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[marksrest.RejectResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}
