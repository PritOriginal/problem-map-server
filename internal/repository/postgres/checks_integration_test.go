//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

// Seeded checks (see seed()):
//
//	1: Bob   on mark 1, history 1, created 2h ago
//	2: Alice on mark 2, history 2, created 3h ago
//	3: Bob   on mark 2, history 4, created 1h ago
const (
	fxCheckBobMark1   = 1
	fxCheckAliceMark2 = 2
	fxCheckBobMark2   = 3

	fxHistoryMark1Initial   = 1
	fxHistoryMark2Confirmed = 4
	fxHistoryMark3Review    = 5
)

func checkIDs(checks []models.Check) []int {
	return ids(checks, func(c models.Check) int { return c.ID })
}

func (s *PostgresSuite) TestChecks_AddCheck() {
	tests := []struct {
		name    string
		check   models.Check
		wantErr error
	}{
		{
			name: "new check for a new history item",
			check: models.Check{
				UserID: fxUserAlice, MarkID: fxMarkFar, MarkStatusId: models.UnderReviewStatus,
				MarkStatusHistoryItemId: fxHistoryMark3Review, Comment: "looks fixed", Result: true,
			},
		},
		{
			name: "same user and history item violates unique_check_per_history",
			check: models.Check{
				UserID: fxUserBob, MarkID: fxMarkNear, MarkStatusId: models.UnconfirmedStatus,
				MarkStatusHistoryItemId: fxHistoryMark1Initial, Comment: "again", Result: false,
			},
			wantErr: repository.ErrExists,
		},
		{
			name: "another user may check the same history item",
			check: models.Check{
				UserID: fxUserAlice, MarkID: fxMarkNear, MarkStatusId: models.UnconfirmedStatus,
				MarkStatusHistoryItemId: fxHistoryMark1Initial, Comment: "me too", Result: true,
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			id, err := s.checks.AddCheck(s.ctx, tt.check)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Greater(id, int64(fxCheckBobMark2))

			got, err := s.checks.GetCheckById(s.ctx, int(id))
			s.Require().NoError(err)
			s.Equal(tt.check.UserID, got.UserID)
			s.Equal(tt.check.MarkID, got.MarkID)
			s.Equal(tt.check.MarkStatusId, got.MarkStatusId)
			s.Equal(tt.check.MarkStatusHistoryItemId, got.MarkStatusHistoryItemId)
			s.Equal(tt.check.Comment, got.Comment)
			s.Equal(tt.check.Result, got.Result)
			s.WithinDuration(time.Now(), got.CreatedAt, time.Minute)
		})
	}
}

func (s *PostgresSuite) TestChecks_AddCheck_UnknownHistoryItem() {
	_, err := s.checks.AddCheck(s.ctx, models.Check{
		UserID: fxUserAlice, MarkID: fxMarkNear, MarkStatusId: models.UnconfirmedStatus,
		MarkStatusHistoryItemId: 999, Comment: "x", Result: true,
	})
	s.Require().Error(err)
	s.NotErrorIs(err, repository.ErrExists)
	s.ErrorContains(err, "fk_checks_mark_status_history")
}

func (s *PostgresSuite) TestChecks_GetCheckById() {
	tests := []struct {
		name         string
		id           int
		wantUser     int
		wantUsername string
		wantMark     int
		wantErr      error
	}{
		{name: "check joined with username", id: fxCheckAliceMark2, wantUser: fxUserAlice, wantUsername: "Alice", wantMark: fxMarkInside},
		{name: "another user", id: fxCheckBobMark1, wantUser: fxUserBob, wantUsername: "Bob", wantMark: fxMarkNear},
		{name: "missing check", id: 404, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.checks.GetCheckById(s.ctx, tt.id)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.id, got.ID)
			s.Equal(tt.wantUser, got.UserID)
			s.Equal(tt.wantUsername, got.Username)
			s.Equal(tt.wantMark, got.MarkID)
			s.False(got.CreatedAt.IsZero())
			s.False(got.UpdatedAt.IsZero())
		})
	}
}

func (s *PostgresSuite) TestChecks_GetChecksByMarkId() {
	tests := []struct {
		name    string
		markID  int
		wantIDs []int // ordered by created_at ASC
	}{
		{name: "mark with two checks ordered by creation", markID: fxMarkInside, wantIDs: []int{fxCheckAliceMark2, fxCheckBobMark2}},
		{name: "mark with one check", markID: fxMarkNear, wantIDs: []int{fxCheckBobMark1}},
		{name: "mark without checks", markID: fxMarkFar, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.checks.GetChecksByMarkId(s.ctx, tt.markID, models.Pagination{})
			s.Require().NoError(err)
			checks := page.Items
			s.NotNil(checks)
			s.Equal(len(tt.wantIDs), page.Total)
			s.Equal(tt.wantIDs, checkIDs(checks))
			for _, c := range checks {
				s.NotEmpty(c.Username)
			}
		})
	}
}

func (s *PostgresSuite) TestChecks_GetChecksByUserId() {
	tests := []struct {
		name    string
		userID  int
		wantIDs []int
	}{
		{name: "bob has two checks", userID: fxUserBob, wantIDs: []int{fxCheckBobMark1, fxCheckBobMark2}},
		{name: "alice has one check", userID: fxUserAlice, wantIDs: []int{fxCheckAliceMark2}},
		{name: "unknown user", userID: 999, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.checks.GetChecksByUserId(s.ctx, tt.userID, models.Pagination{})
			s.Require().NoError(err)
			checks := page.Items
			s.NotNil(checks)
			s.Equal(len(tt.wantIDs), page.Total)
			s.ElementsMatch(tt.wantIDs, checkIDs(checks))
		})
	}
}

