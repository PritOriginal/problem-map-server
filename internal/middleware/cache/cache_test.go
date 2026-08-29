package cache_test

import (
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

func (s *CacheSuite) router(cacher cache.Cacher) (*gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	calls := 0
	r.GET("/data", cache.New(cacher, time.Minute), func(c *gin.Context) {
		calls++
		c.JSON(http.StatusOK, gin.H{"n": calls})
	})
	return r, &calls
}

func (s *CacheSuite) get(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/data", nil))
	return w
}

func (s *CacheSuite) TestNilCacherPassesThrough() {
	r, calls := s.router(nil)

	for i := 1; i <= 2; i++ {
		w := s.get(r)
		s.Equal(http.StatusOK, w.Code)
		s.JSONEq(`{"n":`+string(rune('0'+i))+`}`, w.Body.String())
	}
	s.Equal(2, *calls)
}

func (s *CacheSuite) TestHitAndMiss() {
	cacher := cache.NewMockCacher(s.T())
	r, calls := s.router(cacher)

	cacher.On("GetBytes", mock.Anything, "http:GET:/data:ru").Once().Return(nil, errors.New("miss"))
	cacher.On("Set", mock.Anything, "http:GET:/data:ru", mock.Anything, time.Minute).Once().Return(nil)
	s.JSONEq(`{"n":1}`, s.get(r).Body.String())

	cacher.On("GetBytes", mock.Anything, "http:GET:/data:ru").Once().Return([]byte(`{"n":1}`), nil)
	s.JSONEq(`{"n":1}`, s.get(r).Body.String())

	s.Equal(1, *calls)
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

	cacher.On("GetBytes", mock.Anything, "http:GET:/data:ru").Return(nil, errors.New("redis down"))
	cacher.On("Set", mock.Anything, "http:GET:/data:ru", mock.Anything, time.Minute).Return(errors.New("redis down"))

	s.Equal(http.StatusOK, s.get(r).Code)
	s.Equal(http.StatusOK, s.get(r).Code)
	s.Equal(2, *calls)
}
