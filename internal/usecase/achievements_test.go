package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AchievementsSuite struct {
	suite.Suite
	uc        *usecase.Achievements
	repo      *usecase.MockAchievementsRepository
	users     *usecase.MockAchievementsUsersRepository
	publisher *events.MockPublisher
}

func (suite *AchievementsSuite) SetupTest() {
	suite.repo = usecase.NewMockAchievementsRepository(suite.T())
	suite.users = usecase.NewMockAchievementsUsersRepository(suite.T())
	suite.publisher = events.NewMockPublisher(suite.T())
	suite.uc = usecase.NewAchievements(slogdiscard.NewDiscardLogger(), usecase.AchievementsRepositories{
		Achievements: suite.repo,
		Users:        suite.users,
	}).WithEvents(suite.publisher)
}

func TestAchievements(t *testing.T) {
	suite.Run(t, new(AchievementsSuite))
}

var testCatalogue = []models.Badge{
	{Code: "first_mark", Name: "Первая метка", Metric: models.MetricMarksTotal, Threshold: 1},
	{Code: "reporter_10", Name: "Репортёр", Metric: models.MetricMarksConfirmed, Threshold: 10},
	{Code: "verifier_10", Name: "Проверяющий", Metric: models.MetricChecksCorrect, Threshold: 10},
	{Code: "helper_5", Name: "Помощник", Metric: models.MetricTasksCompleted, Threshold: 5},
}

func (suite *AchievementsSuite) TestEvaluate() {
	tests := []struct {
		name       string
		metrics    method[models.AchievementMetrics]
		catalogue  method[[]models.Badge]
		wantInsert []string
		added      method[[]string]
		wantCodes  []string
		wantErr    error
	}{
		{
			name:    "NothingEarned",
			metrics: method[models.AchievementMetrics]{data: models.AchievementMetrics{}},
			// The catalogue is read before the insert is skipped.
			catalogue: method[[]models.Badge]{data: testCatalogue},
			wantCodes: []string{},
		},
		{
			name:       "OnlyMissingBadgesAreNew",
			metrics:    method[models.AchievementMetrics]{data: models.AchievementMetrics{MarksTotal: 12, MarksConfirmed: 10, ChecksCorrect: 3}},
			catalogue:  method[[]models.Badge]{data: testCatalogue},
			wantInsert: []string{"first_mark", "reporter_10"},
			// first_mark was earned earlier: the store reports only reporter_10.
			added:     method[[]string]{data: []string{"reporter_10"}},
			wantCodes: []string{"reporter_10"},
		},
		{
			name:       "IdempotentSecondRun",
			metrics:    method[models.AchievementMetrics]{data: models.AchievementMetrics{MarksTotal: 12, MarksConfirmed: 10}},
			catalogue:  method[[]models.Badge]{data: testCatalogue},
			wantInsert: []string{"first_mark", "reporter_10"},
			added:      method[[]string]{data: []string{}},
			wantCodes:  []string{},
		},
		{
			name:       "CatalogueOrderKept",
			metrics:    method[models.AchievementMetrics]{data: models.AchievementMetrics{MarksTotal: 1, ChecksCorrect: 10, TasksCompleted: 5}},
			catalogue:  method[[]models.Badge]{data: testCatalogue},
			wantInsert: []string{"first_mark", "verifier_10", "helper_5"},
			added:      method[[]string]{data: []string{"helper_5", "first_mark", "verifier_10"}},
			wantCodes:  []string{"first_mark", "verifier_10", "helper_5"},
		},
		{
			name:    "ErrMetrics",
			metrics: method[models.AchievementMetrics]{err: errRepo},
			wantErr: errRepo,
		},
		{
			name:      "ErrCatalogue",
			metrics:   method[models.AchievementMetrics]{data: models.AchievementMetrics{MarksTotal: 1}},
			catalogue: method[[]models.Badge]{err: errRepo},
			wantErr:   errRepo,
		},
		{
			name:       "ErrUnknownUser",
			metrics:    method[models.AchievementMetrics]{data: models.AchievementMetrics{MarksTotal: 1}},
			catalogue:  method[[]models.Badge]{data: testCatalogue},
			wantInsert: []string{"first_mark"},
			added:      method[[]string]{err: repository.ErrInvalidReference},
			wantErr:    usecase.ErrInvalidArgument,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("GetAchievementMetrics", mock.Anything, 7).Once().Return(tt.metrics.data, tt.metrics.err)
			if tt.metrics.err == nil {
				suite.repo.On("GetBadges", mock.Anything, models.LangRU).Once().Return(tt.catalogue.data, tt.catalogue.err)
			}
			if tt.wantInsert != nil {
				suite.repo.On("AddUserBadges", mock.Anything, 7, tt.wantInsert).Once().Return(tt.added.data, tt.added.err)
			}
			for _, code := range tt.wantCodes {
				suite.publisher.On("Publish", mock.Anything, events.SubjectBadgeEarned, events.NewBadgeEarned(7, code)).Once().Return(nil)
			}

			got, err := suite.uc.Evaluate(context.Background(), 7)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			codes := make([]string, 0, len(got))
			for _, b := range got {
				codes = append(codes, b.Code)
				suite.NotEmpty(b.Name, "the badge is returned with its texts")
			}
			suite.Equal(tt.wantCodes, codes)
		})
	}
}

