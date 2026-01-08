package marksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"

	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-playground/validator/v10"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MarksSuite struct {
	suite.Suite
	r      *chi.Mux
	uc     *marksrest.MockMarks
	cacher *mwcache.MockCacher
}

func (suite *MarksSuite) SetupSuite() {
	accessAuth := jwtauth.New("HS256", []byte("1234"), nil)
	suite.uc = marksrest.NewMockMarks(suite.T())
	suite.cacher = mwcache.NewMockCacher(suite.T())

	log := slogdiscard.NewDiscardLogger()
	validate := validator.New()
	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	suite.r = chi.NewRouter()

	marksrest.Register(suite.r, accessAuth, suite.uc, suite.cacher, baseHandler)
}

func TestMark(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func (suite *MarksSuite) TestGetMarks() {
	tests := []struct {
		name        string
		errGetMarks error
		statusCode  int
	}{
		{
			name:        "Ok200",
			errGetMarks: nil,
			statusCode:  200,
		},
		{
			name:        "Err500",
			errGetMarks: errors.New(""),
			statusCode:  500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetMarks", mock.Anything).Once().
				Return([]models.Mark{}, tt.errGetMarks)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
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
			errGetMarkById: storage.ErrNotFound,
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
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetMarksByUserId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return([]models.Mark{}, tt.errGetMarksByUserId)
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
		rawReq          string
		req             marksrest.AddMarkRequest
		wantErrParseReq bool
		errAddCheck     error
		statusCode      int
	}{
		{
			name: "Ok201",
			req: marksrest.AddMarkRequest{
				Point: marksrest.Point{
					Longitude: 42,
					Latitude:  52,
				},
				TypeMarkID:   1,
				MarkStatusID: 1,
				UserID:       1,
				DistrictID:   1,
			},
			wantErrParseReq: false,
			errAddCheck:     nil,
			statusCode:      201,
		},
		{
			name:            "Err400InvalidJSON",
			rawReq:          "{",
			wantErrParseReq: true,
			errAddCheck:     nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq-1",
			req: marksrest.AddMarkRequest{
				Point: marksrest.Point{
					Longitude: 42,
					Latitude:  52,
				},
				TypeMarkID: 1,
			},
			wantErrParseReq: true,
			errAddCheck:     nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq-2",
			req: marksrest.AddMarkRequest{
				Point: marksrest.Point{
					Longitude: 42,
				},
				TypeMarkID:   1,
				MarkStatusID: 1,
				UserID:       1,
				DistrictID:   1,
			},
			wantErrParseReq: true,
			errAddCheck:     nil,
			statusCode:      400,
		},
		{
			name: "Err500",
			req: marksrest.AddMarkRequest{
				Point: marksrest.Point{
					Longitude: 42,
					Latitude:  52,
				},
				TypeMarkID:   1,
				MarkStatusID: 1,
				UserID:       1,
				DistrictID:   1,
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

			var buf *bytes.Buffer
			if tt.rawReq == "" {
				body, err := json.Marshal(tt.req)
				suite.NoError(err)
				buf = bytes.NewBuffer(body)
			} else {
				buf = bytes.NewBuffer([]byte(tt.rawReq))
			}

			b := &bytes.Buffer{}
			mpw := multipart.NewWriter(b)

			mpw.WriteField("data", buf.String())

			image := gofakeit.ImageJpeg(10, 10)
			fw, err := mpw.CreateFormFile("photo", "test.jpg")
			suite.NoError(err)
			io.Copy(fw, bytes.NewBuffer(image))

			mpw.Close()

			accessToken, err := token.CreateToken(1*time.Minute, 1, "1234")
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
