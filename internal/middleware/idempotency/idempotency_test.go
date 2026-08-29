package idempotency_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/idempotency"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	testKey    = "1234"
	testUserID = 7
	idemKey    = "6f1b4b2e-3c1d-4e8a-9f0b-1c2d3e4f5a6b"
)

var storeKey = idempotency.Key(testUserID, idemKey)

type IdempotencySuite struct {
	suite.Suite
}

func TestIdempotency(t *testing.T) {
	suite.Run(t, new(IdempotencySuite))
}

// router mounts POST /items behind the JWT and idempotency middlewares. The
// handler answers 201 with a counter of its invocations (or the status
// requested through ?status=), so a replay is told apart from a new call.
func (s *IdempotencySuite) router(store idempotency.Store) (*gin.Engine, *int) {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	s.Require().NoError(err)
	s.Require().NoError(authMiddleware.MiddlewareInit())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	calls := 0
	mw := idempotency.New(slogdiscard.NewDiscardLogger(), store, idempotency.Config{TTL: time.Hour, LockTTL: time.Minute})
	r.Use(gin.Recovery())
	r.POST("/items", authMiddleware.MiddlewareFunc(), mw, func(c *gin.Context) {
		calls++
		if c.Query("status") == "500" {
			responses.Internal(c, "boom")
			return
		}
		if c.Query("status") == "panic" {
			panic("boom")
		}
		c.Header("Location", "/items/1")
		responses.Created(c, gin.H{"calls": calls})
	})
	return r, &calls
}

type request struct {
	body        string
	contentType string
	key         string
	query       string
	noAuth      bool
}

func (s *IdempotencySuite) do(r *gin.Engine, req request) *httptest.ResponseRecorder {
	httpReq := httptest.NewRequest(http.MethodPost, "/items"+req.query, strings.NewReader(req.body))
	if req.contentType != "" {
		httpReq.Header.Set("Content-Type", req.contentType)
	} else {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.key != "" {
		httpReq.Header.Set(idempotency.Header, req.key)
	}
	if !req.noAuth {
		accessToken, err := token.CreateToken(time.Minute, testUserID, string(models.RoleUser), testKey)
		s.Require().NoError(err)
		httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httpReq)
	return w
}

// stored decodes the record written through Set.
func stored(args mock.Arguments) map[string]any {
	var rec map[string]any
	if err := json.Unmarshal(args.Get(2).([]byte), &rec); err != nil {
		panic(err)
	}
	return rec
}

func (s *IdempotencySuite) TestNoHeaderPassesThrough() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	for range 2 {
		w := s.do(r, request{body: `{"a":1}`})
		s.Equal(http.StatusCreated, w.Code)
		s.Empty(w.Header().Get(idempotency.ReplayedHeader))
	}
	s.Equal(2, *calls)
}

func (s *IdempotencySuite) TestInvalidKey() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	tests := []struct {
		name string
		key  string
	}{
		{name: "NotUUID", key: "order-1"},
		{name: "TooLong", key: strings.Repeat("a", 65)},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			w := s.do(r, request{body: `{}`, key: tt.key})
			s.Equal(http.StatusBadRequest, w.Code)
		})
	}
	s.Equal(0, *calls)
}

func (s *IdempotencySuite) TestFirstRequestStoresResponse() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	var lock, saved map[string]any
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
	store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().
		Run(func(args mock.Arguments) { lock = stored(args) }).Return(true, nil)
	store.On("Set", mock.Anything, storeKey, mock.Anything, time.Hour).Once().
		Run(func(args mock.Arguments) { saved = stored(args) }).Return(nil)

	w := s.do(r, request{body: `{"a":1}`, key: idemKey})

	s.Equal(http.StatusCreated, w.Code)
	s.Equal(1, *calls)
	s.Empty(w.Header().Get(idempotency.ReplayedHeader))

	s.Equal(true, lock["in_progress"])
	s.Equal(lock["fingerprint"], saved["fingerprint"])
	s.EqualValues(http.StatusCreated, saved["status"])
	headers := saved["headers"].(map[string]any)
	s.Equal("/items/1", headers["Location"])
	s.Contains(headers["Content-Type"], "application/json")
}

func (s *IdempotencySuite) TestReplay() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	// First call: capture what gets stored.
	var saved []byte
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
	store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().Return(true, nil)
	store.On("Set", mock.Anything, storeKey, mock.Anything, time.Hour).Once().
		Run(func(args mock.Arguments) { saved = args.Get(2).([]byte) }).Return(nil)
	first := s.do(r, request{body: `{"a":1}`, key: idemKey})
	s.Equal(http.StatusCreated, first.Code)

	// Second call: the stored record is replayed, the handler is not run.
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(saved, nil)
	second := s.do(r, request{body: `{"a":1}`, key: idemKey})

	s.Equal(http.StatusCreated, second.Code)
	s.Equal("true", second.Header().Get(idempotency.ReplayedHeader))
	s.Equal("/items/1", second.Header().Get("Location"))
	s.Contains(second.Header().Get("Content-Type"), "application/json")
	s.JSONEq(first.Body.String(), second.Body.String())
	s.Equal(1, *calls)
}

func (s *IdempotencySuite) TestConcurrentRepeatIsConflict() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	var lock []byte
	// The first request holds the lock ...
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
	store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().
		Run(func(args mock.Arguments) { lock = args.Get(2).([]byte) }).Return(true, nil)
	store.On("Set", mock.Anything, storeKey, mock.Anything, time.Hour).Once().Return(nil)
	s.Equal(http.StatusCreated, s.do(r, request{body: `{"a":1}`, key: idemKey}).Code)

	tests := []struct {
		name  string
		setup func()
	}{
		{
			name: "LockSeenOnRead",
			setup: func() {
				store.On("GetBytes", mock.Anything, storeKey).Once().Return(lock, nil)
			},
		},
		{
			name: "LostRaceOnSetNX",
			setup: func() {
				store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
				store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().Return(false, nil)
				store.On("GetBytes", mock.Anything, storeKey).Once().Return(lock, nil)
			},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.setup()
			w := s.do(r, request{body: `{"a":1}`, key: idemKey})
			s.Equal(http.StatusConflict, w.Code)
			s.Contains(w.Body.String(), idempotency.MsgInProgress)
		})
	}
	s.Equal(1, *calls)
}

