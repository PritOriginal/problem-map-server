//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

func (s *PostgresSuite) TestMarks_FollowerFields() {
	tests := []struct {
		name          string
		viewer        int
		markID        int
		wantCount     int
		wantFollowing bool
	}{
		{name: "author sees own subscription", viewer: fxUserAlice, markID: fxMarkNear, wantCount: 1, wantFollowing: true},
		{name: "other user is not following", viewer: fxUserBob, markID: fxMarkNear, wantCount: 1, wantFollowing: false},
		{name: "anonymous is never following", viewer: 0, markID: fxMarkNear, wantCount: 1, wantFollowing: false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			ctx := models.ContextWithViewer(s.ctx, tt.viewer)

			got, err := s.marks.GetMarkById(ctx, tt.markID)
			s.Require().NoError(err)
			s.Equal(tt.wantCount, got.FollowersCount)
			s.Equal(tt.wantFollowing, got.IsFollowing)

			// The list queries share the column set with GetMarkById.
			page, err := s.marks.GetMarks(ctx, models.GetMarksFilters{})
			s.Require().NoError(err)
			for _, m := range page.Items {
				if m.ID == tt.markID {
					s.Equal(tt.wantCount, m.FollowersCount)
					s.Equal(tt.wantFollowing, m.IsFollowing)
				}
			}
		})
	}
}

func (s *PostgresSuite) TestMarks_GetSimilarMarks() {
	tests := []struct {
		name    string
		prepare func()
		filters models.GetSimilarMarksFilters
		wantIDs []int
	}{
		{
			name:    "same type within radius",
			filters: models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: 50},
			wantIDs: []int{fxMarkNear},
		},
		{
			name:    "other type is not similar",
			filters: models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 2, RadiusM: 50},
			wantIDs: []int{},
		},
		{
			name:    "outside radius",
			filters: models.GetSimilarMarksFilters{Lon: coordMarkNear.X() + 0.01, Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: 50},
			wantIDs: []int{},
		},
		{
			name:    "large radius orders by distance",
			filters: models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: models.MaxNearbyRadiusM},
			wantIDs: []int{fxMarkNear, fxMarkFar},
		},
		{
			name:    "excluded mark is skipped",
			filters: models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: 50, ExcludeMarkID: fxMarkNear},
			wantIDs: []int{},
		},
		{
			name: "closed mark is ignored",
			prepare: func() {
				_, err := s.db.ExecContext(s.ctx, "UPDATE marks SET mark_status_id = $1 WHERE mark_id = $2", models.ClosedStatus, fxMarkNear)
				s.Require().NoError(err)
			},
			filters: models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: 50},
			wantIDs: []int{},
		},
		{
			name: "refuted mark is ignored",
			prepare: func() {
				_, err := s.db.ExecContext(s.ctx, "UPDATE marks SET mark_status_id = $1 WHERE mark_id = $2", models.RefutedStatus, fxMarkNear)
				s.Require().NoError(err)
			},
			filters: models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: 50},
			wantIDs: []int{},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.SetupTest()
			if tt.prepare != nil {
				tt.prepare()
			}

			got, err := s.marks.GetSimilarMarks(models.ContextWithViewer(s.ctx, fxUserAlice), tt.filters)
			s.Require().NoError(err)
			s.NotNil(got)
			s.Equal(tt.wantIDs, ids(got, func(m models.MarkWithDistance) int { return m.ID }))
			for i, m := range got {
				s.GreaterOrEqual(m.DistanceM, 0.0)
				s.LessOrEqual(m.DistanceM, tt.filters.RadiusM)
				if i > 0 {
					s.LessOrEqual(got[i-1].DistanceM, m.DistanceM)
				}
				s.Equal(m.UserID == fxUserAlice, m.IsFollowing)
			}
		})
	}
}

func (s *PostgresSuite) TestMarks_FollowUnfollow() {
	// Bob follows Alice's mark; following twice is idempotent.
	s.Require().NoError(s.marks.FollowMark(s.ctx, fxUserBob, fxMarkNear))
	s.Require().NoError(s.marks.FollowMark(s.ctx, fxUserBob, fxMarkNear))
	s.Equal(2, s.countRows("mark_followers", "mark_id = $1", fxMarkNear))

	followers, err := s.marks.GetFollowerIDs(s.ctx, fxMarkNear)
	s.Require().NoError(err)
	s.Equal([]int{fxUserAlice, fxUserBob}, followers)

	got, err := s.marks.GetMarkById(models.ContextWithViewer(s.ctx, fxUserBob), fxMarkNear)
	s.Require().NoError(err)
	s.Equal(2, got.FollowersCount)
	s.True(got.IsFollowing)

	// Unknown mark violates the foreign key.
	s.ErrorIs(s.marks.FollowMark(s.ctx, fxUserBob, 404), repository.ErrNotFound)

	// Unfollow is idempotent as well.
	s.Require().NoError(s.marks.UnfollowMark(s.ctx, fxUserBob, fxMarkNear))
	s.Require().NoError(s.marks.UnfollowMark(s.ctx, fxUserBob, fxMarkNear))
	s.Equal(1, s.countRows("mark_followers", "mark_id = $1", fxMarkNear))

	followers, err = s.marks.GetFollowerIDs(s.ctx, 404)
	s.Require().NoError(err)
	s.Empty(followers)
	s.NotNil(followers)
}

