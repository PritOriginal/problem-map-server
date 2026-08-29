//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/guregu/null/v6"
)

func nullFloat(v float64) null.Float {
	return null.FloatFrom(v)
}

// Fixture history (see seed): mark1 unconfirmed for 40 days; mark2 created
// 10 days ago and confirmed 48 h later; mark3 created 5 days ago and put
// under review 24 h later.

func (s *PostgresSuite) TestAnalytics_GetKPI() {
	tests := []struct {
		name    string
		filters models.AnalyticsFilters
		want    models.KPI
	}{
		{
			name: "no filters",
			want: models.KPI{
				Total:              3,
				ByStatus:           map[int]int{1: 1, 2: 1, 3: 1},
				AvgConfirmHours:    nullFloat(48),
				MedianConfirmHours: nullFloat(48),
				OpenOlderThan30d:   1,
			},
		},
		{
			name:    "boundary with marks",
			filters: models.AnalyticsFilters{BoundaryID: fxBoundaryMain},
			want: models.KPI{
				Total:              2,
				ByStatus:           map[int]int{1: 1, 2: 1},
				AvgConfirmHours:    nullFloat(48),
				MedianConfirmHours: nullFloat(48),
				OpenOlderThan30d:   1,
			},
		},
		{
			name:    "empty boundary",
			filters: models.AnalyticsFilters{BoundaryID: fxBoundaryVoid},
			want:    models.KPI{ByStatus: map[int]int{}},
		},
		{
			name:    "unknown boundary",
			filters: models.AnalyticsFilters{BoundaryID: 404},
			want:    models.KPI{ByStatus: map[int]int{}},
		},
		{
			name:    "by type: no mark confirmed",
			filters: models.AnalyticsFilters{MarkTypeID: 1},
			want: models.KPI{
				Total:            2,
				ByStatus:         map[int]int{1: 1, 3: 1},
				OpenOlderThan30d: 1,
			},
		},
		{
			name:    "date range excludes the stale mark",
			filters: models.AnalyticsFilters{DateRange: models.DateRange{From: s.daysAgo(20)}},
			want: models.KPI{
				Total:              2,
				ByStatus:           map[int]int{2: 1, 3: 1},
				AvgConfirmHours:    nullFloat(48),
				MedianConfirmHours: nullFloat(48),
			},
		},
		{
			name:    "closed range",
			filters: models.AnalyticsFilters{DateRange: models.DateRange{From: s.daysAgo(45), To: s.daysAgo(30)}},
			want: models.KPI{
				Total:            1,
				ByStatus:         map[int]int{1: 1},
				OpenOlderThan30d: 1,
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.analytics.GetKPI(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.Equal([]models.OrganizationKPI{}, got.ByOrganization)
			got.ByOrganization = nil
			s.Equal(tt.want, got)
		})
	}
}

func (s *PostgresSuite) TestAnalytics_GetKPI_ClosedAndRefuted() {
	// Close mark 2 three days after creation and refute mark 3 so the closing
	// duration, the refuted share and the median with two samples are covered.
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkInside, models.ClosedStatus))
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkFar, models.RefutedStatus))
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkNear, models.ConfirmedStatus))
	const backdate = `UPDATE mark_status_history SET changed_at = $1::timestamp - $2::interval
		WHERE mark_id = $3 AND new_mark_status_id = $4`
	_, err := s.db.ExecContext(s.ctx, backdate, s.seedNow, "7 days", fxMarkInside, models.ClosedStatus)
	s.Require().NoError(err)
	_, err = s.db.ExecContext(s.ctx, backdate, s.seedNow, "39 days", fxMarkNear, models.ConfirmedStatus)
	s.Require().NoError(err)

	got, err := s.analytics.GetKPI(s.ctx, models.AnalyticsFilters{})
	s.Require().NoError(err)

	// Confirmation: mark1 24 h, mark2 48 h -> avg 36, median 36.
	got.ByOrganization = nil
	s.Equal(models.KPI{
		Total:              3,
		ByStatus:           map[int]int{2: 1, 5: 1, 6: 1},
		AvgConfirmHours:    nullFloat(36),
		MedianConfirmHours: nullFloat(36),
		AvgCloseHours:      nullFloat(72),
		RefutedShare:       1.0 / 3.0,
		// mark1 is confirmed (still open) and older than 30 days.
		OpenOlderThan30d: 1,
	}, got)
}

