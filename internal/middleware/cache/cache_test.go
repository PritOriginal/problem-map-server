package cache_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/middleware/lang"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type CacheSuite struct {
	suite.Suite
}

func TestCache(t *testing.T) {
	suite.Run(t, new(CacheSuite))
}

const cacheKey = "http:GET:/data:ru"

func (s *CacheSuite) router(cacher cache.Cacher, opts ...cache.Option) (*gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	calls := 0
	r.GET("/data", cache.New(cacher, time.Minute, opts...), func(c *gin.Context) {
		calls++
		switch c.Query("status") {
		case "500":
			c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
		case "204":
			c.Status(http.StatusNoContent)
		default:
			c.JSON(http.StatusOK, gin.H{"n": calls})
		}
	})
	return r, &calls
}

func (s *CacheSuite) get(r *gin.Engine, headers ...string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	r.ServeHTTP(w, req)
	return w
}

// stored decodes the entry passed to Set.
func stored(args mock.Arguments) map[string]any {
	var e map[string]any
	if err := json.Unmarshal(args.Get(2).([]byte), &e); err != nil {
		panic(err)
	}
	return e
}

func (s *CacheSuite) TestNilCacherPassesThroughWithETag() {
	r, calls := s.router(nil)

	first := s.get(r)
	s.Equal(http.StatusOK, first.Code)
	s.JSONEq(`{"n":1}`, first.Body.String())
	s.Equal(cache.ETag([]byte(`{"n":1}`)), first.Header().Get("ETag"))
	s.Equal("public, max-age=60", first.Header().Get("Cache-Control"))

	second := s.get(r)
	s.JSONEq(`{"n":2}`, second.Body.String())
	s.Equal(2, *calls)
}

func (s *CacheSuite) TestHitAndMiss() {
	cacher := cache.NewMockCacher(s.T())
	r, calls := s.router(cacher)

	var saved []byte
	cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return(nil, errors.New("miss"))
	cacher.On("Set", mock.Anything, cacheKey, mock.Anything, time.Minute).Once().
		Run(func(args mock.Arguments) {
			saved = args.Get(2).([]byte)
			e := stored(args)
			s.Equal(cache.ETag([]byte(`{"n":1}`)), e["etag"])
			s.Contains(e["content_type"], "application/json")
		}).Return(nil)
	miss := s.get(r)
	s.JSONEq(`{"n":1}`, miss.Body.String())

	cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return(saved, nil)
	hit := s.get(r)
	s.Equal(http.StatusOK, hit.Code)
	s.JSONEq(`{"n":1}`, hit.Body.String())
	s.Contains(hit.Header().Get("Content-Type"), "application/json")
	s.Equal(miss.Header().Get("ETag"), hit.Header().Get("ETag"))
	s.Equal("public, max-age=60", hit.Header().Get("Cache-Control"))

	s.Equal(1, *calls)
}

func (s *CacheSuite) TestLegacyRawEntryIsMiss() {
	cacher := cache.NewMockCacher(s.T())
	r, calls := s.router(cacher)

	// A value written by the previous format (raw body) is not trusted.
	cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return([]byte(`{"n":42}`), nil)
	cacher.On("Set", mock.Anything, cacheKey, mock.Anything, time.Minute).Once().Return(nil)

	w := s.get(r)
	s.JSONEq(`{"n":1}`, w.Body.String())
	s.Equal(1, *calls)
}

