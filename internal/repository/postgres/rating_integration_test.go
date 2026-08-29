//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/guregu/null/v6"
)

func (s *PostgresSuite) userRating(userId int) int {
	var rating int
	s.Require().NoError(s.db.GetContext(s.ctx, &rating, "SELECT rating FROM users WHERE user_id = $1", userId))
	return rating
}

func (s *PostgresSuite) TestUsers_AddRatingEvent() {
	tests := []struct {
		name       string
		event      models.RatingEvent
		wantRating int
		wantErr    error
	}{
		{
			name: "positive delta with mark and check",
			event: models.RatingEvent{
				UserID: fxUserAlice, Delta: 2, Reason: models.RatingReasonCheckCorrect,
				MarkID: null.IntFrom(fxMarkNear), CheckID: null.IntFrom(1),
			},
			wantRating: 12,
		},
		{
			name: "negative delta without check",
			event: models.RatingEvent{
				UserID: fxUserBob, Delta: -2, Reason: models.RatingReasonMarkRefuted,
				MarkID: null.IntFrom(fxMarkFar),
			},
			wantRating: -2,
		},
		{
			name:    "unknown user",
			event:   models.RatingEvent{UserID: 999, Delta: 1, Reason: models.RatingReasonTaskCompleted},
			wantErr: repository.ErrNotFound,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			before := s.countRows("rating_events", "TRUE")

			id, err := s.users.AddRatingEvent(s.ctx, tt.event)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				s.Equal(before, s.countRows("rating_events", "TRUE"), "no event on failure")
				return
			}
			s.Require().NoError(err)
			s.Greater(id, int64(0))

			s.Equal(tt.wantRating, s.userRating(tt.event.UserID), "users.rating is updated in the same call")

			page, err := s.users.GetRatingEvents(s.ctx, tt.event.UserID, models.Pagination{})
			s.Require().NoError(err)
			s.Require().Len(page.Items, 1)
			got := page.Items[0]
			s.Equal(id, got.ID)
			s.Equal(tt.event.UserID, got.UserID)
			s.Equal(tt.event.Delta, got.Delta)
			s.Equal(tt.event.Reason, got.Reason)
			s.Equal(tt.event.MarkID, got.MarkID)
			s.Equal(tt.event.CheckID, got.CheckID)
			s.WithinDuration(time.Now(), got.CreatedAt, time.Minute)
		})
	}
}

func (s *PostgresSuite) TestUsers_AddRatingEvent_RollbackInTransaction() {
	errAbort := errors.New("abort")

	err := s.trm.Do(s.ctx, func(ctx context.Context) error {
		_, err := s.users.AddRatingEvent(ctx, models.RatingEvent{
			UserID: fxUserAlice, Delta: 5, Reason: models.RatingReasonMarkConfirmed, MarkID: null.IntFrom(fxMarkNear),
		})
		s.Require().NoError(err)
		return errAbort
	})
	s.ErrorIs(err, errAbort)

	s.Equal(10, s.userRating(fxUserAlice), "rating change rolled back")
	s.Equal(0, s.countRows("rating_events", "user_id = $1", fxUserAlice))
}

func (s *PostgresSuite) TestUsers_GetRatingEvents_NewestFirstPaginated() {
	for i, delta := range []int{1, 2, 3} {
		_, err := s.db.ExecContext(s.ctx, `
			INSERT INTO rating_events (user_id, delta, reason, mark_id, created_at)
			VALUES ($1, $2, 'check_correct', $3, NOW() - make_interval(hours => $4))
		`, fxUserAlice, delta, fxMarkNear, 3-i)
		s.Require().NoError(err)
	}
	// Another user's event must not leak in.
	_, err := s.users.AddRatingEvent(s.ctx, models.RatingEvent{UserID: fxUserBob, Delta: 9, Reason: models.RatingReasonTaskCompleted})
	s.Require().NoError(err)

	page, err := s.users.GetRatingEvents(s.ctx, fxUserAlice, models.Pagination{Limit: 2})
	s.Require().NoError(err)
	s.Equal(3, page.Total)
	s.Require().Len(page.Items, 2)
	s.Equal([]int{3, 2}, []int{page.Items[0].Delta, page.Items[1].Delta}, "newest first")

	page, err = s.users.GetRatingEvents(s.ctx, fxUserAlice, models.Pagination{Limit: 2, Offset: 2})
	s.Require().NoError(err)
	s.Require().Len(page.Items, 1)
	s.Equal(1, page.Items[0].Delta)
}

func (s *PostgresSuite) TestUsers_GetLeaderboard() {
	_, err := s.users.AddUser(s.ctx, models.User{
		Name: "Carol", Login: "carol", PasswordHash: "x", HomePoint: models.NewPoint(coordAliceHome), Role: models.RoleUser,
	})
	s.Require().NoError(err)
	_, err = s.users.AddRatingEvent(s.ctx, models.RatingEvent{UserID: fxUserBob, Delta: 25, Reason: models.RatingReasonMarkConfirmed})
	s.Require().NoError(err)

	page, err := s.users.GetLeaderboard(s.ctx, models.Pagination{Limit: 2})
	s.Require().NoError(err)
	s.Equal(3, page.Total)
	s.Require().Len(page.Items, 2)
	s.Equal([]int{fxUserBob, fxUserAlice}, ids(page.Items, func(u models.User) int { return u.Id }))
	s.Equal([]int{25, 10}, []int{page.Items[0].Rating, page.Items[1].Rating})
	for _, u := range page.Items {
		s.Empty(u.PasswordHash, "leaderboard must not expose password hashes")
	}
}

func (s *PostgresSuite) TestUsers_GetUserStats() {
	// Fixtures: Alice owns marks 1 (unconfirmed) and 2 (confirmed), has one
	// check and one completed task; Bob owns mark 3 (under review), has two
	// checks and no completed tasks.
	_, err := s.users.AddRatingEvent(s.ctx, models.RatingEvent{
		UserID: fxUserAlice, Delta: 2, Reason: models.RatingReasonCheckCorrect, MarkID: null.IntFrom(fxMarkInside), CheckID: null.IntFrom(2),
	})
	s.Require().NoError(err)
	_, err = s.db.ExecContext(s.ctx, `UPDATE marks SET mark_status_id = $1 WHERE mark_id = $2`, models.RefutedStatus, fxMarkNear)
	s.Require().NoError(err)

	tests := []struct {
		name    string
		userId  int
		want    models.UserStats
		wantErr error
	}{
		{
			name:   "alice",
			userId: fxUserAlice,
			want: models.UserStats{
				Rating: 12, MarksTotal: 2, MarksConfirmed: 1, MarksRefuted: 1,
				ChecksTotal: 1, ChecksCorrect: 1, TasksCompleted: 1,
			},
		},
		{
			name:   "bob",
			userId: fxUserBob,
			want: models.UserStats{
				Rating: 0, MarksTotal: 1, MarksConfirmed: 1, MarksRefuted: 0,
				ChecksTotal: 2, ChecksCorrect: 0, TasksCompleted: 0,
			},
		},
		{name: "missing user", userId: 999, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.users.GetUserStats(s.ctx, tt.userId)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.want, got)
		})
	}
}
