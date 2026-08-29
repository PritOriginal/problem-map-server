package achievementsrest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	achievementsrest "github.com/PritOriginal/problem-map-server/internal/handler/achievements"
	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	"github.com/PritOriginal/problem-map-server/internal/middleware/lang"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const testKey = "1234"

type AchievementsSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *achievementsrest.MockAchievements
}

func (suite *AchievementsSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = achievementsrest.NewMockAchievements(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	suite.r.Use(lang.New())
	achievementsrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestAchievements(t *testing.T) {
	suite.Run(t, new(AchievementsSuite))
}

func testProfile() models.UserProfile {
	joined := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	return models.UserProfile{
		User:  models.User{Id: 1, Name: "name", Rating: 25},
		Level: models.LevelFor(25, models.LangRU),
		Badges: []models.UserBadge{{
			Badge:    models.Badge{Code: "first_mark", Name: "Первая метка", Description: "d", Icon: "flag", Threshold: 1, Metric: models.MetricMarksTotal},
			EarnedAt: joined,
		}},
		Stats:       models.UserStats{Rating: 25, MarksTotal: 3, MarksConfirmed: 2, MarksRefuted: 1, ChecksTotal: 4, ChecksCorrect: 2, TasksCompleted: 1},
		MemberSince: joined,
	}
}

func (suite *AchievementsSuite) assertProfile(w *httptest.ResponseRecorder) {
	var resp responses.Response[map[string]map[string]any]
	suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
	p := resp.Payload["profile"]
	suite.Equal(float64(1), p["user_id"])
	suite.Equal("name", p["username"])
	suite.Equal(float64(25), p["rating"])
	suite.Equal(map[string]any{"number": float64(2), "name": "Наблюдатель", "next_threshold": float64(50)}, p["level"])
	suite.Equal("2025-03-01T12:00:00Z", p["member_since"])
	suite.Equal(map[string]any{
		"marks_total": float64(3), "marks_confirmed": float64(2), "checks_total": float64(4),
		"checks_correct": float64(2), "tasks_completed": float64(1),
	}, p["stats"])
	badges, ok := p["badges"].([]any)
	suite.Require().True(ok)
	suite.Require().Len(badges, 1)
	suite.Equal(map[string]any{
		"code": "first_mark", "name": "Первая метка", "description": "d", "icon": "flag", "earned_at": "2025-03-01T12:00:00Z",
	}, badges[0])
	suite.NotContains(p, "login")
}

func (suite *AchievementsSuite) TestGetProfile() {
	tests := []struct {
		name       string
		id         string
		err        error
		statusCode int
	}{
		{name: "Ok200", id: "1", statusCode: 200},
		{name: "Err400", id: "a", statusCode: 400},
		{name: "Err404", id: "1", err: usecase.ErrNotFound, statusCode: 404},
		{name: "Err500", id: "1", err: errors.New(""), statusCode: 500},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode != 400 {
				suite.uc.On("GetProfile", mock.Anything, 1).Once().Return(testProfile(), tt.err)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/users/"+tt.id+"/profile", nil)
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == 200 {
				suite.assertProfile(w)
			}
		})
	}
}

func (suite *AchievementsSuite) TestGetMyProfile() {
	tests := []struct {
		name       string
		noToken    bool
		err        error
		statusCode int
	}{
		{name: "Ok200", statusCode: 200},
		{name: "Err401", noToken: true, statusCode: 401},
		{name: "Err404", err: usecase.ErrNotFound, statusCode: 404},
		{name: "Err500", err: errors.New(""), statusCode: 500},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.noToken {
				suite.uc.On("GetProfile", mock.Anything, 1).Once().Return(testProfile(), tt.err)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/users/me/profile", nil)
			if !tt.noToken {
				accessToken, err := token.CreateToken(time.Minute, 1, string(models.RoleUser), testKey)
				suite.Require().NoError(err)
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == 200 {
				suite.assertProfile(w)
			}
		})
	}
}

func (suite *AchievementsSuite) TestGetBadges() {
	tests := []struct {
		name       string
		lang       string
		err        error
		statusCode int
	}{
		{name: "Ok200", statusCode: 200},
		{name: "Ok200EN", lang: "en", statusCode: 200},
		{name: "Err500", err: errors.New(""), statusCode: 500},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			wantLang := models.LangRU
			if tt.lang != "" {
				wantLang = models.Lang(tt.lang)
			}
			suite.uc.On("ListBadges", mock.MatchedBy(func(ctx context.Context) bool {
				return models.LangFromContext(ctx) == wantLang
			})).Once().Return([]models.Badge{{Code: "first_mark", Name: "n", Description: "d", Icon: "flag", Threshold: 1, Metric: models.MetricMarksTotal}}, tt.err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/badges", nil)
			if tt.lang != "" {
				req.Header.Set("Accept-Language", tt.lang)
			}
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == 200 {
				var resp responses.Response[map[string][]map[string]any]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Require().Len(resp.Payload["badges"], 1)
				suite.Equal(map[string]any{
					"code": "first_mark", "name": "n", "description": "d", "icon": "flag", "threshold": float64(1), "metric": "marks_total",
				}, resp.Payload["badges"][0])
			}
		})
	}
}
