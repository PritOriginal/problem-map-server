//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	tasksrest "github.com/PritOriginal/problem-map-server/internal/handler/tasks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
)

// testConfigPath is the config the functional tests run against, relative to
// this package. The REST server under test must be started with the same file.
const testConfigPath = "../../configs/config-tests.yaml"

// Coordinates inside the city bounding box used by all fixtures (lon, lat).
var (
	fixtureHomePoint = geom.Coord{41.4006, 52.6999}
	fixtureMarkPoint = geom.Coord{41.4028, 52.7001}
)

// fixtures is the deterministic data set seeded once per test binary through
// the public API. Tests read from it instead of picking random rows out of
// whatever the database happens to contain.
type fixtures struct {
	// user is a regular account created via /auth/signup.
	user struct {
		ID          int
		Username    string
		Login       string
		Password    string
		AccessToken string
	}
	// moderatorToken is an access token with the moderator role for a
	// second account; it is signed with the server key from the config.
	moderatorToken string
	// markTypeID is the first mark type returned by /marks/types.
	markTypeID int
	// markID is an unconfirmed mark owned by user.
	markID int
	// taskID is a task created by the moderator for markID.
	taskID int
}

var (
	fixtureOnce sync.Once
	fixture     fixtures
	fixtureCfg  *config.Config
)

// loadFixtures loads the test config and seeds the shared fixtures exactly
// once; every suite calls it from SetupSuite.
func loadFixtures(t *testing.T) (*config.Config, *fixtures) {
	t.Helper()

	fixtureOnce.Do(func() {
		fixtureCfg = config.MustLoadPath(testConfigPath)
		seedFixtures(t, fixtureCfg)
	})
	require.NotNil(t, fixtureCfg, "fixtures failed to seed in an earlier suite")

	return fixtureCfg, &fixture
}

func seedFixtures(t *testing.T, cfg *config.Config) {
	t.Helper()

	fixture.user.Username = gofakeit.FirstName()
	fixture.user.Login = gofakeit.Username()
	fixture.user.Password = gofakeit.Password(true, true, true, true, true, 10)

	signUpReq, err := json.Marshal(authrest.SignUpRequest{
		Username:  fixture.user.Username,
		Login:     fixture.user.Login,
		Password:  fixture.user.Password,
		HomePoint: models.NewPoint(fixtureHomePoint),
	})
	require.NoError(t, err)
	_ = signUp(t, bytes.NewBuffer(signUpReq), &cfg.REST, http.StatusCreated)

	signInReq, err := json.Marshal(authrest.SignInRequest{
		Login:    fixture.user.Login,
		Password: fixture.user.Password,
	})
	require.NoError(t, err)
	signInResp := signIn(t, bytes.NewBuffer(signInReq), &cfg.REST, http.StatusOK)
	fixture.user.AccessToken = signInResp.Payload.AccessToken
	fixture.user.ID = currentUserId(t, &cfg.REST, fixture.user.AccessToken)

	fixture.moderatorToken = moderatorToken(t, cfg)

	markTypes := getMarkTypes(t, &cfg.REST, http.StatusOK)
	require.NotEmpty(t, markTypes.Payload.MarkTypes, "mark types must be migrated")
	fixture.markTypeID = markTypes.Payload.MarkTypes[0].ID

	fixture.markID = addNewMark(t, &cfg.REST, fixture.user.AccessToken).Payload.MarkId

	taskReq, err := json.Marshal(tasksrest.AddTaskRequest{Name: "fixture task", MarkID: fixture.markID})
	require.NoError(t, err)
	fixture.taskID = addTask(t, &cfg.REST, bytes.NewBuffer(taskReq), fixture.moderatorToken, http.StatusCreated).Payload.TaskId
}

func addTask(t *testing.T, cfg *config.RESTConfig, body *bytes.Buffer, accessToken string, expectedStatusCode int) responses.Response[tasksrest.AddTaskResponse] {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, makeUrl(makeUrlParams{
		host: cfg.Host,
		port: cfg.Port,
		path: "/tasks",
	}), body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[tasksrest.AddTaskResponse]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))

	return response
}

type makeUrlParams struct {
	host  string
	port  int
	path  string
	query string
}

func makeUrl(params makeUrlParams) string {
	host := params.host
	if params.port > 0 {
		host = fmt.Sprintf("%s:%d", params.host, params.port)
	}

	u := url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     params.path,
		RawQuery: params.query,
	}
	return u.String()
}

// getImages returns num small JPEG images alternating between portrait and
// landscape orientation.
func getImages(num int) [][]byte {
	images := make([][]byte, 0, num)
	for i := range num {
		if i%2 == 0 {
			images = append(images, gofakeit.ImageJpeg(9, 12))
		} else {
			images = append(images, gofakeit.ImageJpeg(12, 9))
		}
	}
	return images
}
