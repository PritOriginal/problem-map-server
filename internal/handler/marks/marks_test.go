package marksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MarksSuite struct {
	suite.Suite
	r             *gin.Engine
	uc            *marksrest.MockMarks
	statusUpdater *marksrest.MockStatusUpdater
	cacher        *mwcache.MockCacher
}

func (suite *MarksSuite) SetupSuite() {
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

			suite.Equal(tt.statusCode, w.Code)
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
				MarkTypeIds:   []int{},
				MarkStatusIds: []int{},
				Pagination:    models.Pagination{Limit: models.DefaultLimit},
			},
			statusCode: http.StatusOK,
		},
		{
			name:  "AllFilters",
			query: "?bbox=41.4,52.7,41.5,52.8&limit=50&offset=100&sort=updated_at&order=asc&user_id=7&created_from=2025-01-02T03:04:05Z&created_to=2025-02-01T00:00:00Z&mark_type_ids=1,2",
			wantFilters: models.GetMarksFilters{
				MarkTypeIds:   []int{1, 2},
				MarkStatusIds: []int{},
				UserID:        7,
				BBox:          &models.BBox{MinLon: 41.4, MinLat: 52.7, MaxLon: 41.5, MaxLat: 52.8},
				CreatedFrom:   createdFrom,
				CreatedTo:     createdTo,
				Sort:          models.MarksSortUpdatedAt,
				Order:         models.SortAsc,
				Pagination:    models.Pagination{Limit: 50, Offset: 100},
			},
			statusCode: http.StatusOK,
		},
		{name: "ErrBBoxThreeParts", query: "?bbox=1,2,3", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxNotNumber", query: "?bbox=a,2,3,4", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxMinGreaterThanMax", query: "?bbox=41.5,52.7,41.4,52.8", statusCode: http.StatusBadRequest},
		{name: "ErrBBoxOutOfRange", query: "?bbox=-181,52.7,41.4,52.8", statusCode: http.StatusBadRequest},
		{name: "ErrLimitTooBig", query: "?limit=501", statusCode: http.StatusBadRequest},
		{name: "ErrLimitZero", query: "?limit=0", statusCode: http.StatusBadRequest},
		{name: "ErrOffsetNegative", query: "?offset=-5", statusCode: http.StatusBadRequest},
		{name: "ErrSort", query: "?sort=description", statusCode: http.StatusBadRequest},
		{name: "ErrOrder", query: "?order=random", statusCode: http.StatusBadRequest},
		{name: "ErrUserIdNegative", query: "?user_id=-1", statusCode: http.StatusBadRequest},
		{name: "ErrCreatedFromFormat", query: "?created_from=2025-01-02", statusCode: http.StatusBadRequest},
		{name: "ErrCreatedToFormat", query: "?created_to=yesterday", statusCode: http.StatusBadRequest},
		{name: "ErrCreatedRange", query: "?created_from=2025-02-01T00:00:00Z&created_to=2025-01-01T00:00:00Z", statusCode: http.StatusBadRequest},
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
			errGetMarkById: repository.ErrNotFound,
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

			suite.Equal(tt.statusCode, w.Code)
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

			suite.Equal(tt.statusCode, w.Code)
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
				suite.uc.On("AddMark", mock.Anything, mock.Anything, mock.Anything).Once().
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

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *MarksSuite) TestGetMarkTypes() {
	tests := []struct {
		name            string
		errGetMarkTypes error
		statusCode      int
	}{
		{
			name:            "Ok200",
			errGetMarkTypes: nil,
			statusCode:      200,
		},
		{
			name:            "Err500",
			errGetMarkTypes: errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			suite.uc.On("GetMarkTypes", mock.Anything).Once().
				Return([]models.MarkType{}, tt.errGetMarkTypes)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/types", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatuses() {
	tests := []struct {
		name               string
		errGetMarkStatuses error
		statusCode         int
	}{
		{
			name:               "Ok200",
			errGetMarkStatuses: nil,
			statusCode:         200,
		},
		{
			name:               "Err500",
			errGetMarkStatuses: errors.New(""),
			statusCode:         500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			suite.uc.On("GetMarkStatuses", mock.Anything).Once().
				Return([]models.MarkStatus{}, tt.errGetMarkStatuses)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/statuses", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
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

			suite.Equal(tt.statusCode, w.Code)
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

			suite.Equal(tt.statusCode, w.Code)
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

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}
