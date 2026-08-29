//go:build integration

package postgres_test

import (
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/twpayne/go-geom"
)

func markIDs(marks []models.Mark) []int {
	return ids(marks, func(m models.Mark) int { return m.ID })
}

func (s *PostgresSuite) TestMarks_GetMarks() {
	tests := []struct {
		name    string
		filters models.GetMarksFilters
		wantIDs []int
	}{
		{name: "no filters returns all", wantIDs: []int{fxMarkNear, fxMarkInside, fxMarkFar}},
		{name: "by status", filters: models.GetMarksFilters{MarkStatusIds: []int{int(models.ConfirmedStatus)}}, wantIDs: []int{fxMarkInside}},
		{name: "by several statuses", filters: models.GetMarksFilters{MarkStatusIds: []int{int(models.UnconfirmedStatus), int(models.UnderReviewStatus)}}, wantIDs: []int{fxMarkNear, fxMarkFar}},
		{name: "by type", filters: models.GetMarksFilters{MarkTypeIds: []int{1}}, wantIDs: []int{fxMarkNear, fxMarkFar}},
		{name: "by type and status", filters: models.GetMarksFilters{MarkTypeIds: []int{1}, MarkStatusIds: []int{int(models.UnderReviewStatus)}}, wantIDs: []int{fxMarkFar}},
		{name: "no match", filters: models.GetMarksFilters{MarkStatusIds: []int{int(models.ClosedStatus)}}, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.marks.GetMarks(s.ctx, tt.filters)
			s.Require().NoError(err)
			marks := page.Items
			s.NotNil(marks)
			s.Equal(len(tt.wantIDs), page.Total)
			s.ElementsMatch(tt.wantIDs, markIDs(marks))
			for _, m := range marks {
				s.Require().NotNil(m.Geom)
				s.Equal(4326, m.Geom.Ewkb.SRID())
				s.False(m.CreatedAt.IsZero())
				s.False(m.UpdatedAt.IsZero())
			}
		})
	}
}

func (s *PostgresSuite) TestMarks_GetMarkById() {
	tests := []struct {
		name    string
		id      int
		want    models.Mark
		wantErr error
	}{
		{
			name: "existing mark",
			id:   fxMarkInside,
			want: models.Mark{
				ID: fxMarkInside, Description: "Разбитая лавка", Geom: models.NewPoint(coordMarkIn),
				MarkTypeID: 2, MarkStatusID: models.ConfirmedStatus, UserID: fxUserAlice,
			},
		},
		{name: "missing mark", id: 404, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.marks.GetMarkById(s.ctx, tt.id)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.want.ID, got.ID)
			s.Equal(tt.want.Description, got.Description)
			s.Equal(tt.want.MarkTypeID, got.MarkTypeID)
			s.Equal(tt.want.MarkStatusID, got.MarkStatusID)
			s.Equal(tt.want.UserID, got.UserID)
			s.Require().NotNil(got.Geom)
			s.InDelta(tt.want.Geom.Ewkb.X(), got.Geom.Ewkb.X(), 1e-6)
			s.InDelta(tt.want.Geom.Ewkb.Y(), got.Geom.Ewkb.Y(), 1e-6)
		})
	}
}

func (s *PostgresSuite) TestMarks_GetMarksByUserId() {
	tests := []struct {
		name    string
		userID  int
		wantIDs []int
	}{
		{name: "alice owns two marks", userID: fxUserAlice, wantIDs: []int{fxMarkNear, fxMarkInside}},
		{name: "bob owns one mark", userID: fxUserBob, wantIDs: []int{fxMarkFar}},
		{name: "unknown user has no marks", userID: 999, wantIDs: []int{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.marks.GetMarksByUserId(s.ctx, tt.userID, models.Pagination{})
			s.Require().NoError(err)
			marks := page.Items
			s.NotNil(marks)
			s.Equal(len(tt.wantIDs), page.Total)
			s.ElementsMatch(tt.wantIDs, markIDs(marks))
		})
	}
}