func (s *PostgresSuite) TestAnalytics_GetTimeseries() {
	day := func(daysAgo int) time.Time {
		return s.daysAgo(daysAgo).Truncate(24 * time.Hour)
	}

	tests := []struct {
		name        string
		filters     models.TimeseriesFilters
		wantPeriods int
		wantFirst   time.Time
		wantNonZero map[time.Time]models.TimeseriesPoint
	}{
		{
			name: "daily: created and confirmed land in their days, the rest is zero",
			filters: models.TimeseriesFilters{
				Step:             models.StepDay,
				AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: s.daysAgo(12), To: s.seedNow}},
			},
			wantPeriods: 13,
			wantFirst:   day(12),
			wantNonZero: map[time.Time]models.TimeseriesPoint{
				day(10): {Period: day(10), Created: 1},
				day(8):  {Period: day(8), Confirmed: 1},
				day(5):  {Period: day(5), Created: 1},
			},
		},
		{
			name: "daily filtered by type excludes mark 2",
			filters: models.TimeseriesFilters{
				Step:             models.StepDay,
				AnalyticsFilters: models.AnalyticsFilters{MarkTypeID: 1, DateRange: models.DateRange{From: s.daysAgo(12), To: s.seedNow}},
			},
			wantPeriods: 13,
			wantFirst:   day(12),
			wantNonZero: map[time.Time]models.TimeseriesPoint{
				day(5): {Period: day(5), Created: 1},
			},
		},
		{
			name: "daily filtered by boundary excludes mark 3",
			filters: models.TimeseriesFilters{
				Step:             models.StepDay,
				AnalyticsFilters: models.AnalyticsFilters{BoundaryID: fxBoundaryMain, DateRange: models.DateRange{From: s.daysAgo(12), To: s.seedNow}},
			},
			wantPeriods: 13,
			wantFirst:   day(12),
			wantNonZero: map[time.Time]models.TimeseriesPoint{
				day(10): {Period: day(10), Created: 1},
				day(8):  {Period: day(8), Confirmed: 1},
			},
		},
		{
			name: "range without events is all zeros",
			filters: models.TimeseriesFilters{
				Step:             models.StepDay,
				AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: s.daysAgo(3), To: s.seedNow}},
			},
			wantPeriods: 4,
			wantFirst:   day(3),
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			points, err := s.analytics.GetTimeseries(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.Require().Len(points, tt.wantPeriods)
			s.Equal(tt.wantFirst, points[0].Period)

			for i, p := range points {
				if i > 0 {
					s.True(p.Period.After(points[i-1].Period), "periods must be ascending")
				}
				want, ok := tt.wantNonZero[p.Period]
				if !ok {
					want = models.TimeseriesPoint{Period: p.Period}
				}
				s.Equal(want, p, "period %s", p.Period)
			}
		})
	}
}

func (s *PostgresSuite) TestAnalytics_GetTimeseries_WeekAndMonth() {
	sum := func(points []models.TimeseriesPoint) models.TimeseriesPoint {
		var total models.TimeseriesPoint
		for _, p := range points {
			total.Created += p.Created
			total.Confirmed += p.Confirmed
			total.Closed += p.Closed
			total.Refuted += p.Refuted
		}
		return total
	}

	// Close mark 3 now (DB clock, slightly after seedNow) so every counter but
	// "refuted" is non-zero; the range therefore ends an hour past seedNow.
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, fxMarkFar, models.ClosedStatus))

	for _, step := range []models.TimeseriesStep{models.StepWeek, models.StepMonth} {
		s.Run(string(step), func() {
			points, err := s.analytics.GetTimeseries(s.ctx, models.TimeseriesFilters{
				Step:             step,
				AnalyticsFilters: models.AnalyticsFilters{DateRange: models.DateRange{From: s.daysAgo(60), To: s.seedNow.Add(time.Hour)}},
			})
			s.Require().NoError(err)
			s.NotEmpty(points)
			s.Equal(models.TimeseriesPoint{Created: 3, Confirmed: 1, Closed: 1}, sum(points))

			// Consecutive buckets are exactly one step apart (no gaps).
			for i := 1; i < len(points); i++ {
				prev, cur := points[i-1].Period, points[i].Period
				switch step {
				case models.StepWeek:
					s.Equal(prev.AddDate(0, 0, 7), cur)
				case models.StepMonth:
					s.Equal(prev.AddDate(0, 1, 0), cur)
				}
			}
		})
	}
}

func (s *PostgresSuite) TestAnalytics_GetTopTypes() {
	tests := []struct {
		name    string
		filters models.TopTypesFilters
		want    []models.TopType
	}{
		{
			name:    "all types, zero counts kept, ordered by count then id",
			filters: models.TopTypesFilters{Limit: 10},
			want: []models.TopType{
				{MarkTypeID: 1, Name: "Мусор", Count: 2, Share: 2.0 / 3.0},
				{MarkTypeID: 2, Name: "Зелёные зоны и парки", Count: 1, Share: 1.0 / 3.0},
				{MarkTypeID: 3, Name: "Освещение"},
				{MarkTypeID: 4, Name: "Информационные и визуальные дефекты"},
			},
		},
		{
			name:    "limit",
			filters: models.TopTypesFilters{Limit: 1},
			want: []models.TopType{
				{MarkTypeID: 1, Name: "Мусор", Count: 2, Share: 2.0 / 3.0},
			},
		},
		{
			name:    "boundary",
			filters: models.TopTypesFilters{BoundaryID: fxBoundaryMain, Limit: 2},
			want: []models.TopType{
				{MarkTypeID: 1, Name: "Мусор", Count: 1, Share: 0.5},
				{MarkTypeID: 2, Name: "Зелёные зоны и парки", Count: 1, Share: 0.5},
			},
		},
		{
			name:    "date range",
			filters: models.TopTypesFilters{DateRange: models.DateRange{From: s.daysAgo(7)}, Limit: 2},
			want: []models.TopType{
				{MarkTypeID: 1, Name: "Мусор", Count: 1, Share: 1},
				{MarkTypeID: 2, Name: "Зелёные зоны и парки"},
			},
		},
		{
			name:    "no marks: zero shares, not division by zero",
			filters: models.TopTypesFilters{BoundaryID: fxBoundaryVoid, Limit: 2},
			want: []models.TopType{
				{MarkTypeID: 1, Name: "Мусор"},
				{MarkTypeID: 2, Name: "Зелёные зоны и парки"},
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.analytics.GetTopTypes(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.Require().Len(got, len(tt.want))
			for i := range tt.want {
				s.Equal(tt.want[i].MarkTypeID, got[i].MarkTypeID)
				s.Equal(tt.want[i].Name, got[i].Name)
				s.Equal(tt.want[i].Count, got[i].Count)
				s.InDelta(tt.want[i].Share, got[i].Share, 1e-9)
			}
		})
	}
}