func (s *CacheSuite) TestNotModified() {
	cacher := cache.NewMockCacher(s.T())
	r, calls := s.router(cacher)

	var saved []byte
	cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return(nil, errors.New("miss"))
	cacher.On("Set", mock.Anything, cacheKey, mock.Anything, time.Minute).Once().
		Run(func(args mock.Arguments) { saved = args.Get(2).([]byte) }).Return(nil)
	etag := s.get(r).Header().Get("ETag")
	s.NotEmpty(etag)

	// Without a cache hit the handler runs again and answers {"n":2}, so the
	// validator of that (fresh) body is what a client would have to present.
	etagSecond := cache.ETag([]byte(`{"n":2}`))

	tests := []struct {
		name        string
		ifNoneMatch string
		cached      bool
		wantETag    string
		wantStatus  int
	}{
		{name: "MatchFromCache", ifNoneMatch: etag, cached: true, wantETag: etag, wantStatus: http.StatusNotModified},
		{name: "MatchFromHandler", ifNoneMatch: etagSecond, cached: false, wantETag: etagSecond, wantStatus: http.StatusNotModified},
		{name: "WeakMatch", ifNoneMatch: "W/" + etag, cached: true, wantETag: etag, wantStatus: http.StatusNotModified},
		{name: "ListMatch", ifNoneMatch: `"other", ` + etag, cached: true, wantETag: etag, wantStatus: http.StatusNotModified},
		{name: "Star", ifNoneMatch: "*", cached: true, wantETag: etag, wantStatus: http.StatusNotModified},
		{name: "Mismatch", ifNoneMatch: `"stale"`, cached: true, wantETag: etag, wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			if tt.cached {
				cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return(saved, nil)
			} else {
				cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return(nil, errors.New("miss"))
				cacher.On("Set", mock.Anything, cacheKey, mock.Anything, time.Minute).Once().Return(nil)
			}

			w := s.get(r, "If-None-Match", tt.ifNoneMatch)

			s.Equal(tt.wantStatus, w.Code)
			s.Equal(tt.wantETag, w.Header().Get("ETag"))
			s.Equal("public, max-age=60", w.Header().Get("Cache-Control"))
			if tt.wantStatus == http.StatusNotModified {
				s.Empty(w.Body.Bytes())
			} else {
				s.JSONEq(`{"n":1}`, w.Body.String())
			}
		})
	}
	// The handler ran once for the first fill and once more for MatchFromHandler.
	s.Equal(2, *calls)
}

func (s *CacheSuite) TestErrorsAreNotCachedNorTagged() {
	cacher := cache.NewMockCacher(s.T())
	r, _ := s.router(cacher)

	cacher.On("GetBytes", mock.Anything, "http:GET:/data?status=500:ru").Once().Return(nil, errors.New("miss"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/data?status=500", nil))

	s.Equal(http.StatusInternalServerError, w.Code)
	s.JSONEq(`{"error":"boom"}`, w.Body.String())
	s.Empty(w.Header().Get("ETag"))
	s.Empty(w.Header().Get("Cache-Control"))
}

func (s *CacheSuite) TestWithMaxAge() {
	r, _ := s.router(nil, cache.WithMaxAge(24*time.Hour))
	s.Equal("public, max-age=86400", s.get(r).Header().Get("Cache-Control"))
}

func (s *CacheSuite) TestKeyIncludesLanguage() {
	cacher := cache.NewMockCacher(s.T())
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/data", lang.New(), cache.New(cacher, time.Minute), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"lang": models.LangFromContext(c.Request.Context())})
	})

	cacher.On("GetBytes", mock.Anything, "http:GET:/data:en").Once().Return(nil, errors.New("miss"))
	cacher.On("Set", mock.Anything, "http:GET:/data:en", mock.Anything, time.Minute).Once().Return(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("Accept-Language", "en")
	r.ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.JSONEq(`{"lang":"en"}`, w.Body.String())
	s.Contains(w.Header().Values("Vary"), "Accept-Language")
}

func (s *CacheSuite) TestBackendErrorFailsOpen() {
	cacher := cache.NewMockCacher(s.T())
	r, calls := s.router(cacher)

	cacher.On("GetBytes", mock.Anything, cacheKey).Return(nil, errors.New("redis down"))
	cacher.On("Set", mock.Anything, cacheKey, mock.Anything, time.Minute).Return(errors.New("redis down"))

	s.Equal(http.StatusOK, s.get(r).Code)
	s.Equal(http.StatusOK, s.get(r).Code)
	s.Equal(2, *calls)
}

func (s *CacheSuite) TestMatches() {
	tests := []struct {
		name        string
		ifNoneMatch string
		want        bool
	}{
		{name: "Empty", ifNoneMatch: "", want: false},
		{name: "Exact", ifNoneMatch: `"a"`, want: true},
		{name: "Weak", ifNoneMatch: `W/"a"`, want: true},
		{name: "List", ifNoneMatch: `"x", "a"`, want: true},
		{name: "Star", ifNoneMatch: "*", want: true},
		{name: "Other", ifNoneMatch: `"b"`, want: false},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.want, cache.Matches(tt.ifNoneMatch, `"a"`))
		})
	}
}
