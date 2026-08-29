package models_test

import (
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/stretchr/testify/suite"
)

type AchievementsSuite struct {
	suite.Suite
}

func TestAchievements(t *testing.T) {
	suite.Run(t, new(AchievementsSuite))
}

func ptr(v int) *int { return &v }

func (s *AchievementsSuite) TestLevelFor() {
	tests := []struct {
		name   string
		rating int
		lang   models.Lang
		want   models.Level
	}{
		{name: "Negative", rating: -10, lang: models.LangRU, want: models.Level{Number: 1, Name: "Новичок", NextThreshold: ptr(20)}},
		{name: "Zero", rating: 0, lang: models.LangRU, want: models.Level{Number: 1, Name: "Новичок", NextThreshold: ptr(20)}},
		{name: "BelowSecond", rating: 19, lang: models.LangRU, want: models.Level{Number: 1, Name: "Новичок", NextThreshold: ptr(20)}},
		{name: "ExactlySecond", rating: 20, lang: models.LangRU, want: models.Level{Number: 2, Name: "Наблюдатель", NextThreshold: ptr(50)}},
		{name: "Third", rating: 50, lang: models.LangEN, want: models.Level{Number: 3, Name: "Activist", NextThreshold: ptr(100)}},
		{name: "Fourth", rating: 199, lang: models.LangEN, want: models.Level{Number: 4, Name: "Expert", NextThreshold: ptr(200)}},
		{name: "Fifth", rating: 200, lang: models.LangRU, want: models.Level{Number: 5, Name: "Мастер", NextThreshold: ptr(400)}},
		{name: "Sixth", rating: 799, lang: models.LangRU, want: models.Level{Number: 6, Name: "Герой", NextThreshold: ptr(800)}},
		{name: "LastHasNoNext", rating: 800, lang: models.LangEN, want: models.Level{Number: 7, Name: "Legend"}},
		{name: "AboveLast", rating: 100000, lang: models.LangRU, want: models.Level{Number: 7, Name: "Легенда"}},
		{name: "UnknownLangFallsBack", rating: 25, lang: "de", want: models.Level{Number: 2, Name: "Наблюдатель", NextThreshold: ptr(50)}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.want, models.LevelFor(tt.rating, tt.lang))
		})
	}
	s.Equal(7, models.MaxLevel)
}

func (s *AchievementsSuite) TestEarnedBadges() {
	catalogue := []models.Badge{
		{Code: "first_mark", Metric: models.MetricMarksTotal, Threshold: 1},
		{Code: "reporter_10", Metric: models.MetricMarksConfirmed, Threshold: 10},
		{Code: "verifier_10", Metric: models.MetricChecksCorrect, Threshold: 10},
		{Code: "streak_7", Metric: models.MetricCheckStreakDays, Threshold: 7},
		{Code: "helper_5", Metric: models.MetricTasksCompleted, Threshold: 5},
		{Code: "resolver", Metric: models.MetricMarksClosed, Threshold: 1},
		{Code: "typo", Metric: "unknown_metric", Threshold: 0},
	}

	tests := []struct {
		name    string
		metrics models.AchievementMetrics
		want    []string
	}{
		{name: "Nothing", metrics: models.AchievementMetrics{}, want: []string{}},
		{name: "FirstMark", metrics: models.AchievementMetrics{MarksTotal: 1}, want: []string{"first_mark"}},
		{name: "BelowThreshold", metrics: models.AchievementMetrics{MarksTotal: 3, MarksConfirmed: 9, ChecksCorrect: 9, CheckStreakDays: 6, TasksCompleted: 4}, want: []string{"first_mark"}},
		{
			name:    "Everything",
			metrics: models.AchievementMetrics{MarksTotal: 50, MarksConfirmed: 10, ChecksCorrect: 100, CheckStreakDays: 7, TasksCompleted: 5, MarksClosed: 1},
			want:    []string{"first_mark", "reporter_10", "verifier_10", "streak_7", "helper_5", "resolver"},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got := models.EarnedBadges(catalogue, tt.metrics)
			codes := make([]string, 0, len(got))
			for _, b := range got {
				codes = append(codes, b.Code)
			}
			s.Equal(tt.want, codes)
		})
	}
}

func (s *AchievementsSuite) TestLeaderboardPeriod() {
	s.NoError(models.LeaderboardPeriod("").Validate())
	s.NoError(models.LeaderboardAll.Validate())
	s.NoError(models.LeaderboardMonth.Validate())
	s.ErrorIs(models.LeaderboardPeriod("year").Validate(), models.ErrInvalidPeriod)

	s.Zero(models.LeaderboardAll.Window())
	s.Greater(models.LeaderboardMonth.Window(), models.LeaderboardWeek.Window())

	s.False(models.LeaderboardFilters{}.Any())
	s.False(models.LeaderboardFilters{Period: models.LeaderboardAll}.Any())
	s.True(models.LeaderboardFilters{BoundaryID: 1}.Any())
	s.True(models.LeaderboardFilters{Period: models.LeaderboardWeek}.Any())
}