func (s *PostgresSuite) TestMarks_AddMark() {
	tests := []struct {
		name    string
		mark    models.Mark
		wantErr bool
	}{
		{
			name: "mark gets default status and a history row",
			mark: models.Mark{Description: "Новая", Geom: models.NewPoint(geom.Coord{41.41, 52.70}), MarkTypeID: 3, UserID: fxUserBob},
		},
		{
			name:    "unknown type violates foreign key",
			mark:    models.Mark{Description: "Плохая", Geom: models.NewPoint(geom.Coord{41.41, 52.70}), MarkTypeID: 999, UserID: fxUserBob},
			wantErr: true,
		},
		{
			name:    "unknown user violates foreign key",
			mark:    models.Mark{Description: "Плохая", Geom: models.NewPoint(geom.Coord{41.41, 52.70}), MarkTypeID: 1, UserID: 999},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			id, err := s.marks.AddMark(s.ctx, tt.mark)
			if tt.wantErr {
				s.Error(err)
				s.NotErrorIs(err, repository.ErrExists)
				return
			}
			s.Require().NoError(err)
			s.Greater(id, int64(fxMarkFar))

			got, err := s.marks.GetMarkById(s.ctx, int(id))
			s.Require().NoError(err)
			s.Equal(models.UnconfirmedStatus, got.MarkStatusID, "default status must be 'unconfirmed'")
			s.Equal(tt.mark.Description, got.Description)
			s.Equal(tt.mark.MarkTypeID, got.MarkTypeID)
			s.Equal(tt.mark.UserID, got.UserID)

			// The insert trigger must log the initial status.
			history, err := s.marks.GetMarkStatusHistoryByMarkId(s.ctx, int(id))
			s.Require().NoError(err)
			s.Require().Len(history, 1)
			s.False(history[0].OldMarkStatusID.Valid)
			s.Equal(models.UnconfirmedStatus, history[0].NewMarkStatusID)
			s.False(history[0].PrevId.Valid)
		})
	}
}

func (s *PostgresSuite) TestMarks_GetMarkTypes() {
	types, err := s.marks.GetMarkTypes(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(types, 4)

	names := make([]string, 0, len(types))
	for i, t := range types {
		names = append(names, t.Name)
		s.NotZero(t.ID)
		if i > 0 {
			s.LessOrEqual(types[i-1].Name, t.Name, "types must be ordered by name")
		}
	}
	s.ElementsMatch([]string{"Мусор", "Зелёные зоны и парки", "Освещение", "Информационные и визуальные дефекты"}, names)
}

func (s *PostgresSuite) TestMarks_GetMarkStatuses() {
	statuses, err := s.marks.GetMarkStatuses(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(statuses, 6)

	byID := make(map[int]models.MarkStatus, len(statuses))
	for i, st := range statuses {
		byID[st.ID] = st
		if i > 0 {
			s.Less(statuses[i-1].ID, st.ID, "statuses must be ordered by id")
		}
	}
	s.Equal("Неподтверждённая", byID[int(models.UnconfirmedStatus)].Name)
	s.Equal("Подтверждённая", byID[int(models.ConfirmedStatus)].Name)
	s.Equal("На проверке", byID[int(models.UnderReviewStatus)].Name)
	s.Equal("Переоткрытая", byID[int(models.RediscoveredStatus)].Name)
	s.Equal("Закрытая", byID[int(models.ClosedStatus)].Name)
	s.Equal("Опровергнутая", byID[int(models.RefutedStatus)].Name)

	s.True(byID[int(models.ConfirmedStatus)].ParentId.Valid)
	s.Equal(int64(models.UnconfirmedStatus), byID[int(models.ConfirmedStatus)].ParentId.Int64)
	s.False(byID[int(models.UnconfirmedStatus)].ParentId.Valid)
}

func (s *PostgresSuite) TestMarks_UpdateMarkStatus_WritesHistoryChain() {
	// mark1: NULL->1 (history id 1). Move it 1 -> 3 -> 5.
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkNear, models.UnderReviewStatus))
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkNear, models.ClosedStatus))

	mark, err := s.marks.GetMarkById(s.ctx, fxMarkNear)
	s.Require().NoError(err)
	s.Equal(models.ClosedStatus, mark.MarkStatusID)

	history, err := s.marks.GetMarkStatusHistoryByMarkId(s.ctx, fxMarkNear)
	s.Require().NoError(err)
	s.Require().Len(history, 3)

	s.False(history[0].OldMarkStatusID.Valid)
	s.Equal(models.UnconfirmedStatus, history[0].NewMarkStatusID)
	s.False(history[0].PrevId.Valid)

	s.Equal(models.UnconfirmedStatus, history[1].OldMarkStatusID.V)
	s.Equal(models.UnderReviewStatus, history[1].NewMarkStatusID)
	s.Equal(int64(history[0].ID), history[1].PrevId.Int64, "prev_id must point at the row that set the old status")

	s.Equal(models.UnderReviewStatus, history[2].OldMarkStatusID.V)
	s.Equal(models.ClosedStatus, history[2].NewMarkStatusID)
	s.Equal(int64(history[1].ID), history[2].PrevId.Int64)

	for _, h := range history {
		s.Equal(fxMarkNear, h.MarkID)
		s.False(h.ChangedAt.IsZero())
	}
}