func (s *PostgresSuite) TestMarks_GetFollowedMarks() {
	// Bob additionally follows mark 1 after his own mark 3, so it comes first.
	s.Require().NoError(s.marks.FollowMark(s.ctx, fxUserBob, fxMarkNear))

	tests := []struct {
		name      string
		userID    int
		p         models.Pagination
		wantIDs   []int
		wantTotal int
	}{
		{name: "alice follows her marks", userID: fxUserAlice, wantIDs: []int{fxMarkInside, fxMarkNear}, wantTotal: 2},
		{name: "bob newest subscription first", userID: fxUserBob, wantIDs: []int{fxMarkNear, fxMarkFar}, wantTotal: 2},
		{name: "paginated", userID: fxUserBob, p: models.Pagination{Limit: 1, Offset: 1}, wantIDs: []int{fxMarkFar}, wantTotal: 2},
		{name: "unknown user", userID: 999, wantIDs: []int{}, wantTotal: 0},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.marks.GetFollowedMarks(models.ContextWithViewer(s.ctx, tt.userID), tt.userID, tt.p)
			s.Require().NoError(err)
			s.NotNil(page.Items)
			s.Equal(tt.wantIDs, markIDs(page.Items))
			s.Equal(tt.wantTotal, page.Total)
			for _, m := range page.Items {
				s.True(m.IsFollowing)
			}
		})
	}
}

func (s *PostgresSuite) TestMarks_UpdateMark() {
	desc := "Обновлено"
	typeID := 3

	tests := []struct {
		name    string
		id      int
		upd     models.MarkUpdate
		wantErr error
	}{
		{name: "description only", id: fxMarkNear, upd: models.MarkUpdate{Description: &desc}},
		{name: "type only", id: fxMarkNear, upd: models.MarkUpdate{MarkTypeID: &typeID}},
		{name: "both", id: fxMarkInside, upd: models.MarkUpdate{Description: &desc, MarkTypeID: &typeID}},
		{name: "missing mark", id: 404, upd: models.MarkUpdate{Description: &desc}, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.SetupTest()
			// Push updated_at into the past so the bump is observable.
			_, err := s.db.ExecContext(s.ctx, "UPDATE marks SET updated_at = NOW() - INTERVAL '1 hour'")
			s.Require().NoError(err)
			historyRows := s.countRows("mark_status_history", "mark_id = $1", tt.id)

			err = s.marks.UpdateMark(s.ctx, tt.id, tt.upd)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)

			before, err := s.marks.GetMarkById(s.ctx, tt.id)
			s.Require().NoError(err)
			if tt.upd.Description != nil {
				s.Equal(desc, before.Description)
			}
			if tt.upd.MarkTypeID != nil {
				s.Equal(typeID, before.MarkTypeID)
			} else {
				s.Equal(1, before.MarkTypeID)
			}
			s.WithinDuration(time.Now(), before.UpdatedAt, time.Minute)
			s.True(before.UpdatedAt.After(before.CreatedAt.Add(-time.Minute)))
			// Status is untouched, so the trigger writes no history.
			s.Equal(historyRows, s.countRows("mark_status_history", "mark_id = $1", tt.id))
		})
	}
}

func (s *PostgresSuite) TestMarks_DeleteMark_Cascade() {
	// Mark 2 has two checks (history 2 and 4), a task, two history rows and
	// one follower; everything else must survive.
	s.Require().NoError(s.marks.FollowMark(s.ctx, fxUserBob, fxMarkInside))

	err := s.trm.Do(s.ctx, func(ctx context.Context) error {
		return s.marks.DeleteMark(ctx, fxMarkInside)
	})
	s.Require().NoError(err)

	_, err = s.marks.GetMarkById(s.ctx, fxMarkInside)
	s.ErrorIs(err, repository.ErrNotFound)
	for _, table := range []string{"checks", "tasks", "mark_status_history", "mark_followers"} {
		s.Equal(0, s.countRows(table, "mark_id = $1", fxMarkInside), table)
	}

	// Rows of the other marks are intact.
	s.Equal(2, s.countRows("marks", "TRUE"))
	s.Equal(1, s.countRows("checks", "TRUE"))
	s.Equal(2, s.countRows("tasks", "TRUE"))
	s.Equal(3, s.countRows("mark_status_history", "TRUE"))
	s.Equal(2, s.countRows("mark_followers", "TRUE"))

	// Missing mark: nothing to delete, the transaction is rolled back.
	err = s.trm.Do(s.ctx, func(ctx context.Context) error {
		return s.marks.DeleteMark(ctx, 404)
	})
	s.ErrorIs(err, repository.ErrNotFound)
	s.Equal(2, s.countRows("marks", "TRUE"))
	s.Equal(0, s.countRows("mark_tombstones", "mark_id = $1", 404), "no tombstone for a missing mark")
}

