//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/tests/rest/suite"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
)

func TestGetRegions(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/regions", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetRegionsResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Regions)
}

func TestGetCities(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/cities", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetCitiesResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Cities)
}

func TestGetDistricts(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/districts", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetDistrictsResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Districts)
}

func TestGetMarks(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/marks", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetMarksResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Marks)
}

func TestAddMark(t *testing.T) {
	st := suite.New(t)

	signInResponse := signIn(t, st, authrest.SignInRequest{
		Username: "user4",
		Password: "1234qwer",
	})

	mark := models.Mark{
		Geom:         models.NewPoint(geom.Coord{41.497976, 52.707605}),
		TypeMarkID:   1,
		MarkStatusID: 1,
		UserID:       1,
		DistrictID:   1,
	}

	jsonData, err := json.Marshal(mark)
	require.NoError(t, err)

	b := &bytes.Buffer{}
	w := multipart.NewWriter(b)

	w.WriteField("data", string(jsonData))

	image := gofakeit.ImageJpeg(10, 10)
	fw, err := w.CreateFormFile("photo", "test.jpg")
	io.Copy(fw, bytes.NewBuffer(image))

	w.Close()

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/map/marks", st.Cfg.REST.Host, st.Cfg.REST.Port), b)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+signInResponse.Payload.AccessToken)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	defer resp.Body.Close()

}