func (s *PostgresSuite) TestMarks_UpdateMarkStatus_SameStatusIsNotLogged() {
	before := s.countRows("mark_status_history", "mark_id = $1", fxMarkNear)
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkNear, models.UnconfirmedStatus))
	s.Equal(before, s.countRows("mark_status_history", "mark_id = $1", fxMarkNear))
}

func (s *PostgresSuite) TestMarks_UpdateMarkStatus_UnknownMark() {
	// UPDATE of a missing row is a no-op for the repository.
	s.NoError(s.marks.UpdateMarkStatus(s.ctx, 404, models.ClosedStatus))
	s.Equal(0, s.countRows("mark_status_history", "mark_id = $1", 404))
}

func (s *PostgresSuite) TestMarks_GetMarkStatusHistoryByMarkId() {
	tests := []struct {
		name     string
		markID   int
		wantNew  []models.MarkStatusType
		wantPrev []bool
	}{
		{name: "mark with one transition", markID: fxMarkInside, wantNew: []models.MarkStatusType{models.UnconfirmedStatus, models.ConfirmedStatus}, wantPrev: []bool{false, true}},
		{name: "mark with initial row only", markID: fxMarkNear, wantNew: []models.MarkStatusType{models.UnconfirmedStatus}, wantPrev: []bool{false}},
		{name: "unknown mark", markID: 404, wantNew: []models.MarkStatusType{}, wantPrev: []bool{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			history, err := s.marks.GetMarkStatusHistoryByMarkId(s.ctx, tt.markID)
			s.Require().NoError(err)
			s.Require().Len(history, len(tt.wantNew))
			for i, h := range history {
				s.Equal(tt.wantNew[i], h.NewMarkStatusID)
				s.Equal(tt.wantPrev[i], h.PrevId.Valid)
			}
		})
	}
}

func (s *PostgresSuite) TestMarks_GetLastMarkStatusHistoryItemWithStatus() {
	// Re-open mark 2 twice so there are two rows with the same new status.
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkInside, models.UnderReviewStatus))
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkInside, models.ConfirmedStatus))

	tests := []struct {
		name    string
		markID  int
		status  models.MarkStatusType
		wantOld models.MarkStatusType
		wantErr error
	}{
		{name: "latest row with the status", markID: fxMarkInside, status: models.ConfirmedStatus, wantOld: models.UnderReviewStatus},
		{name: "status never set on the mark", markID: fxMarkInside, status: models.ClosedStatus, wantErr: repository.ErrNotFound},
		{name: "unknown mark", markID: 404, status: models.UnconfirmedStatus, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			item, err := s.marks.GetLastMarkStatusHistoryItemWithStatus(s.ctx, tt.markID, tt.status)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.markID, item.MarkID)
			s.Equal(tt.status, item.NewMarkStatusID)
			s.Equal(tt.wantOld, item.OldMarkStatusID.V)
		})
	}
}

