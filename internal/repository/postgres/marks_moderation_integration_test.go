//go:build integration

package postgres_test

import (
	"context"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

// TestMarks_HiddenVisibility: a hidden mark leaves every public list, map
// and statistic, but stays visible to its author and to moderators; the
// single-row read is unfiltered (the use case decides).
func (s *PostgresSuite) TestMarks_HiddenVisibility() {
	s.Require().NoError(s.marks.SetMarkHidden(s.ctx, fxMarkNear, true))

	mark, err := s.marks.GetMarkById(s.ctx, fxMarkNear)
	s.Require().NoError(err)
	s.True(mark.Hidden)
	s.False(mark.MergedIntoID.Valid)

	anonymous := s.ctx
	author := models.ContextWithViewer(s.ctx, fxUserAlice)
	stranger := models.ContextWithViewer(s.ctx, fxUserBob)
	moderator := models.ContextWithActor(s.ctx, models.Actor{UserID: fxUserBob, Role: models.RoleModerator})
	admin := models.ContextWithActor(s.ctx, models.Actor{UserID: 999, Role: models.RoleAdmin})

	viewers := []struct {
		name   string
		ctx    context.Context
		wantIt bool
	}{
		{name: "anonymous", ctx: anonymous},
		{name: "stranger", ctx: stranger},
		{name: "author", ctx: author, wantIt: true},
		{name: "moderator", ctx: moderator, wantIt: true},
		{name: "admin", ctx: admin, wantIt: true},
	}

	for _, v := range viewers {
		s.Run("GetMarks/"+v.name, func() {
			page, err := s.marks.GetMarks(v.ctx, models.GetMarksFilters{})
			s.Require().NoError(err)
			s.Equal(v.wantIt, contains(markIDs(page.Items), fxMarkNear))
			if v.wantIt {
				s.Equal(3, page.Total)
			} else {
				s.Equal(2, page.Total)
			}
		})
		s.Run("CountMarks/"+v.name, func() {
			n, err := s.marks.CountMarks(v.ctx, models.GetMarksFilters{})
			s.Require().NoError(err)
			if v.wantIt {
				s.Equal(3, n)
			} else {
				s.Equal(2, n)
			}
		})
		s.Run("GetMarksNearby/"+v.name, func() {
			page, err := s.marks.GetMarksNearby(v.ctx, models.GetMarksNearbyFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), RadiusM: 10})
			s.Require().NoError(err)
			s.Equal(v.wantIt, contains(ids(page.Items, func(m models.MarkWithDistance) int { return m.ID }), fxMarkNear))
		})
		s.Run("GetSimilarMarks/"+v.name, func() {
			similar, err := s.marks.GetSimilarMarks(v.ctx, models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: 10})
			s.Require().NoError(err)
			s.Equal(v.wantIt, contains(ids(similar, func(m models.MarkWithDistance) int { return m.ID }), fxMarkNear))
		})
		s.Run("GetMarksByUserId/"+v.name, func() {
			page, err := s.marks.GetMarksByUserId(v.ctx, fxUserAlice, models.Pagination{})
			s.Require().NoError(err)
			s.Equal(v.wantIt, contains(markIDs(page.Items), fxMarkNear))
		})
		s.Run("GetFollowedMarks/"+v.name, func() {
			page, err := s.marks.GetFollowedMarks(v.ctx, fxUserAlice, models.Pagination{})
			s.Require().NoError(err)
			s.Equal(v.wantIt, contains(markIDs(page.Items), fxMarkNear))
		})
	}

	s.Run("IterateMarks skips it for anonymous", func() {
		var got []int
		s.Require().NoError(s.marks.IterateMarks(anonymous, models.GetMarksFilters{}, func(m models.Mark) error {
			got = append(got, m.ID)
			return nil
		}))
		s.ElementsMatch([]int{fxMarkInside, fxMarkFar}, got)
	})

	s.Run("heatmap and boundary counts never include it", func() {
		bbox := models.BBox{MinLon: 41.39, MinLat: 52.69, MaxLon: 41.42, MaxLat: 52.71}
		cells, err := s.maps.GetHeatmap(moderator, models.HeatmapFilters{BBox: bbox, CellM: 250})
		s.Require().NoError(err)
		total := 0
		for _, c := range cells {
			total += c.Count
		}
		s.Equal(1, total, "only mark 2 remains inside the bbox")

		counts, err := s.maps.GetAdminBoundariesMarksCount(moderator, models.GetAdminBoundaryMarksCountFilters{})
		s.Require().NoError(err)
		for _, c := range counts {
			if c.Id == fxBoundaryMain {
				s.Equal(1, c.TotalCount)
			}
		}
	})

	s.Run("analytics never include it", func() {
		kpi, err := s.analytics.GetKPI(moderator, models.AnalyticsFilters{})
		s.Require().NoError(err)
		s.Equal(2, kpi.Total)
	})

	s.Run("the tasker never assigns it", func() {
		distances, err := s.marks.GetDistancesFromMarkToPoint(s.ctx, models.GetDistanceFromMarkToPointFilters{
			MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus, models.ConfirmedStatus, models.UnderReviewStatus},
			MaxRadius:     1_000_000,
		})
		s.Require().NoError(err)
		for _, d := range distances {
			s.NotEqual(fxMarkNear, d.MarkId)
		}
	})

	s.Run("showing it again", func() {
		s.Require().NoError(s.marks.SetMarkHidden(s.ctx, fxMarkNear, false))
		page, err := s.marks.GetMarks(anonymous, models.GetMarksFilters{})
		s.Require().NoError(err)
		s.Equal(3, page.Total)
	})

	s.Run("missing mark", func() {
		s.ErrorIs(s.marks.SetMarkHidden(s.ctx, 999, true), repository.ErrNotFound)
	})
}