func (s *IdempotencySuite) TestDifferentPayloadIsRejected() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	var saved []byte
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
	store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().Return(true, nil)
	store.On("Set", mock.Anything, storeKey, mock.Anything, time.Hour).Once().
		Run(func(args mock.Arguments) { saved = args.Get(2).([]byte) }).Return(nil)
	s.Equal(http.StatusCreated, s.do(r, request{body: `{"a":1}`, key: idemKey}).Code)

	tests := []struct {
		name string
		req  request
	}{
		{name: "OtherBody", req: request{body: `{"a":2}`, key: idemKey}},
		{name: "OtherQuery", req: request{body: `{"a":1}`, key: idemKey, query: "?force=true"}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			store.On("GetBytes", mock.Anything, storeKey).Once().Return(saved, nil)
			w := s.do(r, tt.req)
			s.Equal(http.StatusUnprocessableEntity, w.Code)
			s.Contains(w.Body.String(), idempotency.MsgPayloadMismatch)
		})
	}
	s.Equal(1, *calls)
}

func (s *IdempotencySuite) TestMultipartFingerprintIgnoresFiles() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	multipartBody := func(desc string, file []byte) (string, string) {
		var b bytes.Buffer
		mpw := multipart.NewWriter(&b)
		s.Require().NoError(mpw.WriteField("description", desc))
		s.Require().NoError(mpw.WriteField("mark_type_id", "1"))
		fw, err := mpw.CreateFormFile("photos", "p.jpg")
		s.Require().NoError(err)
		_, err = fw.Write(file)
		s.Require().NoError(err)
		s.Require().NoError(mpw.Close())
		return b.String(), mpw.FormDataContentType()
	}

	var saved []byte
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
	store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().Return(true, nil)
	store.On("Set", mock.Anything, storeKey, mock.Anything, time.Hour).Once().
		Run(func(args mock.Arguments) { saved = args.Get(2).([]byte) }).Return(nil)
	body, ct := multipartBody("hole", []byte("img-1"))
	s.Equal(http.StatusCreated, s.do(r, request{body: body, contentType: ct, key: idemKey}).Code)

	// Same fields, another file: replayed.
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(saved, nil)
	body, ct = multipartBody("hole", []byte("img-2"))
	w := s.do(r, request{body: body, contentType: ct, key: idemKey})
	s.Equal(http.StatusCreated, w.Code)
	s.Equal("true", w.Header().Get(idempotency.ReplayedHeader))

	// Other fields: rejected.
	store.On("GetBytes", mock.Anything, storeKey).Once().Return(saved, nil)
	body, ct = multipartBody("crack", []byte("img-1"))
	w = s.do(r, request{body: body, contentType: ct, key: idemKey})
	s.Equal(http.StatusUnprocessableEntity, w.Code)

	s.Equal(1, *calls)
}

func (s *IdempotencySuite) TestServerErrorReleasesKey() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
	store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().Return(true, nil)
	store.On("Del", mock.Anything, storeKey).Once().Return(nil)

	w := s.do(r, request{body: `{}`, key: idemKey, query: "?status=500"})
	s.Equal(http.StatusInternalServerError, w.Code)
	s.Equal(1, *calls)
}

func (s *IdempotencySuite) TestPanicReleasesKey() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	store.On("GetBytes", mock.Anything, storeKey).Once().Return(nil, repository.ErrNotFound)
	store.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Once().Return(true, nil)
	store.On("Del", mock.Anything, storeKey).Once().Return(nil)

	w := s.do(r, request{body: `{}`, key: idemKey, query: "?status=panic"})
	s.Equal(http.StatusInternalServerError, w.Code)
	s.Equal(1, *calls)
}

func (s *IdempotencySuite) TestStoreDownFailsOpen() {
	tests := []struct {
		name  string
		store func() idempotency.Store
	}{
		{name: "NilStore", store: func() idempotency.Store { return nil }},
		{
			name: "GetFails",
			store: func() idempotency.Store {
				m := idempotency.NewMockStore(s.T())
				m.On("GetBytes", mock.Anything, storeKey).Return(nil, errors.New("redis down"))
				return m
			},
		},
		{
			name: "SetNXFails",
			store: func() idempotency.Store {
				m := idempotency.NewMockStore(s.T())
				m.On("GetBytes", mock.Anything, storeKey).Return(nil, repository.ErrNotFound)
				m.On("SetNX", mock.Anything, storeKey, mock.Anything, time.Minute).Return(false, errors.New("redis down"))
				return m
			},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			r, calls := s.router(tt.store())
			for range 2 {
				w := s.do(r, request{body: `{}`, key: idemKey})
				s.Equal(http.StatusCreated, w.Code)
				s.Empty(w.Header().Get(idempotency.ReplayedHeader))
			}
			s.Equal(2, *calls)
		})
	}
}

func (s *IdempotencySuite) TestUnauthenticatedIsRejectedBeforeStore() {
	store := idempotency.NewMockStore(s.T())
	r, calls := s.router(store)

	w := s.do(r, request{body: `{}`, key: idemKey, noAuth: true})
	s.Equal(http.StatusUnauthorized, w.Code)
	s.Equal(0, *calls)
}
