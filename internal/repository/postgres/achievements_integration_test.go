//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/guregu/null/v6"
)

// badgeCodes lists the catalogue codes seeded by migration 000042.
var badgeCodes = []string{"first_mark", "reporter_10", "reporter_50", "verifier_10", "verifier_100", "streak_7", "helper_5", "resolver"}

func (s *PostgresSuite) TestAchievements_GetBadges() {
	ru, err := s.achievements.GetBadges(s.ctx, models.LangRU)
	s.Require().NoError(err)
	s.ElementsMatch(badgeCodes, ids(ru, func(b models.Badge) string { return b.Code }))

	en, err := s.achievements.GetBadges(s.ctx, models.LangEN)
	s.Require().NoError(err)
	s.Require().Len(en, len(ru))

	byCode := map[string]models.Badge{}
	for _, b := range ru {
		byCode[b.Code] = b
	}
	s.Equal("Первая метка", byCode["first_mark"].Name)
	s.Equal(models.MetricMarksTotal, byCode["first_mark"].Metric)
	s.Equal(1, byCode["first_mark"].Threshold)
	s.Equal(models.MetricCheckStreakDays, byCode["streak_7"].Metric)
	s.Equal(7, byCode["streak_7"].Threshold)

	for _, b := range en {
		if b.Code == "first_mark" {
			s.Equal("First mark", b.Name, "english texts for en")
		}
	}
	for _, b := range ru {
		s.NotEmpty(b.Name)
		s.NotEmpty(b.Description)
		s.NotEmpty(b.Icon)
		_, ok := models.AchievementMetrics{}.Value(b.Metric)
		s.True(ok, "catalogue metric %q is known to the code", b.Metric)
	}
}

func (s *PostgresSuite) TestAchievements_AddUserBadges_Idempotent() {
	added, err := s.achievements.AddUserBadges(s.ctx, fxUserAlice, []string{"first_mark", "helper_5"})
	s.Require().NoError(err)
	s.ElementsMatch([]string{"first_mark", "helper_5"}, added)

	// Only the missing badge is new on the second call.
	added, err = s.achievements.AddUserBadges(s.ctx, fxUserAlice, []string{"first_mark", "helper_5", "resolver"})
	s.Require().NoError(err)
	s.Equal([]string{"resolver"}, added)

	added, err = s.achievements.AddUserBadges(s.ctx, fxUserAlice, nil)
	s.Require().NoError(err)
	s.Empty(added)

	badges, err := s.achievements.GetUserBadges(s.ctx, fxUserAlice, models.LangEN)
	s.Require().NoError(err)
	s.Equal([]string{"first_mark", "helper_5", "resolver"}, ids(badges, func(b models.UserBadge) string { return b.Code }))
	s.Equal("First mark", badges[0].Name)
	s.WithinDuration(time.Now(), badges[0].EarnedAt, time.Minute)

	other, err := s.achievements.GetUserBadges(s.ctx, fxUserBob, models.LangRU)
	s.Require().NoError(err)
	s.Empty(other, "badges of another user do not leak")

	_, err = s.achievements.AddUserBadges(s.ctx, fxUserAlice, []string{"no_such_badge"})
	s.ErrorIs(err, repository.ErrInvalidReference)
	_, err = s.achievements.AddUserBadges(s.ctx, 999, []string{"first_mark"})
	s.ErrorIs(err, repository.ErrInvalidReference)
}

func (s *PostgresSuite) TestAchievements_GetAchievementMetrics() {
	// Fixtures: Alice owns marks 1 (unconfirmed) and 2 (confirmed), has one
	// check (3 h ago) and one completed task; Bob owns mark 3 (under
	// review), has two checks today and no completed tasks.
	_, err := s.users.AddRatingEvent(s.ctx, models.RatingEvent{
		UserID: fxUserAlice, Delta: 2, Reason: models.RatingReasonCheckCorrect, MarkID: null.IntFrom(fxMarkInside), CheckID: null.IntFrom(2),
	})
	s.Require().NoError(err)
	_, err = s.users.AddRatingEvent(s.ctx, models.RatingEvent{
		UserID: fxUserAlice, Delta: -1, Reason: models.RatingReasonCheckWrong, MarkID: null.IntFrom(fxMarkInside), CheckID: null.IntFrom(2),
	})
	s.Require().NoError(err)
	_, err = s.db.ExecContext(s.ctx, `UPDATE marks SET mark_status_id = $1 WHERE mark_id = $2`, models.ClosedStatus, fxMarkInside)
	s.Require().NoError(err)

	// Bob checks on 4 consecutive days, a gap, then 2 more days: the
	// longest streak is 4 (today's fixture checks join the first run). One
	// check per voting stage is allowed, so every check gets its own mark
	// (owned by Bob, which raises his marks_total to 6).
	for _, daysAgo := range []int{1, 2, 3, 6, 7} {
		var markId int
		s.Require().NoError(s.db.GetContext(s.ctx, &markId, `
			INSERT INTO marks (description, geom, type_mark_id, user_id)
			VALUES ('streak', ST_SetSRID(ST_MakePoint($1, $2), 4326), 1, $3) RETURNING mark_id
		`, coordMarkFar.X(), coordMarkFar.Y(), fxUserBob))
		_, err := s.db.ExecContext(s.ctx, `
			INSERT INTO checks (user_id, mark_id, mark_status_id, mark_status_history_id, comment, result, created_at)
			SELECT $1, $2, 1, h.id, 'streak', true, NOW() - make_interval(days => $3)
			FROM mark_status_history h WHERE h.mark_id = $2
		`, fxUserBob, markId, daysAgo)
		s.Require().NoError(err)
	}

	tests := []struct {
		name   string
		userId int
		want   models.AchievementMetrics
	}{
		{
			name:   "alice",
			userId: fxUserAlice,
			want:   models.AchievementMetrics{MarksTotal: 2, MarksConfirmed: 1, ChecksCorrect: 1, CheckStreakDays: 1, TasksCompleted: 1, MarksClosed: 1},
		},
		{
			name:   "bob",
			userId: fxUserBob,
			want:   models.AchievementMetrics{MarksTotal: 6, MarksConfirmed: 1, CheckStreakDays: 4},
		},
		{name: "unknown user has zero metrics", userId: 999},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.achievements.GetAchievementMetrics(s.ctx, tt.userId)
			s.Require().NoError(err)
			s.Equal(tt.want, got)
		})
	}
}

