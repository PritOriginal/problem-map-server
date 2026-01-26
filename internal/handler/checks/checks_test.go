package checksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"

	checksrest "github.com/PritOriginal/problem-map-server/internal/handler/checks"
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

type ChecksSuite struct {
	suite.Suite
	r  *chi.Mux
	uc *checksrest.MockChecks
}

func (suite *ChecksSuite) SetupSuite() {
	accessAuth := jwtauth.New("HS256", []byte("1234"), nil)
	suite.uc = checksrest.NewMockChecks(suite.T())

	log := slogdiscard.NewDiscardLogger()
	validate := validator.New()
	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	suite.r = chi.NewRouter()

	checksrest.Register(suite.r, accessAuth, suite.uc, baseHandler)
}

func TestUsers(t *testing.T) {
	suite.Run(t, new(ChecksSuite))
}

func (suite *ChecksSuite) TestGetCheckById() {
	tests := []struct {
		name            string
		id              string
		wantErrParseId  bool
		errGetCheckById error
		statusCode      int
	}{
		{
			name:            "Ok200",
			id:              "1",
			wantErrParseId:  false,
			errGetCheckById: nil,
			statusCode:      200,
		},
		{
			name:            "Err500",
			id:              "1",
			wantErrParseId:  false,
			errGetCheckById: errors.New(""),
			statusCode:      500,
		},
		{
			name:            "Err400",
			id:              "a",
			wantErrParseId:  true,
			errGetCheckById: nil,
			statusCode:      400,
		},
		{
			name:            "Err404",
			id:              "1",
			wantErrParseId:  false,
			errGetCheckById: storage.ErrNotFound,
			statusCode:      404,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetCheckById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(models.Check{}, tt.errGetCheckById)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/checks/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByMarkId() {
	tests := []struct {
		name                 string
		id                   string
		wantErrParseId       bool
		errGetChecksByMarkId error
		statusCode           int
	}{
		{
			name:                 "Ok200",
			id:                   "1",
			wantErrParseId:       false,
			errGetChecksByMarkId: nil,
			statusCode:           200,
		},
		{
			name:                 "Err500",
			id:                   "1",
			wantErrParseId:       false,
			errGetChecksByMarkId: errors.New(""),
			statusCode:           500,
		},
		{
			name:                 "Err400",
			id:                   "a",
			wantErrParseId:       true,
			errGetChecksByMarkId: nil,
			statusCode:           400,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetChecksByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return([]models.Check{}, tt.errGetChecksByMarkId)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/checks/mark/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *ChecksSuite) TestGetChecksByUserId() {
	tests := []struct {
		name                 string
		id                   string
		wantErrParseId       bool
		errGetChecksByUserId error
		statusCode           int
	}{
		{
			name:                 "Ok200",
			id:                   "1",
			wantErrParseId:       false,
			errGetChecksByUserId: nil,
			statusCode:           200,
		},
		{
			name:                 "Err500",
			id:                   "1",
			wantErrParseId:       false,
			errGetChecksByUserId: errors.New(""),
			statusCode:           500,
		},
		{
			name:                 "Err400",
			id:                   "a",
			wantErrParseId:       true,
			errGetChecksByUserId: nil,
			statusCode:           400,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetChecksByUserId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return([]models.Check{}, tt.errGetChecksByUserId)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/checks/user/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *ChecksSuite) TestAddCheck() {
	tests := []struct {
		name            string
		rawReq          string
		req             checksrest.AddCheckRequest
		wantErrParseReq bool
		errAddCheck     error
		statusCode      int
	}{
		{
			name: "Ok201",
			req: checksrest.AddCheckRequest{
				MarkID:  1,
				Result:  true,
				Comment: "",
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
			name: "Err400InvalidReq",
			req: checksrest.AddCheckRequest{
				Result:  true,
				Comment: "",
			},
			wantErrParseReq: true,
			errAddCheck:     nil,
			statusCode:      400,
		},
		{
			name: "Err500",
			req: checksrest.AddCheckRequest{
				MarkID:  1,
				Result:  true,
				Comment: "",
			},
			wantErrParseReq: false,
			errAddCheck:     errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseReq {
				suite.uc.On("AddCheck", mock.Anything, mock.Anything, mock.Anything).Once().
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

			req := httptest.NewRequest("POST", "/checks", b)
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("Content-Type", mpw.FormDataContentType())

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}