func (suite *AchievementsSuite) TestEvaluatePublishFailureIsNotAnError() {
	suite.repo.On("GetAchievementMetrics", mock.Anything, 7).Once().Return(models.AchievementMetrics{MarksTotal: 1}, nil)
	suite.repo.On("GetBadges", mock.Anything, models.LangRU).Once().Return(testCatalogue, nil)
	suite.repo.On("AddUserBadges", mock.Anything, 7, []string{"first_mark"}).Once().Return([]string{"first_mark"}, nil)
	suite.publisher.On("Publish", mock.Anything, events.SubjectBadgeEarned, mock.Anything).Once().Return(errors.New("broker down"))

	got, err := suite.uc.Evaluate(context.Background(), 7)
	suite.NoError(err)
	suite.Len(got, 1)
}

func (suite *AchievementsSuite) TestListBadges() {
	ctx := models.ContextWithLang(context.Background(), models.LangEN)

	suite.repo.On("GetBadges", mock.Anything, models.LangEN).Once().Return(testCatalogue, nil)
	got, err := suite.uc.ListBadges(ctx)
	suite.NoError(err)
	suite.Equal(testCatalogue, got)

	suite.repo.On("GetBadges", mock.Anything, models.LangEN).Once().Return(nil, errRepo)
	_, err = suite.uc.ListBadges(ctx)
	suite.ErrorIs(err, errRepo)
}

func (suite *AchievementsSuite) TestGetProfile() {
	joined := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	user := models.User{Id: 7, Name: "alice", Login: "alice", PasswordHash: "h", Rating: 55, Role: models.RoleUser, CreatedAt: joined}
	stats := models.UserStats{Rating: 55, MarksTotal: 3, ChecksTotal: 4}
	badges := []models.UserBadge{{Badge: testCatalogue[0], EarnedAt: joined}}

	tests := []struct {
		name     string
		lang     models.Lang
		getUser  method[models.User]
		getStats *method[models.UserStats]
		badges   *method[[]models.UserBadge]
		wantErr  error
		wantName string
	}{
		{name: "OkRU", lang: models.LangRU, getUser: method[models.User]{data: user}, getStats: &method[models.UserStats]{data: stats}, badges: &method[[]models.UserBadge]{data: badges}, wantName: "Активист"},
		{name: "OkEN", lang: models.LangEN, getUser: method[models.User]{data: user}, getStats: &method[models.UserStats]{data: stats}, badges: &method[[]models.UserBadge]{data: badges}, wantName: "Activist"},
		{name: "ErrNotFound", lang: models.LangRU, getUser: method[models.User]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{name: "ErrStats", lang: models.LangRU, getUser: method[models.User]{data: user}, getStats: &method[models.UserStats]{err: errRepo}, wantErr: errRepo},
		{name: "ErrBadges", lang: models.LangRU, getUser: method[models.User]{data: user}, getStats: &method[models.UserStats]{data: stats}, badges: &method[[]models.UserBadge]{err: errRepo}, wantErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.users.On("GetUserById", mock.Anything, 7).Once().Return(tt.getUser.data, tt.getUser.err)
			if tt.getStats != nil {
				suite.users.On("GetUserStats", mock.Anything, 7).Once().Return(tt.getStats.data, tt.getStats.err)
			}
			if tt.badges != nil {
				suite.repo.On("GetUserBadges", mock.Anything, 7, tt.lang).Once().Return(tt.badges.data, tt.badges.err)
			}

			got, err := suite.uc.GetProfile(models.ContextWithLang(context.Background(), tt.lang), 7)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(user.Public(), got.User, "private fields are cleared")
			suite.Equal(3, got.Level.Number)
			suite.Equal(tt.wantName, got.Level.Name)
			suite.Equal(badges, got.Badges)
			suite.Equal(stats, got.Stats)
			suite.Equal(joined, got.User.CreatedAt)
		})
	}
}
