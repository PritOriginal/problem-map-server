package marksrest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/middleware/lang"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/gin-gonic/gin"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MarksSuite struct {
	suite.Suite
	r             *gin.Engine
	uc            *marksrest.MockMarks
	statusUpdater *marksrest.MockStatusUpdater
	cacher        *mwcache.MockCacher
}

func (suite *MarksSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Key: []byte("1234"),
	})
	if err != nil {
		panic(err)
	}
	errInit := authMiddleware.MiddlewareInit()
	if errInit != nil {
		panic(errInit)
	}

	suite.uc = marksrest.NewMockMarks(suite.T())
	suite.statusUpdater = marksrest.NewMockStatusUpdater(suite.T())
	suite.cacher = mwcache.NewMockCacher(suite.T())

	log := slogdiscard.NewDiscardLogger()

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	suite.r.Use(lang.New())

	marksrest.Register(suite.r, log, marksrest.Params{
		AuthMiddleware: authMiddleware,
		Cacher:         suite.cacher,
		Usecase:        suite.uc,
		StatusUpdater:  suite.statusUpdater,
	})
}

func TestMark(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func (suite *MarksSuite) TestGetMarks() {
	tests := []struct {
		name                      string
		query                     string
		wantErrParseMarkTypeIds   bool
		wantErrParseMarkStatusIds bool
		errGetMarks               error
		statusCode                int
	}{
		{
			name:       "Ok200",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "?mark_type_ids=1",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "?mark_type_ids=1,2",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "?mark_type_ids=1,2&mark_status_ids=1",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "?mark_type_ids=1,2&mark_status_ids=1,2",
			statusCode: http.StatusOK,
		},
		{
			name:                    "Ok400",
			query:                   "?mark_type_ids=a",
			wantErrParseMarkTypeIds: true,
			statusCode:              http.StatusBadRequest,
		},
		{
			name:                    "Ok400",
			query:                   "?mark_status_ids=a",
			wantErrParseMarkTypeIds: true,
			statusCode:              http.StatusBadRequest,
		},
		{
			name:        "Err500",
			errGetMarks: errors.New(""),
			statusCode:  500,
		},
		{
			name:        "Err400InvalidArgumentFromUsecase",
			errGetMarks: usecase.ErrInvalidArgument,
			statusCode:  http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseMarkStatusIds && !tt.wantErrParseMarkTypeIds {
				suite.uc.On("ListMarks", mock.Anything, mock.Anything).Once().
					Return(models.Page[models.Mark]{Items: []models.Mark{}}, tt.errGetMarks)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestGetMarksFilters() {
	createdFrom := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	createdTo := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		query       string
		wantFilters models.GetMarksFilters
		statusCode  int
	}{
		{
			name:  "Defaults",
			query: "",
			wantFilters: models.GetMarksFilters{
				IDs:           []int{},
				MarkTypeIds:   []int{},
				MarkStatusIds: []int{},
				Pagination:    models.Pagination{Limit: models.DefaultLimit},
			},
			statusCode: http.StatusOK,
		},
		{
			name:  "AllFilters",
			query: "?bbox=41.4,52.7,41.5,52.8&limit=50&offset=100&sort=updated_at&order=asc&user_id=7&created_from=2025-01-02T03:04:05Z&created_to=2025-02-01T00:00:00Z&updated_since=2025-01-10T00:00:00Z&mark_type_ids=1,2",
			wantFilters: models.GetMarksFilters{
				IDs:           []int{},
				MarkTypeIds:   []int{1, 2},
				MarkStatusIds: []int{},
				UserID:        7,
				BBox:          &models.BBox{MinLon: 41.4, MinLat: 52.7, MaxLon: 41.5, MaxLat: 52.8},
				CreatedFrom:   createdFrom,
				CreatedTo:     createdTo,
				UpdatedSince:  time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
				Sort:          models.MarksSortUpdatedAt,
				Order:         models.SortAsc,
				Pagination:    models.Pagination{Limit: 50, Offset: 100},
			},
			statusCode: http.StatusOK,
		},
		{
			name:  "Ids",
			query: "?ids=3,1,2",
			wantFilters: models.GetMarksFilters{
				IDs:           []int{3, 1, 2},
				MarkTypeIds:   []int{},
				MarkStatusIds: []int{},
				Pagination:    models.Pagination{Limit: models.DefaultLimit},
			},
			statusCode: http.StatusOK,
		},
		{name: "ErrIdsNotNumber", query: "?ids=1,x", statusCode: http.StatusBadRequest},
		{name: "ErrIdsTooMany", query: "?ids=" + strings.Repeat("1,", models.MaxMarksIDs) + "1", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxThreeParts", query: "?bbox=1,2,3", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxNotNumber", query: "?bbox=a,2,3,4", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxMinGreaterThanMax", query: "?bbox=41.5,52.7,41.4,52.8", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxOutOfRange", query: "?bbox=-181,52.7,41.4,52.8", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxNaN", query: "?bbox=NaN,52.7,41.4,52.8", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxInf", query: "?bbox=41.4,52.7,Inf,52.8", statusCode: http.StatusBadRequest},
		{name: "ErrLimitTooBig", query: "?limit=501", statusCode: http.StatusBadRequest},
		{name: "ErrLimitZero", query: "?limit=0", statusCode: http.StatusBadRequest},
		{name: "ErrOffsetNegative", query: "?offset=-5", statusCode: http.StatusBadRequest},
		{name: "ErrSort", query: "?sort=description", statusCode: http.StatusBadRequest},
		{name: "ErrOrder", query: "?order=random", statusCode: http.StatusBadRequest},
		{name: "ErrUserIdNegative", query: "?user_id=-1", statusCode: http.StatusBadRequest},
		{name: "ErrCreatedFromFormat", query: "?created_from=2025-01-02", statusCode: http.StatusBadRequest},
		{name: "ErrCreatedToFormat", query: "?created_to=yesterday", statusCode: http.StatusBadRequest},
		{name: "ErrCreatedRange", query: "?created_from=2025-02-01T00:00:00Z&created_to=2025-01-01T00:00:00Z", statusCode: http.StatusBadRequest},
		{name: "ErrUpdatedSinceFormat", query: "?updated_since=2025-01-10", statusCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode == http.StatusOK {
				suite.uc.On("ListMarks", mock.Anything, tt.wantFilters).Once().
					Return(models.Page[models.Mark]{Items: []models.Mark{{ID: 1}}, Total: 321}, nil)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[marksrest.GetMarksResponse]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Len(resp.Payload.Marks, 1)
				suite.Equal(&responses.ListMeta{
					Limit:  tt.wantFilters.Pagination.Limit,
					Offset: tt.wantFilters.Pagination.Offset,
					Total:  321,
				}, resp.Meta)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkChanges() {
	since := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	serverTime := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		query       string
		wantFilters models.MarkChangesFilters
		changes     models.MarkChanges
		err         error
		statusCode  int
	}{
		{
			name:        "Ok200",
			query:       "?since=2025-03-01T12:00:00Z",
			wantFilters: models.MarkChangesFilters{Since: since, Pagination: models.Pagination{Limit: models.DefaultLimit}},
			changes: models.MarkChanges{
				Marks: []models.Mark{{ID: 1}, {ID: 2}}, Total: 2,
				DeletedIDs: []int{3}, DeletedTotal: 1, HiddenIDs: []int{}, ServerTime: serverTime,
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "Ok200EmptyArraysNotNull",
			query:       "?since=2025-03-01T12:00:00Z&limit=10&offset=20",
			wantFilters: models.MarkChangesFilters{Since: since, Pagination: models.Pagination{Limit: 10, Offset: 20}},
			changes:     models.MarkChanges{ServerTime: serverTime},
			statusCode:  http.StatusOK,
		},
		{name: "Err400MissingSince", query: "", statusCode: http.StatusBadRequest},
		{name: "Err400SinceFormat", query: "?since=yesterday", statusCode: http.StatusBadRequest},
		{name: "Err400SinceInFuture", query: "?since=2999-01-01T00:00:00Z", statusCode: http.StatusBadRequest},
		{name: "Err400Limit", query: "?since=2025-03-01T12:00:00Z&limit=0", statusCode: http.StatusBadRequest},
		{
			name:        "Err500",
			query:       "?since=2025-03-01T12:00:00Z",
			wantFilters: models.MarkChangesFilters{Since: since, Pagination: models.Pagination{Limit: models.DefaultLimit}},
			err:         errors.New("db down"),
			statusCode:  http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode != http.StatusBadRequest {
				suite.uc.On("GetMarkChanges", mock.Anything, tt.wantFilters).Once().Return(tt.changes, tt.err)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/changes"+tt.query, nil)
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusOK {
				return
			}

			var resp responses.Response[marksrest.GetMarkChangesResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Len(resp.Payload.Marks, len(tt.changes.Marks))
			suite.NotNil(resp.Payload.DeletedIDs)
			suite.NotNil(resp.Payload.HiddenIDs)
			suite.ElementsMatch(tt.changes.DeletedIDs, resp.Payload.DeletedIDs)
			suite.Equal(tt.changes.DeletedTotal, resp.Payload.DeletedTotal)
			suite.True(serverTime.Equal(resp.Payload.ServerTime))
			suite.Equal(&responses.ListMeta{
				Limit: tt.wantFilters.Pagination.Limit, Offset: tt.wantFilters.Pagination.Offset, Total: tt.changes.Total,
			}, resp.Meta)
			// Raw JSON must carry arrays, never null.
			suite.Contains(w.Body.String(), `"deleted_ids":[`)
			suite.Contains(w.Body.String(), `"hidden_ids":[`)
			suite.Contains(w.Body.String(), `"marks":[`)
		})
	}
}

func (suite *MarksSuite) TestGetMarksNearby() {
	lon, lat := 41.45, 52.72

	tests := []struct {
		name        string
		query       string
		wantFilters models.GetMarksNearbyFilters
		errNearby   error
		statusCode  int
	}{
		{
			name:  "Ok",
			query: "?lon=41.45&lat=52.72&radius=1500",
			wantFilters: models.GetMarksNearbyFilters{
				Lon: lon, Lat: lat, RadiusM: 1500,
				MarkTypeIds: []int{}, MarkStatusIds: []int{},
				Pagination: models.Pagination{Limit: models.DefaultLimit},
			},
			statusCode: http.StatusOK,
		},
		{
			name:  "OkZeroCoordinates",
			query: "?lon=0&lat=0&radius=10&limit=5&offset=5&mark_status_ids=1,2",
			wantFilters: models.GetMarksNearbyFilters{
				Lon: 0, Lat: 0, RadiusM: 10,
				MarkTypeIds: []int{}, MarkStatusIds: []int{1, 2},
				Pagination: models.Pagination{Limit: 5, Offset: 5},
			},
			statusCode: http.StatusOK,
		},
		{name: "ErrMissingLon", query: "?lat=52.72&radius=1500", statusCode: http.StatusBadRequest},
		{name: "ErrMissingLat", query: "?lon=41.45&radius=1500", statusCode: http.StatusBadRequest},
		{name: "ErrMissingRadius", query: "?lon=41.45&lat=52.72", statusCode: http.StatusBadRequest},
		{name: "ErrRadiusTooBig", query: "?lon=41.45&lat=52.72&radius=50001", statusCode: http.StatusBadRequest},
		{name: "ErrRadiusNegative", query: "?lon=41.45&lat=52.72&radius=-1", statusCode: http.StatusBadRequest},
		{name: "ErrLonOutOfRange", query: "?lon=181&lat=52.72&radius=100", statusCode: http.StatusBadRequest},
		{name: "ErrLatOutOfRange", query: "?lon=41.45&lat=91&radius=100", statusCode: http.StatusBadRequest},
		{name: "ErrMarkTypeIds", query: "?lon=41.45&lat=52.72&radius=100&mark_type_ids=x", statusCode: http.StatusBadRequest},
		{name: "ErrLimit", query: "?lon=41.45&lat=52.72&radius=100&limit=1000", statusCode: http.StatusBadRequest},
		{
			name:  "Err500",
			query: "?lon=41.45&lat=52.72&radius=1500",
			wantFilters: models.GetMarksNearbyFilters{
				Lon: lon, Lat: lat, RadiusM: 1500,
				MarkTypeIds: []int{}, MarkStatusIds: []int{},
				Pagination: models.Pagination{Limit: models.DefaultLimit},
			},
			errNearby:  errors.New(""),
			statusCode: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode != http.StatusBadRequest {
				suite.uc.On("GetMarksNearby", mock.Anything, tt.wantFilters).Once().
					Return(models.Page[models.MarkWithDistance]{
						Items: []models.MarkWithDistance{{Mark: models.Mark{ID: 1}, DistanceM: 42.5}},
						Total: 1,
					}, tt.errNearby)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/nearby"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[map[string][]map[string]any]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Len(resp.Payload["marks"], 1)
				suite.Equal(42.5, resp.Payload["marks"][0]["distance_m"])
				suite.Equal(float64(1), resp.Payload["marks"][0]["mark_id"])
				suite.Equal(&responses.ListMeta{
					Limit:  tt.wantFilters.Pagination.Limit,
					Offset: tt.wantFilters.Pagination.Offset,
					Total:  1,
				}, resp.Meta)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkById() {
	tests := []struct {
		name           string
		id             string
		wantErrParseId bool
		errGetMarkById error
		statusCode     int
	}{
		{
			name:           "Ok200",
			id:             "1",
			wantErrParseId: false,
			errGetMarkById: nil,
			statusCode:     200,
		},
		{
			name:           "Err500",
			id:             "1",
			wantErrParseId: false,
			errGetMarkById: errors.New(""),
			statusCode:     500,
		},
		{
			name:           "Err400",
			id:             "a",
			wantErrParseId: true,
			errGetMarkById: nil,
			statusCode:     400,
		},
		{
			name:           "Err404",
			id:             "1",
			wantErrParseId: false,
			errGetMarkById: usecase.ErrNotFound,
			statusCode:     404,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.Mark{}, tt.errGetMarkById)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestGetMarksByUserId() {
	tests := []struct {
		name                string
		id                  string
		wantErrParseId      bool
		errGetMarksByUserId error
		statusCode          int
	}{
		{
			name:                "Ok200",
			id:                  "1",
			wantErrParseId:      false,
			errGetMarksByUserId: nil,
			statusCode:          200,
		},
		{
			name:                "Err500",
			id:                  "1",
			wantErrParseId:      false,
			errGetMarksByUserId: errors.New(""),
			statusCode:          500,
		},
		{
			name:                "Err400",
			id:                  "a",
			wantErrParseId:      true,
			errGetMarksByUserId: nil,
			statusCode:          400,
		},
		{
			name:           "Err400Limit",
			id:             "1?limit=501",
			wantErrParseId: true,
			statusCode:     400,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("ListMarksByUserId", mock.Anything, mock.AnythingOfType("int"), models.Pagination{Limit: models.DefaultLimit}).Once().
					Return(models.Page[models.Mark]{Items: []models.Mark{}}, tt.errGetMarksByUserId)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/user/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestAddMark() {
	tests := []struct {
		name            string
		req             marksrest.AddMarkRequest
		invalidPhoto    bool
		wantErrParseReq bool
		errAddCheck     error
		statusCode      int
	}{
		{
			name: "Err400InvalidPhoto",
			req: marksrest.AddMarkRequest{
				Longitude:   42,
				Latitude:    52,
				MarkTypeID:  1,
				Description: "",
			},
			invalidPhoto:    true,
			wantErrParseReq: true,
			statusCode:      400,
		},
		{
			name: "Ok201",
			req: marksrest.AddMarkRequest{
				Longitude:   42,
				Latitude:    52,
				MarkTypeID:  1,
				Description: "",
			},
			wantErrParseReq: false,
			errAddCheck:     nil,
			statusCode:      201,
		},
		{
			name: "Err400InvalidReq-1",
			req: marksrest.AddMarkRequest{
				Longitude: 42,
				Latitude:  52,
			},
			wantErrParseReq: true,
			errAddCheck:     nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq-2",
			req: marksrest.AddMarkRequest{
				Longitude:   42,
				MarkTypeID:  1,
				Description: "",
			},
			wantErrParseReq: true,
			errAddCheck:     nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq-3",
			req: marksrest.AddMarkRequest{
				Longitude:   42,
				Latitude:    52,
				MarkTypeID:  1,
				Description: strings.Repeat("A", 257),
			},
			wantErrParseReq: true,
			errAddCheck:     nil,
			statusCode:      400,
		},
		{
			name: "Err500",
			req: marksrest.AddMarkRequest{
				Longitude:   42,
				Latitude:    52,
				MarkTypeID:  1,
				Description: "",
			},
			wantErrParseReq: false,
			errAddCheck:     errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseReq {
				suite.uc.On("AddMark", mock.Anything, mock.MatchedBy(func(m models.Mark) bool {
					// X must be longitude, Y must be latitude (GeoJSON/PostGIS order)
					return m.Geom != nil &&
						m.Geom.Ewkb.X() == tt.req.Longitude &&
						m.Geom.Ewkb.Y() == tt.req.Latitude
				}), mock.Anything, false).Once().
					Return(int64(1), tt.errAddCheck)
			}

			w := httptest.NewRecorder()

			b := &bytes.Buffer{}
			mpw := multipart.NewWriter(b)

			suite.NoError(mpw.WriteField("longitude", strconv.FormatFloat(tt.req.Longitude, 'f', -1, 64)))
			suite.NoError(mpw.WriteField("latitude", strconv.FormatFloat(tt.req.Latitude, 'f', -1, 64)))
			suite.NoError(mpw.WriteField("mark_type_id", strconv.Itoa(tt.req.MarkTypeID)))
			suite.NoError(mpw.WriteField("description", tt.req.Description))

			image := gofakeit.ImageJpeg(10, 10)
			if tt.invalidPhoto {
				image = []byte("not an image")
			}
			fw, err := mpw.CreateFormFile("photos", "test.jpg")
			suite.NoError(err)
			_, err = io.Copy(fw, bytes.NewBuffer(image))
			suite.NoError(err)

			suite.NoError(mpw.Close())

			accessToken, err := token.CreateToken(1*time.Minute, 1, string(models.RoleUser), "1234")
			suite.NoError(err)

			req := httptest.NewRequest("POST", "/marks", b)
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("Content-Type", mpw.FormDataContentType())

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

// bearer returns an Authorization header value for user id 1 with the role.
func (suite *MarksSuite) bearer(role models.Role) string {
	accessToken, err := token.CreateToken(1*time.Minute, 1, string(role), "1234")
	suite.Require().NoError(err)
	return "Bearer " + accessToken
}

func (suite *MarksSuite) TestAddMarkSimilar() {
	similar := []models.MarkWithDistance{{Mark: models.Mark{ID: 5, MarkTypeID: 1}, DistanceM: 12.5}}

	tests := []struct {
		name       string
		query      string
		wantForce  bool
		errAdd     error
		statusCode int
	}{
		{name: "Conflict409WithSimilar", errAdd: &usecase.SimilarMarksError{Marks: similar}, statusCode: http.StatusConflict},
		{name: "ForcedCreated201", query: "?force=true", wantForce: true, statusCode: http.StatusCreated},
		{name: "ForceFalseCreated201", query: "?force=false", statusCode: http.StatusCreated},
		{name: "ConflictWrapped409", errAdd: fmt.Errorf("op: %w", &usecase.SimilarMarksError{Marks: similar}), statusCode: http.StatusConflict},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("AddMark", mock.Anything, mock.Anything, mock.Anything, tt.wantForce).Once().
				Return(int64(1), tt.errAdd)

			b := &bytes.Buffer{}
			mpw := multipart.NewWriter(b)
			suite.NoError(mpw.WriteField("longitude", "41.44"))
			suite.NoError(mpw.WriteField("latitude", "52.72"))
			suite.NoError(mpw.WriteField("mark_type_id", "1"))
			fw, err := mpw.CreateFormFile("photos", "test.jpg")
			suite.NoError(err)
			_, err = io.Copy(fw, bytes.NewBuffer(gofakeit.ImageJpeg(10, 10)))
			suite.NoError(err)
			suite.NoError(mpw.Close())

			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/marks"+tt.query, b)
			req.Header.Set("Authorization", suite.bearer(models.RoleUser))
			req.Header.Set("Content-Type", mpw.FormDataContentType())

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code, w.Body.String())
			if tt.statusCode == http.StatusConflict {
				var body responses.Response[marksrest.SimilarMarksPayload]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body))
				suite.False(body.Success)
				suite.Require().NotNil(body.Error)
				suite.Require().Len(body.Payload.SimilarMarks, 1)
				suite.Equal(5, body.Payload.SimilarMarks[0].ID)
				suite.Equal(12.5, body.Payload.SimilarMarks[0].DistanceM)
			}
		})
	}
}

func (suite *MarksSuite) TestGetSimilarMarks() {
	tests := []struct {
		name        string
		query       string
		wantFilters *models.GetSimilarMarksFilters
		errFind     error
		statusCode  int
	}{
		{name: "Ok200", query: "?lon=41.44&lat=52.72&mark_type_id=1", wantFilters: &models.GetSimilarMarksFilters{Lon: 41.44, Lat: 52.72, MarkTypeID: 1}, statusCode: http.StatusOK},
		{name: "Ok200Radius", query: "?lon=41.44&lat=52.72&mark_type_id=2&radius=100", wantFilters: &models.GetSimilarMarksFilters{Lon: 41.44, Lat: 52.72, MarkTypeID: 2, RadiusM: 100}, statusCode: http.StatusOK},
		{name: "Err400NoType", query: "?lon=41.44&lat=52.72", statusCode: http.StatusBadRequest},
		{name: "Err400NoLon", query: "?lat=52.72&mark_type_id=1", statusCode: http.StatusBadRequest},
		{name: "Err400RadiusTooBig", query: "?lon=41.44&lat=52.72&mark_type_id=1&radius=50001", statusCode: http.StatusBadRequest},
		{name: "Err400FromUsecase", query: "?lon=41.44&lat=52.72&mark_type_id=1", wantFilters: &models.GetSimilarMarksFilters{Lon: 41.44, Lat: 52.72, MarkTypeID: 1}, errFind: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err500", query: "?lon=41.44&lat=52.72&mark_type_id=1", wantFilters: &models.GetSimilarMarksFilters{Lon: 41.44, Lat: 52.72, MarkTypeID: 1}, errFind: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantFilters != nil {
				suite.uc.On("FindSimilarMarks", mock.Anything, *tt.wantFilters).Once().
					Return([]models.MarkWithDistance{}, tt.errFind)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/similar"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestUpdateMark() {
	desc := "fixed"
	tests := []struct {
		name       string
		id         string
		body       string
		role       models.Role
		noToken    bool
		wantUpd    *models.MarkUpdate
		errUpdate  error
		statusCode int
	}{
		{name: "Ok200Description", id: "1", body: `{"description":"fixed"}`, role: models.RoleUser, wantUpd: &models.MarkUpdate{Description: &desc}, statusCode: http.StatusOK},
		{name: "Ok200Type", id: "1", body: `{"mark_type_id":2}`, role: models.RoleModerator, wantUpd: &models.MarkUpdate{MarkTypeID: ptr(2)}, statusCode: http.StatusOK},
		{name: "Err400BadId", id: "a", body: `{"description":"fixed"}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400BadJSON", id: "1", body: `{`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400TooLong", id: "1", body: `{"description":"` + strings.Repeat("A", 257) + `"}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400BadType", id: "1", body: `{"mark_type_id":0}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400Empty", id: "1", body: `{}`, role: models.RoleUser, wantUpd: &models.MarkUpdate{}, errUpdate: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err401NoToken", id: "1", body: `{"description":"fixed"}`, noToken: true, statusCode: http.StatusUnauthorized},
		{name: "Err403Stranger", id: "1", body: `{"description":"fixed"}`, role: models.RoleUser, wantUpd: &models.MarkUpdate{Description: &desc}, errUpdate: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", id: "1", body: `{"description":"fixed"}`, role: models.RoleUser, wantUpd: &models.MarkUpdate{Description: &desc}, errUpdate: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409WrongStatus", id: "1", body: `{"description":"fixed"}`, role: models.RoleUser, wantUpd: &models.MarkUpdate{Description: &desc}, errUpdate: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", id: "1", body: `{"description":"fixed"}`, role: models.RoleUser, wantUpd: &models.MarkUpdate{Description: &desc}, errUpdate: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantUpd != nil {
				suite.uc.On("UpdateMark", mock.Anything, models.Actor{UserID: 1, Role: tt.role}, 1, *tt.wantUpd).Once().
					Return(models.Mark{ID: 1, Description: desc}, tt.errUpdate)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("PATCH", "/marks/"+tt.id, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(tt.role))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestDeleteMark() {
	tests := []struct {
		name       string
		id         string
		role       models.Role
		noToken    bool
		callUC     bool
		errDelete  error
		statusCode int
	}{
		{name: "Ok200Owner", id: "1", role: models.RoleUser, callUC: true, statusCode: http.StatusOK},
		{name: "Ok200Moderator", id: "1", role: models.RoleModerator, callUC: true, statusCode: http.StatusOK},
		{name: "Err400BadId", id: "a", role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err401NoToken", id: "1", noToken: true, statusCode: http.StatusUnauthorized},
		{name: "Err403", id: "1", role: models.RoleUser, callUC: true, errDelete: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", id: "1", role: models.RoleUser, callUC: true, errDelete: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409", id: "1", role: models.RoleUser, callUC: true, errDelete: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", id: "1", role: models.RoleUser, callUC: true, errDelete: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callUC {
				suite.uc.On("DeleteMark", mock.Anything, models.Actor{UserID: 1, Role: tt.role}, 1).Once().Return(tt.errDelete)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("DELETE", "/marks/"+tt.id, nil)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(tt.role))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestFollowUnfollowMark() {
	tests := []struct {
		name       string
		method     string
		id         string
		noToken    bool
		callUC     bool
		errUC      error
		statusCode int
		following  bool
	}{
		{name: "FollowOk200", method: "POST", id: "1", callUC: true, statusCode: http.StatusOK, following: true},
		{name: "UnfollowOk200", method: "DELETE", id: "1", callUC: true, statusCode: http.StatusOK},
		{name: "FollowErr400", method: "POST", id: "a", statusCode: http.StatusBadRequest},
		{name: "UnfollowErr400", method: "DELETE", id: "a", statusCode: http.StatusBadRequest},
		{name: "FollowErr401", method: "POST", id: "1", noToken: true, statusCode: http.StatusUnauthorized},
		{name: "UnfollowErr401", method: "DELETE", id: "1", noToken: true, statusCode: http.StatusUnauthorized},
		{name: "FollowErr404", method: "POST", id: "1", callUC: true, errUC: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "UnfollowErr404", method: "DELETE", id: "1", callUC: true, errUC: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "FollowErr500", method: "POST", id: "1", callUC: true, errUC: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			ucMethod := "FollowMark"
			if tt.method == "DELETE" {
				ucMethod = "UnfollowMark"
			}
			if tt.callUC {
				suite.uc.On(ucMethod, mock.Anything, 1, 1).Once().Return(tt.errUC)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/marks/"+tt.id+"/follow", nil)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(models.RoleUser))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var body responses.Response[marksrest.FollowResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body))
				suite.Equal(marksrest.FollowResponse{MarkId: 1, Following: tt.following}, body.Payload)
			}
		})
	}
}

func (suite *MarksSuite) TestGetFollowedMarks() {
	tests := []struct {
		name       string
		query      string
		noToken    bool
		wantP      *models.Pagination
		errList    error
		statusCode int
	}{
		{name: "Ok200", wantP: &models.Pagination{Limit: 100}, statusCode: http.StatusOK},
		{name: "Ok200Paginated", query: "?limit=5&offset=10", wantP: &models.Pagination{Limit: 5, Offset: 10}, statusCode: http.StatusOK},
		{name: "Err400Limit", query: "?limit=0", statusCode: http.StatusBadRequest},
		{name: "Err401", noToken: true, statusCode: http.StatusUnauthorized},
		{name: "Err500", wantP: &models.Pagination{Limit: 100}, errList: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantP != nil {
				suite.uc.On("ListFollowedMarks", mock.Anything, 1, *tt.wantP).Once().
					Return(models.Page[models.Mark]{Items: []models.Mark{{ID: 3, IsFollowing: true, FollowersCount: 2}}, Total: 1}, tt.errList)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users/me/following"+tt.query, nil)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(models.RoleUser))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var body responses.Response[marksrest.GetFollowedMarksResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body))
				suite.Require().Len(body.Payload.Marks, 1)
				suite.True(body.Payload.Marks[0].IsFollowing)
				suite.Equal(2, body.Payload.Marks[0].FollowersCount)
				suite.Require().NotNil(body.Meta)
				suite.Equal(1, body.Meta.Total)
			}
		})
	}
}

// TestViewerIsRecorded checks that a valid token on a public read endpoint
// is passed on as the viewer, and that a missing or bad token is not.
func (suite *MarksSuite) TestViewerIsRecorded() {
	tests := []struct {
		name       string
		auth       string
		wantViewer int
	}{
		{name: "Anonymous", wantViewer: 0},
		{name: "Authenticated", auth: suite.bearer(models.RoleUser), wantViewer: 1},
		{name: "BadTokenIsAnonymous", auth: "Bearer not-a-token", wantViewer: 0},
		{name: "ExpiredTokenIsAnonymous", auth: suite.expiredBearer(), wantViewer: 0},
		{name: "WrongKeyIsAnonymous", auth: suite.bearerWithKey("wrong"), wantViewer: 0},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetMarkById", mock.MatchedBy(func(ctx context.Context) bool {
				return models.ViewerFromContext(ctx) == tt.wantViewer
			}), 1).Once().Return(models.Mark{ID: 1}, nil)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/1", nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, http.StatusOK)
		})
	}
}

// expiredBearer returns an Authorization header with a token that expired
// a minute ago.
func (suite *MarksSuite) expiredBearer() string {
	accessToken, err := token.CreateToken(-1*time.Minute, 1, string(models.RoleModerator), "1234")
	suite.Require().NoError(err)
	return "Bearer " + accessToken
}

// bearerWithKey returns an Authorization header signed with a foreign key.
func (suite *MarksSuite) bearerWithKey(key string) string {
	accessToken, err := token.CreateToken(1*time.Minute, 1, string(models.RoleModerator), key)
	suite.Require().NoError(err)
	return "Bearer " + accessToken
}

// TestProtectedRoutesRequireValidJWT makes sure OptionalAuth on the /marks
// group does not weaken the routes guarded by the strict middleware: a
// missing, malformed, expired or foreign-signed token yields 401 and the
// usecase is never called (the mocks fail on unexpected calls).
func (suite *MarksSuite) TestProtectedRoutesRequireValidJWT() {
	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/marks"},
		{"PATCH", "/marks/1"},
		{"DELETE", "/marks/1"},
		{"POST", "/marks/1/follow"},
		{"DELETE", "/marks/1/follow"},
		{"POST", "/marks/1/confirm"},
		{"POST", "/marks/1/reject"},
		{"GET", "/users/me/following"},
	}
	auths := []struct {
		name string
		auth string
	}{
		{name: "NoToken"},
		{name: "Malformed", auth: "Bearer not-a-token"},
		{name: "Expired", auth: suite.expiredBearer()},
		{name: "WrongKey", auth: suite.bearerWithKey("wrong")},
	}
	for _, rt := range routes {
		for _, a := range auths {
			suite.Run(rt.method+" "+rt.path+"/"+a.name, func() {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{"description":"x"}`))
				req.Header.Set("Content-Type", "application/json")
				if a.auth != "" {
					req.Header.Set("Authorization", a.auth)
				}

				suite.r.ServeHTTP(w, req)

				handlertest.AssertResponse(suite.T(), w, http.StatusUnauthorized)
			})
		}
	}
}

func ptr[T any](v T) *T { return &v }

func (suite *MarksSuite) TestGetMarkTypes() {
	tests := []struct {
		name            string
		acceptLanguage  string
		wantLang        models.Lang
		types           []models.MarkType
		errGetMarkTypes error
		statusCode      int
		wantBody        string
	}{
		{
			name:       "Ok200DefaultRU",
			wantLang:   models.LangRU,
			types:      []models.MarkType{{ID: 1, Code: "garbage", Name: "Мусор", SLAHours: 72, Active: true}},
			statusCode: 200,
			wantBody:   `{"mark_types":[{"id":1,"mark_type_id":1,"code":"garbage","name":"Мусор","sla_hours":72,"icon":null,"color":null,"active":true,"sort_order":0}]}`,
		},
		{
			name:           "Ok200EN",
			acceptLanguage: "en-US,en;q=0.8,ru;q=0.5",
			wantLang:       models.LangEN,
			types:          []models.MarkType{{ID: 1, Code: "garbage", Name: "Garbage", SLAHours: 72, Icon: null.StringFrom("trash"), Color: null.StringFrom("#ff8800"), Active: true, SortOrder: 2}},
			statusCode:     200,
			wantBody:       `{"mark_types":[{"id":1,"mark_type_id":1,"code":"garbage","name":"Garbage","sla_hours":72,"icon":"trash","color":"#ff8800","active":true,"sort_order":2}]}`,
		},
		{
			name:           "Ok200UnsupportedFallsBackToRU",
			acceptLanguage: "de-DE",
			wantLang:       models.LangRU,
			types:          []models.MarkType{},
			statusCode:     200,
			wantBody:       `{"mark_types":[]}`,
		},
		{
			name:            "Err500",
			wantLang:        models.LangRU,
			errGetMarkTypes: errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			cacheKey := mwcache.Key("GET", "/marks/types", tt.wantLang)
			suite.cacher.
				On("GetBytes", mock.Anything, cacheKey).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, cacheKey, mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			suite.uc.On("GetMarkTypes", mock.Anything, tt.wantLang).Once().
				Return(tt.types, tt.errGetMarkTypes)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/types", nil)
			if tt.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tt.acceptLanguage)
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			suite.Equal([]string{"Accept-Language"}, w.Header().Values("Vary"))
			if tt.wantBody != "" {
				suite.Equal(string(tt.wantLang), w.Header().Get("Content-Language"))
				var resp responses.Response[json.RawMessage]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.JSONEq(tt.wantBody, string(resp.Payload))
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatuses() {
	tests := []struct {
		name               string
		acceptLanguage     string
		wantLang           models.Lang
		statuses           []models.MarkStatus
		errGetMarkStatuses error
		statusCode         int
		wantBody           string
	}{
		{
			name:       "Ok200DefaultRU",
			wantLang:   models.LangRU,
			statuses:   []models.MarkStatus{{ID: 1, Code: "unconfirmed", Name: "Неподтверждённая"}},
			statusCode: 200,
			wantBody:   `{"mark_statuses":[{"id":1,"mark_status_id":1,"parent_id":null,"code":"unconfirmed","name":"Неподтверждённая"}]}`,
		},
		{
			name:           "Ok200EN",
			acceptLanguage: "en",
			wantLang:       models.LangEN,
			statuses:       []models.MarkStatus{{ID: 1, Code: "unconfirmed", Name: "Unconfirmed"}},
			statusCode:     200,
			wantBody:       `{"mark_statuses":[{"id":1,"mark_status_id":1,"parent_id":null,"code":"unconfirmed","name":"Unconfirmed"}]}`,
		},
		{
			name:               "Err500",
			wantLang:           models.LangRU,
			errGetMarkStatuses: errors.New(""),
			statusCode:         500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			cacheKey := mwcache.Key("GET", "/marks/statuses", tt.wantLang)
			suite.cacher.
				On("GetBytes", mock.Anything, cacheKey).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, cacheKey, mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			suite.uc.On("GetMarkStatuses", mock.Anything, tt.wantLang).Once().
				Return(tt.statuses, tt.errGetMarkStatuses)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/statuses", nil)
			if tt.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tt.acceptLanguage)
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.wantBody != "" {
				var resp responses.Response[json.RawMessage]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.JSONEq(tt.wantBody, string(resp.Payload))
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatusHistoryByMarkId() {
	tests := []struct {
		name                            string
		id                              string
		wantErrParseId                  bool
		query                           string
		wantErrParseWithChecks          bool
		errGetMarkStatusHistoryByMarkId error
		statusCode                      int
	}{
		{
			name:       "Ok200",
			id:         "1",
			statusCode: 200,
		},
		{
			name:       "Ok200",
			id:         "1",
			query:      "?withChecks=false",
			statusCode: 200,
		},
		{
			name:       "Ok200",
			id:         "1",
			query:      "?withChecks=true",
			statusCode: 200,
		},
		{
			name:           "Err400-id",
			id:             "a",
			wantErrParseId: true,
			statusCode:     400,
		},
		{
			name:                   "Err400-withChecks",
			id:                     "1",
			query:                  "?withChecks=a",
			wantErrParseWithChecks: true,
			statusCode:             400,
		},
		{
			name:                            "Err500",
			id:                              "1",
			errGetMarkStatusHistoryByMarkId: errors.New(""),
			statusCode:                      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId && !tt.wantErrParseWithChecks {
				suite.uc.On("GetMarkStatusHistoryByMarkId", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("bool")).Once().
					Return([]models.MarkStatusHistoryItem{}, tt.errGetMarkStatusHistoryByMarkId)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/"+tt.id+"/status-history"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestConfirm() {
	tests := []struct {
		name           string
		id             string
		role           models.Role
		noToken        bool
		wantErrParseId bool
		errConfirm     error
		statusCode     int
	}{
		{
			name:       "Ok200Moderator",
			id:         "1",
			role:       models.RoleModerator,
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200Admin",
			id:         "1",
			role:       models.RoleAdmin,
			statusCode: http.StatusOK,
		},
		{
			name:           "Err401NoToken",
			id:             "1",
			noToken:        true,
			wantErrParseId: true,
			statusCode:     http.StatusUnauthorized,
		},
		{
			name:           "Err403User",
			id:             "1",
			role:           models.RoleUser,
			wantErrParseId: true,
			statusCode:     http.StatusForbidden,
		},
		{
			name:           "Ok400",
			id:             "a",
			wantErrParseId: true,
			statusCode:     http.StatusBadRequest,
		},
		{
			name:       "Ok409",
			id:         "1",
			errConfirm: usecase.ErrConflict,
			statusCode: http.StatusConflict,
		},
		{
			name:       "Err500",
			id:         "1",
			errConfirm: errors.New(""),
			statusCode: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.statusUpdater.On("Confirm", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.ConfirmedStatus, tt.errConfirm)
			}
			w := httptest.NewRecorder()

			req := httptest.NewRequest("POST", "/marks/"+tt.id+"/confirm", nil)
			if !tt.noToken {
				role := tt.role
				if role == "" {
					role = models.RoleModerator
				}
				accessToken, err := token.CreateToken(1*time.Minute, 1, string(role), "1234")
				suite.NoError(err)
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MarksSuite) TestReject() {
	tests := []struct {
		name           string
		id             string
		role           models.Role
		noToken        bool
		wantErrParseId bool
		errReject      error
		statusCode     int
	}{
		{
			name:       "Ok200Moderator",
			id:         "1",
			role:       models.RoleModerator,
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200Admin",
			id:         "1",
			role:       models.RoleAdmin,
			statusCode: http.StatusOK,
		},
		{
			name:           "Err401NoToken",
			id:             "1",
			noToken:        true,
			wantErrParseId: true,
			statusCode:     http.StatusUnauthorized,
		},
		{
			name:           "Err403User",
			id:             "1",
			role:           models.RoleUser,
			wantErrParseId: true,
			statusCode:     http.StatusForbidden,
		},
		{
			name:           "Ok400",
			id:             "a",
			wantErrParseId: true,
			statusCode:     http.StatusBadRequest,
		},
		{
			name:       "Ok409",
			id:         "1",
			errReject:  usecase.ErrConflict,
			statusCode: http.StatusConflict,
		},
		{
			name:       "Err404",
			id:         "1",
			errReject:  usecase.ErrNotFound,
			statusCode: http.StatusNotFound,
		},
		{
			name:       "Err500",
			id:         "1",
			errReject:  errors.New(""),
			statusCode: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.statusUpdater.On("Reject", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.ConfirmedStatus, tt.errReject)
			}
			w := httptest.NewRecorder()

			req := httptest.NewRequest("POST", "/marks/"+tt.id+"/reject", nil)
			if !tt.noToken {
				role := tt.role
				if role == "" {
					role = models.RoleModerator
				}
				accessToken, err := token.CreateToken(1*time.Minute, 1, string(role), "1234")
				suite.NoError(err)
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

// The binding tag cannot reference a constant, so keep the REST limit in
// sync with models.MaxMarkDescriptionLen (shared with gRPC) explicitly.
func TestDescriptionLimitMatchesModels(t *testing.T) {
	want := fmt.Sprintf("max=%d", models.MaxMarkDescriptionLen)
	for _, typ := range []reflect.Type{
		reflect.TypeOf(marksrest.AddMarkRequest{}),
		reflect.TypeOf(marksrest.UpdateMarkRequest{}),
	} {
		f, ok := typ.FieldByName("Description")
		require.True(t, ok, typ.Name())
		assert.Contains(t, f.Tag.Get("binding"), want, typ.Name())
	}
}