func (s *PostgresSuite) TestMarks_DeleteMark_Tombstone() {
	before := time.Now().Add(-time.Second)

	err := s.trm.Do(s.ctx, func(ctx context.Context) error {
		return s.marks.DeleteMark(ctx, fxMarkInside)
	})
	s.Require().NoError(err)

	s.Equal(1, s.countRows("mark_tombstones", "mark_id = $1", fxMarkInside))

	deleted, err := s.marks.GetDeletedMarkIDs(s.ctx, before, models.Pagination{})
	s.Require().NoError(err)
	s.Equal([]int{fxMarkInside}, deleted.Items)
	s.Equal(1, deleted.Total)

	// An empty page beyond the first still carries the total.
	deleted, err = s.marks.GetDeletedMarkIDs(s.ctx, before, models.Pagination{Limit: 10, Offset: 10})
	s.Require().NoError(err)
	s.Empty(deleted.Items)
	s.Equal(1, deleted.Total)

	// Nothing was deleted after "now": an empty (not nil) slice.
	deleted, err = s.marks.GetDeletedMarkIDs(s.ctx, time.Now().Add(time.Minute), models.Pagination{})
	s.Require().NoError(err)
	s.NotNil(deleted.Items)
	s.Empty(deleted.Items)
	s.Equal(0, deleted.Total)

	// A rolled-back deletion leaves no tombstone behind.
	errRollback := errors.New("rollback")
	err = s.trm.Do(s.ctx, func(ctx context.Context) error {
		if err := s.marks.DeleteMark(ctx, fxMarkNear); err != nil {
			return err
		}
		return errRollback
	})
	s.ErrorIs(err, errRollback)
	s.Equal(0, s.countRows("mark_tombstones", "mark_id = $1", fxMarkNear))
	_, err = s.marks.GetMarkById(s.ctx, fxMarkNear)
	s.NoError(err)
}

func (s *PostgresSuite) TestMarks_GetHiddenMarkIDs() {
	// Backdate every mark, then hide one: only it changed since.
	_, err := s.db.ExecContext(s.ctx, "UPDATE marks SET updated_at = NOW() - INTERVAL '1 hour'")
	s.Require().NoError(err)
	since := time.Now().Add(-time.Minute)

	hidden, err := s.marks.GetHiddenMarkIDs(s.ctx, since, models.Pagination{})
	s.Require().NoError(err)
	s.NotNil(hidden.Items)
	s.Empty(hidden.Items)
	s.Equal(0, hidden.Total)

	s.Require().NoError(s.marks.SetMarkHidden(s.ctx, fxMarkInside, true))

	hidden, err = s.marks.GetHiddenMarkIDs(s.ctx, since, models.Pagination{})
	s.Require().NoError(err)
	s.Equal([]int{fxMarkInside}, hidden.Items)
	s.Equal(1, hidden.Total)

	// An empty page beyond the first still carries the total.
	hidden, err = s.marks.GetHiddenMarkIDs(s.ctx, since, models.Pagination{Limit: 10, Offset: 10})
	s.Require().NoError(err)
	s.Empty(hidden.Items)
	s.Equal(1, hidden.Total)

	// A mark hidden before since is not a change; showing it again is one
	// but it is no longer hidden.
	s.Require().NoError(s.marks.SetMarkHidden(s.ctx, fxMarkInside, false))
	hidden, err = s.marks.GetHiddenMarkIDs(s.ctx, since, models.Pagination{})
	s.Require().NoError(err)
	s.Empty(hidden.Items)
}

func (s *PostgresSuite) TestMarks_GetMarks_UpdatedSince() {
	// Backdate every mark, then touch one: only it is "changed since".
	_, err := s.db.ExecContext(s.ctx, "UPDATE marks SET updated_at = NOW() - INTERVAL '1 hour'")
	s.Require().NoError(err)
	since := time.Now().Add(-time.Minute)
	touched := "touched"
	s.Require().NoError(s.marks.UpdateMark(s.ctx, fxMarkFar, models.MarkUpdate{Description: &touched}))

	page, err := s.marks.GetMarks(s.ctx, models.GetMarksFilters{
		UpdatedSince: since, Sort: models.MarksSortUpdatedAt, Order: models.SortAsc,
	})
	s.Require().NoError(err)
	s.Equal(1, page.Total)
	s.Equal([]int{fxMarkFar}, markIDs(page.Items))

	// The bound is strict: a mark updated exactly at since is not returned.
	exact := page.Items[0].UpdatedAt
	page, err = s.marks.GetMarks(s.ctx, models.GetMarksFilters{UpdatedSince: exact})
	s.Require().NoError(err)
	s.Equal(0, page.Total)
}
