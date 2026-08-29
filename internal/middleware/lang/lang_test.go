package lang_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/middleware/lang"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

type LangSuite struct {
	suite.Suite
}

func TestLang(t *testing.T) {
	suite.Run(t, new(LangSuite))
}

func (s *LangSuite) TestResolvesLanguage() {
	tests := []struct {
		name   string
		header string
		want   models.Lang
	}{
		{name: "Missing", header: "", want: models.LangRU},
		{name: "Ru", header: "ru", want: models.LangRU},
		{name: "En", header: "en", want: models.LangEN},
		{name: "EnRegion", header: "en-US", want: models.LangEN},
		{name: "Weighted", header: "ru;q=0.5, en;q=0.9", want: models.LangEN},
		{name: "FirstWins", header: "en, ru", want: models.LangEN},
		{name: "UnsupportedSkipped", header: "de, fr;q=0.9, ru;q=0.1", want: models.LangRU},
		{name: "OnlyUnsupported", header: "de-DE", want: models.LangRU},
		{name: "ZeroQualityIgnored", header: "en;q=0, ru", want: models.LangRU},
		{name: "Garbage", header: ";;;,,,q=", want: models.LangRU},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			var got models.Lang
			r.GET("/", lang.New(), func(c *gin.Context) {
				got = models.LangFromContext(c.Request.Context())
				c.Status(http.StatusNoContent)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(lang.Header, tt.header)
			}
			r.ServeHTTP(w, req)

			s.Equal(tt.want, got)
			s.Equal(string(tt.want), w.Header().Get("Content-Language"))
			s.Contains(w.Header().Values("Vary"), lang.Header)
		})
	}
}

func (s *LangSuite) TestLangFromContextDefault() {
	s.Equal(models.DefaultLang, models.LangFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()))
}