func (s *PostgresSuite) TestMarks_GetLastMarkStatusHistoryItem() {
	tests := []struct {
		name    string
		markID  int
		wantNew models.MarkStatusType
		wantErr error
	}{
		{name: "mark with transitions", markID: fxMarkFar, wantNew: models.UnderReviewStatus},
		{name: "mark with initial row only", markID: fxMarkNear, wantNew: models.UnconfirmedStatus},
		{name: "unknown mark", markID: 404, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			item, err := s.marks.GetLastMarkStatusHistoryItem(s.ctx, tt.markID)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.markID, item.MarkID)
			s.Equal(tt.wantNew, item.NewMarkStatusID)
		})
	}
}

func (s *PostgresSuite) TestMarks_GetDistancesFromMarkToPoint() {
	type pair struct{ mark, user int }

	tests := []struct {
		name      string
		filters   models.GetDistanceFromMarkToPointFilters
		wantPairs []pair
	}{
		{
			name:      "radius 1 km catches only alice near mark 1",
			filters:   models.GetDistanceFromMarkToPointFilters{MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus}, MaxRadius: 1000},
			wantPairs: []pair{{fxMarkNear, fxUserAlice}},
		},
		{
			name:      "radius 2 km catches mark 2 for alice too",
			filters:   models.GetDistanceFromMarkToPointFilters{MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus, models.ConfirmedStatus}, MaxRadius: 2000},
			wantPairs: []pair{{fxMarkNear, fxUserAlice}, {fxMarkInside, fxUserAlice}},
		},
		{
			name:      "status filter excludes mark 2",
			filters:   models.GetDistanceFromMarkToPointFilters{MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus}, MaxRadius: 2000},
			wantPairs: []pair{{fxMarkNear, fxUserAlice}},
		},
		{
			name:    "huge radius pairs everyone with everything",
			filters: models.GetDistanceFromMarkToPointFilters{MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus, models.ConfirmedStatus, models.UnderReviewStatus}, MaxRadius: 1_000_000},
			wantPairs: []pair{
				{fxMarkNear, fxUserAlice}, {fxMarkNear, fxUserBob},
				{fxMarkInside, fxUserAlice}, {fxMarkInside, fxUserBob},
				{fxMarkFar, fxUserAlice}, {fxMarkFar, fxUserBob},
			},
		},
		{
			name:      "zero radius matches nothing",
			filters:   models.GetDistanceFromMarkToPointFilters{MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus}, MaxRadius: 0},
			wantPairs: []pair{},
		},
		{
			name:      "empty status list matches nothing",
			filters:   models.GetDistanceFromMarkToPointFilters{MarkStatusIds: []models.MarkStatusType{}, MaxRadius: 1_000_000},
			wantPairs: []pair{},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			distances, err := s.marks.GetDistancesFromMarkToPoint(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.NotNil(distances)

			got := make([]pair, 0, len(distances))
			for _, d := range distances {
				got = append(got, pair{d.MarkId, d.UserId})
				s.GreaterOrEqual(d.Distance, 0.0)
				s.LessOrEqual(d.Distance, float64(tt.filters.MaxRadius)/1000.0+0.01, "distance_km must respect the radius")
			}
			s.Equal(tt.wantPairs, got, "rows must be ordered by mark_id, user_id")
		})
	}
}

func (s *PostgresSuite) TestMarks_GetDistancesFromMarkToPoint_DistanceValue() {
	distances, err := s.marks.GetDistancesFromMarkToPoint(s.ctx, models.GetDistanceFromMarkToPointFilters{
		MarkStatusIds: []models.MarkStatusType{models.UnconfirmedStatus},
		MaxRadius:     1_000_000,
	})
	s.Require().NoError(err)
	s.Require().Len(distances, 2)

	// Alice -> mark 1 is about 150 m; Bob (Moscow) -> mark 1 is about 420 km.
	s.Equal(fxUserAlice, distances[0].UserId)
	s.InDelta(0.15, distances[0].Distance, 0.05)
	s.Equal(fxUserBob, distances[1].UserId)
	s.InDelta(420, distances[1].Distance, 40)
}
