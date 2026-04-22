//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MarksSuite struct {
	suite.Suite
	Cfg *config.Config
}

func (st *MarksSuite) SetupSuite() {
	st.Cfg = config.MustLoadPath("../../configs/config.yaml")
}

func TestMarksSuite(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func (st *MarksSuite) TestGetMarks() {
	response := getMarks(st.T(), &st.Cfg.REST, http.StatusOK)
	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Marks)
}

func getMarks(t *testing.T, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[marksrest.GetMarksResponse] {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/marks", cfg.Host, cfg.Port))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[marksrest.GetMarksResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *MarksSuite) TestGetMarkById() {
	getMarksResponse := getMarks(st.T(), &st.Cfg.REST, http.StatusOK)

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
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/marks/%s", st.Cfg.REST.Host, st.Cfg.REST.Port, tt.id))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.GetMarkByIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.Mark)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MarksSuite) TestGetMarkByUserId() {
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
			name:       "Ok400",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/marks/user/%s", st.Cfg.REST.Host, st.Cfg.REST.Port, tt.id))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.GetMarkByIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.Mark)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MarksSuite) TestAddMark() {
	signInResponse := addNewUser(st.T(), &st.Cfg.REST)

	markTypesResponse := getMarkTypes(st, http.StatusOK)
	randomMarkTypeIndex := rand.Intn(len(markTypesResponse.Payload.MarkTypes))
	randomMarkType := markTypesResponse.Payload.MarkTypes[randomMarkTypeIndex]

	long, err := gofakeit.LatitudeInRange(52.6, 52.8)
	st.NoError(err)
	lat, err := gofakeit.LongitudeInRange(41.25, 41.55)
	st.NoError(err)

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
				MarkTypeID:  randomMarkType.ID,
				Description: "",
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
			mpw.WriteField("longitude", strconv.FormatFloat(tt.req.Longitude, 'f', -1, 64))
			mpw.WriteField("latitude", strconv.FormatFloat(tt.req.Latitude, 'f', -1, 64))
			mpw.WriteField("mark_type_id", strconv.Itoa(tt.req.MarkTypeID))
			mpw.WriteField("description", tt.req.Description)

			image := gofakeit.ImageJpeg(10, 10)
			fw, err := mpw.CreateFormFile("photos", "test.jpg")
			st.NoError(err)
			io.Copy(fw, bytes.NewBuffer(image))

			mpw.Close()

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/marks", st.Cfg.REST.Host, st.Cfg.REST.Port), b)
			st.NoError(err)

			req.Header.Set("Authorization", "Bearer "+signInResponse.Payload.AccessToken)
			req.Header.Set("Content-Type", mpw.FormDataContentType())

			resp, err := http.DefaultClient.Do(req)
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.AddMarkResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.MarkId)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
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
			response := getMarkTypes(st, tt.statusCode)
			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.MarkTypes)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func getMarkTypes(st *MarksSuite, expectedStatusCode int) responses.Response[marksrest.GetMarkTypesResponse] {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/marks/types", st.Cfg.REST.Host, st.Cfg.REST.Port))
	st.NoError(err)
	defer resp.Body.Close()

	st.Equal(expectedStatusCode, resp.StatusCode)

	var response responses.Response[marksrest.GetMarkTypesResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	st.NoError(err)

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
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/marks/statuses", st.Cfg.REST.Host, st.Cfg.REST.Port))
			st.NoError(err)
			defer resp.Body.Close()

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
	getMarksResponse := getMarks(st.T(), &st.Cfg.REST, http.StatusOK)
	markId := strconv.Itoa(getMarksResponse.Payload.Marks[0].ID)

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
			query:      "?withChecks=false",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			id:         "1",
			query:      "?withChecks=true",
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400-id",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Err400-withChecks",
			id:         "1",
			query:      "?withChecks=a",
			statusCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(fmt.Sprintf(
				"http://%s:%d/marks/%s/status-history%s",
				st.Cfg.REST.Host,
				st.Cfg.REST.Port,
				tt.id,
				tt.query,
			))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[marksrest.GetMarkStatusHistoryByMarkIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.HistoryItems)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}