func (s *PostgresSuite) TestUsers_GetLeaderboard_Filters() {
	// Alice: +5 on mark 1 (inside "Центр", 10 days ago), +4 on mark 3
	// (outside, today); Bob: +3 on mark 2 (inside, today), +9 without a
	// mark (3 days ago). Ratings in users: Alice 10, Bob 0 (fixtures).
	for _, e := range []struct {
		user, delta int
		mark        null.Int
		daysAgo     int
	}{
		{fxUserAlice, 5, null.IntFrom(fxMarkNear), 10},
		{fxUserAlice, 4, null.IntFrom(fxMarkFar), 0},
		{fxUserBob, 3, null.IntFrom(fxMarkInside), 0},
		{fxUserBob, 9, null.Int{}, 3},
	} {
		_, err := s.db.ExecContext(s.ctx, `
			INSERT INTO rating_events (user_id, delta, reason, mark_id, created_at)
			VALUES ($1, $2, 'check_correct', $3, NOW() - make_interval(days => $4))
		`, e.user, e.delta, e.mark, e.daysAgo)
		s.Require().NoError(err)
	}
	_, err := s.achievements.AddUserBadges(s.ctx, fxUserBob, []string{"first_mark", "helper_5"})
	s.Require().NoError(err)

	type row struct{ user, rating, badges int }
	rows := func(items []models.LeaderboardEntry) []row {
		out := make([]row, 0, len(items))
		for _, e := range items {
			out = append(out, row{e.UserID, e.Rating, e.BadgesCount})
		}
		return out
	}

	tests := []struct {
		name    string
		filters models.LeaderboardFilters
		want    []row
	}{
		{name: "all is users.rating", want: []row{{fxUserAlice, 10, 0}, {fxUserBob, 0, 2}}},
		{name: "explicit all", filters: models.LeaderboardFilters{Period: models.LeaderboardAll}, want: []row{{fxUserAlice, 10, 0}, {fxUserBob, 0, 2}}},
		{name: "boundary sums events of marks inside", filters: models.LeaderboardFilters{BoundaryID: fxBoundaryMain}, want: []row{{fxUserAlice, 5, 0}, {fxUserBob, 3, 2}}},
		{name: "empty boundary lists nobody", filters: models.LeaderboardFilters{BoundaryID: fxBoundaryVoid}, want: []row{}},
		{name: "week", filters: models.LeaderboardFilters{Period: models.LeaderboardWeek}, want: []row{{fxUserBob, 12, 2}, {fxUserAlice, 4, 0}}},
		{name: "month", filters: models.LeaderboardFilters{Period: models.LeaderboardMonth}, want: []row{{fxUserBob, 12, 2}, {fxUserAlice, 9, 0}}},
		{name: "boundary and week", filters: models.LeaderboardFilters{BoundaryID: fxBoundaryMain, Period: models.LeaderboardWeek}, want: []row{{fxUserBob, 3, 2}}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.users.GetLeaderboard(s.ctx, tt.filters, models.Pagination{Limit: 10})
			s.Require().NoError(err)
			s.Equal(tt.want, rows(page.Items))
			s.Equal(len(tt.want), page.Total)
		})
	}

	// Pagination of the filtered variant, including an offset past the end.
	page, err := s.users.GetLeaderboard(s.ctx, models.LeaderboardFilters{Period: models.LeaderboardMonth}, models.Pagination{Limit: 1, Offset: 1})
	s.Require().NoError(err)
	s.Equal([]row{{fxUserAlice, 9, 0}}, rows(page.Items))
	s.Equal(2, page.Total)
	page, err = s.users.GetLeaderboard(s.ctx, models.LeaderboardFilters{Period: models.LeaderboardMonth}, models.Pagination{Limit: 1, Offset: 5})
	s.Require().NoError(err)
	s.Empty(page.Items)
	s.Equal(2, page.Total)
}

func (s *PostgresSuite) TestUsers_CreatedAt() {
	user, err := s.users.GetUserById(s.ctx, fxUserAlice)
	s.Require().NoError(err)
	s.WithinDuration(time.Now(), user.CreatedAt, time.Minute, "users.created_at defaults to now")
}