func (s *PostgresSuite) TestMarks_GetMarkBriefs() {
	s.Require().NoError(s.marks.SetMarkHidden(s.ctx, fxMarkNear, true))

	briefs, err := s.marks.GetMarkBriefs(s.ctx, []int{fxMarkNear, fxMarkFar, 999})
	s.Require().NoError(err)
	s.Len(briefs, 2)
	s.Equal("Свалка у дома", briefs[fxMarkNear].Description)
	s.True(briefs[fxMarkNear].Hidden, "briefs are unfiltered: the queue shows hidden marks")
	s.Equal(fxUserAlice, briefs[fxMarkNear].UserID)
	s.Equal(models.UnderReviewStatus, briefs[fxMarkFar].MarkStatusID)
	s.Require().NotNil(briefs[fxMarkFar].Geom)
	s.InDelta(coordMarkFar.X(), briefs[fxMarkFar].Geom.Ewkb.X(), 1e-6)

	empty, err := s.marks.GetMarkBriefs(s.ctx, nil)
	s.Require().NoError(err)
	s.Empty(empty)
}

// TestMarks_Merge covers the pieces of a merge: followers, issued tasks and
// the status/merged_into_id of the source.
func (s *PostgresSuite) TestMarks_Merge() {
	// Bob follows mark 3 (his own) and mark 1; Alice follows marks 1 and 2.
	_, err := s.db.ExecContext(s.ctx, `INSERT INTO mark_followers (user_id, mark_id) VALUES (2, 1)`)
	s.Require().NoError(err)

	s.Run("MoveFollowers keeps one subscription per user", func() {
		s.Require().NoError(s.marks.MoveFollowers(s.ctx, fxMarkNear, fxMarkInside))

		got, err := s.marks.GetFollowerIDs(s.ctx, fxMarkInside)
		s.Require().NoError(err)
		s.Equal([]int{fxUserAlice, fxUserBob}, got)
		gone, err := s.marks.GetFollowerIDs(s.ctx, fxMarkNear)
		s.Require().NoError(err)
		s.Empty(gone)
	})

	s.Run("MoveOpenTasks moves what can be moved and drops the rest", func() {
		// Fixtures: Alice has an issued task on mark 1 and a completed one on
		// mark 2; Bob has an issued task on mark 3. Alice is the author of
		// mark 2, so her task cannot move there; Bob's task moves from 3 to
		// 1, but not when he already holds an issued task on 1.
		s.Require().NoError(s.tasks.MoveOpenTasks(s.ctx, fxMarkNear, fxMarkInside))
		s.Equal(0, s.countRows("tasks", "mark_id = $1", fxMarkNear))
		s.Equal(1, s.countRows("tasks", "mark_id = $1 AND user_id = $2", fxMarkInside, fxUserAlice), "only the completed task remains")

		s.Require().NoError(s.tasks.MoveOpenTasks(s.ctx, fxMarkFar, fxMarkNear))
		s.Equal(1, s.countRows("tasks", "mark_id = $1 AND user_id = $2 AND status_id = $3", fxMarkNear, fxUserBob, models.UnfulfilledStatus))
		s.Equal(0, s.countRows("tasks", "mark_id = $1", fxMarkFar))

		_, err := s.db.ExecContext(s.ctx, `INSERT INTO tasks (name, user_id, mark_id, status_id) VALUES ('again', 2, 3, 1)`)
		s.Require().NoError(err)
		s.Require().NoError(s.tasks.MoveOpenTasks(s.ctx, fxMarkFar, fxMarkNear))
		s.Equal(1, s.countRows("tasks", "mark_id = $1 AND user_id = $2 AND status_id = $3", fxMarkNear, fxUserBob, models.UnfulfilledStatus), "the duplicate is dropped, not doubled")
		s.Equal(0, s.countRows("tasks", "mark_id = $1", fxMarkFar))
	})

	s.Run("MergeMark sets the status and the target", func() {
		s.Require().NoError(s.marks.MergeMark(s.ctx, fxMarkNear, fxMarkInside))

		got, err := s.marks.GetMarkById(s.ctx, fxMarkNear)
		s.Require().NoError(err)
		s.Equal(models.DuplicateStatus, got.MarkStatusID)
		s.Equal(int64(fxMarkInside), got.MergedIntoID.ValueOrZero())

		last, err := s.marks.GetLastMarkStatusHistoryItem(s.ctx, fxMarkNear)
		s.Require().NoError(err)
		s.Equal(models.DuplicateStatus, last.NewMarkStatusID, "the trigger records the merge in the history")

		similar, err := s.marks.GetSimilarMarks(s.ctx, models.GetSimilarMarksFilters{Lon: coordMarkNear.X(), Lat: coordMarkNear.Y(), MarkTypeID: 1, RadiusM: 10})
		s.Require().NoError(err)
		s.Empty(similar, "a duplicate is not an active mark any more")
	})

	s.Run("MergeMark into a missing mark is ErrInvalidReference", func() {
		s.ErrorIs(s.marks.MergeMark(s.ctx, fxMarkFar, 999), repository.ErrInvalidReference)
	})

	s.Run("MergeMark of a missing mark is ErrNotFound", func() {
		s.ErrorIs(s.marks.MergeMark(s.ctx, 999, fxMarkInside), repository.ErrNotFound)
	})

	s.Run("the duplicate status is a dictionary entry", func() {
		statuses, err := s.marks.GetMarkStatuses(s.ctx, models.Lang("en"))
		s.Require().NoError(err)
		var found bool
		for _, st := range statuses {
			if st.ID == int(models.DuplicateStatus) {
				found = true
				s.Equal("duplicate", st.Code)
				s.Equal("Duplicate", st.Name)
			}
		}
		s.True(found)
	})
}

func (s *PostgresSuite) TestUsers_GetUserIDsByRole() {
	got, err := s.users.GetUserIDsByRole(s.ctx, models.RoleModerator, models.RoleAdmin)
	s.Require().NoError(err)
	s.Equal([]int{fxUserBob}, got)

	got, err = s.users.GetUserIDsByRole(s.ctx, models.RoleAdmin)
	s.Require().NoError(err)
	s.Empty(got)

	got, err = s.users.GetUserIDsByRole(s.ctx)
	s.Require().NoError(err)
	s.Empty(got)
}

func contains(items []int, id int) bool {
	for _, it := range items {
		if it == id {
			return true
		}
	}
	return false
}
