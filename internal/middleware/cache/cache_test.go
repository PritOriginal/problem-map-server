package cache_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/cache"
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

	cacher.On("GetBytes", mock.Anything, "http:GET:/data").Once().Return(nil, errors.New("miss"))
	cacher.On("Set", mock.Anything, "http:GET:/data", mock.Anything, time.Minute).Once().Return(nil)
	s.JSONEq(`{"n":1}`, s.get(r).Body.String())

	cacher.On("GetBytes", mock.Anything, "http:GET:/data").Once().Return([]byte(`{"n":1}`), nil)
	s.JSONEq(`{"n":1}`, s.get(r).Body.String())

	s.Equal(1, *calls)
}

func (s *CacheSuite) TestBackendErrorFailsOpen() {
	cacher := cache.NewMockCacher(s.T())
	r, calls := s.router(cacher)

	cacher.On("GetBytes", mock.Anything, "http:GET:/data").Return(nil, errors.New("redis down"))
	cacher.On("Set", mock.Anything, "http:GET:/data", mock.Anything, time.Minute).Return(errors.New("redis down"))

	s.Equal(http.StatusOK, s.get(r).Code)
	s.Equal(http.StatusOK, s.get(r).Code)
	s.Equal(2, *calls)
}