func (s *PostgresSuite) TestChecks_GetChecksByMarkHistoryId() {
	tests := []struct {
		name      string
		historyID int
		wantIDs   []int
	}{
		{name: "history item with a check", historyID: fxHistoryMark2Confirmed, wantIDs: []int{fxCheckBobMark2}},
		{name: "history item without checks", historyID: fxHistoryMark3Review, wantIDs: []int{}},
		{name: "unknown history item", historyID: 999, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			checks, err := s.checks.GetChecksByMarkHistoryId(s.ctx, tt.historyID)
			s.Require().NoError(err)
			s.NotNil(checks)
			s.ElementsMatch(tt.wantIDs, checkIDs(checks))
		})
	}
}

func (s *PostgresSuite) TestChecks_GetChecksByUserIdAndMarkId() {
	tests := []struct {
		name    string
		userID  int
		markID  int
		wantIDs []int
	}{
		{name: "match", userID: fxUserBob, markID: fxMarkInside, wantIDs: []int{fxCheckBobMark2}},
		{name: "user checked another mark only", userID: fxUserAlice, markID: fxMarkNear, wantIDs: []int{}},
		{name: "unknown mark", userID: fxUserBob, markID: 404, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			checks, err := s.checks.GetChecksByUserIdAndMarkId(s.ctx, tt.userID, tt.markID)
			s.Require().NoError(err)
			s.NotNil(checks)
			s.ElementsMatch(tt.wantIDs, checkIDs(checks))
		})
	}
}

func (s *PostgresSuite) TestChecks_GetChecksByUserIdAndMarkIdSince() {
	// checks.created_at is TIMESTAMP (without time zone) filled by NOW() in the
	// server's zone (UTC in the container), so the boundary must be UTC too.
	now := time.Now().UTC()
	tests := []struct {
		name    string
		userID  int
		markID  int
		since   time.Time
		wantIDs []int
	}{
		{name: "since long ago includes the check", userID: fxUserBob, markID: fxMarkInside, since: now.Add(-24 * time.Hour), wantIDs: []int{fxCheckBobMark2}},
		{name: "since 90 minutes ago includes the 1h-old check", userID: fxUserBob, markID: fxMarkInside, since: now.Add(-90 * time.Minute), wantIDs: []int{fxCheckBobMark2}},
		{name: "since 30 minutes ago excludes it", userID: fxUserBob, markID: fxMarkInside, since: now.Add(-30 * time.Minute), wantIDs: []int{}},
		{name: "3h-old check is excluded by a 2h window", userID: fxUserAlice, markID: fxMarkInside, since: now.Add(-2 * time.Hour), wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			checks, err := s.checks.GetChecksByUserIdAndMarkIdSince(s.ctx, tt.userID, tt.markID, tt.since)
			s.Require().NoError(err)
			s.NotNil(checks)
			s.ElementsMatch(tt.wantIDs, checkIDs(checks))
		})
	}
}

// TestChecks_GetUserMarkCheck covers the recursive CTE that walks the status
// history chain through parent statuses: a check left on the "unconfirmed"
// row must be found when asking about the child "confirmed" row.
func (s *PostgresSuite) TestChecks_GetUserMarkCheck() {
	// Extend mark 2: confirmed(4) -> under review -> confirmed. Under review is
	// not a parent of confirmed, so the chain from the newest row stops there.
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkInside, models.UnderReviewStatus))
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkInside, models.ConfirmedStatus))
	last, err := s.marks.GetLastMarkStatusHistoryItem(s.ctx, fxMarkInside)
	s.Require().NoError(err)
	s.Require().Equal(models.ConfirmedStatus, last.NewMarkStatusID)

	tests := []struct {
		name      string
		userID    int
		historyID int
		wantCheck int
		wantErr   error
	}{
		{
			name:   "check on the same history row",
			userID: fxUserBob, historyID: fxHistoryMark2Confirmed, wantCheck: fxCheckBobMark2,
		},
		{
			name:   "check on the parent-status row is reachable from the child row",
			userID: fxUserAlice, historyID: fxHistoryMark2Confirmed, wantCheck: fxCheckAliceMark2,
		},
		{
			name:   "walk stops at a non-parent status",
			userID: fxUserBob, historyID: last.ID, wantErr: repository.ErrNotFound,
		},
		{
			name:   "user without checks on the chain",
			userID: fxUserAlice, historyID: fxHistoryMark1Initial, wantErr: repository.ErrNotFound,
		},
		{
			name:   "unknown history row",
			userID: fxUserAlice, historyID: 999, wantErr: repository.ErrNotFound,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.checks.GetUserMarkCheck(s.ctx, tt.userID, tt.historyID)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.wantCheck, got.ID)
			s.Equal(tt.userID, got.UserID)
			s.NotEmpty(got.Username)
		})
	}
}

func (s *PostgresSuite) TestChecks_GetUserMarkCheck_ReturnsLatest() {
	// Two checks by Alice reachable from the same chain: on history 2 (3h old,
	// seeded) and a fresh one on history 4. The newest must win.
	id, err := s.checks.AddCheck(s.ctx, models.Check{
		UserID: fxUserAlice, MarkID: fxMarkInside, MarkStatusId: models.ConfirmedStatus,
		MarkStatusHistoryItemId: fxHistoryMark2Confirmed, Comment: "fresh", Result: true,
	})
	s.Require().NoError(err)

	got, err := s.checks.GetUserMarkCheck(s.ctx, fxUserAlice, fxHistoryMark2Confirmed)
	s.Require().NoError(err)
	s.Equal(int(id), got.ID)
}
