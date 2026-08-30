//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

func (s *PostgresSuite) newReport(reporter int, targetType models.ReportTargetType, targetId int) models.Report {
	return models.Report{ReporterID: reporter, TargetType: targetType, TargetID: targetId, Reason: models.ReportReasonSpam, Comment: "spam"}
}

func (s *PostgresSuite) TestReports_AddReport() {
	s.Run("stores the report with defaults", func() {
		got, err := s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetMark, fxMarkNear))
		s.Require().NoError(err)
		s.NotZero(got.ID)
		s.Equal(fxUserBob, got.ReporterID)
		s.Equal(models.ReportTargetMark, got.TargetType)
		s.Equal(fxMarkNear, got.TargetID)
		s.Equal(models.ReportReasonSpam, got.Reason)
		s.Equal("spam", got.Comment)
		s.Equal(models.ReportStatusOpen, got.Status)
		s.False(got.ResolvedBy.Valid)
		s.False(got.ResolvedAt.Valid)
		s.WithinDuration(time.Now(), got.CreatedAt, time.Minute)
	})

	s.Run("repeated report on the same target is ErrExists", func() {
		_, err := s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetMark, fxMarkNear))
		s.ErrorIs(err, repository.ErrExists)
	})

	s.Run("same reporter, another target type with the same id is fine", func() {
		_, err := s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetCheck, fxMarkNear))
		s.NoError(err)
	})

	s.Run("a comment needs no existing row", func() {
		_, err := s.reports.AddReport(s.ctx, s.newReport(fxUserAlice, models.ReportTargetComment, 424242))
		s.NoError(err)
	})

	s.Run("unknown reporter is ErrInvalidReference", func() {
		_, err := s.reports.AddReport(s.ctx, s.newReport(999, models.ReportTargetMark, fxMarkNear))
		s.ErrorIs(err, repository.ErrInvalidReference)
	})

	s.Run("unknown reason is rejected by the check constraint", func() {
		r := s.newReport(fxUserAlice, models.ReportTargetMark, fxMarkFar)
		r.Reason = "boring"
		_, err := s.reports.AddReport(s.ctx, r)
		s.Error(err)
	})
}

func (s *PostgresSuite) TestReports_GetReportsAndResolve() {
	r1, err := s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetMark, fxMarkNear))
	s.Require().NoError(err)
	r2, err := s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetCheck, 1))
	s.Require().NoError(err)
	r3, err := s.reports.AddReport(s.ctx, s.newReport(fxUserAlice, models.ReportTargetMark, fxMarkFar))
	s.Require().NoError(err)

	s.Require().NoError(s.reports.ResolveReport(s.ctx, r2.ID, models.ReportStatusDismissed, fxUserBob))

	reportIDs := func(items []models.Report) []int {
		return ids(items, func(r models.Report) int { return r.ID })
	}

	tests := []struct {
		name    string
		filters models.GetReportsFilters
		wantIDs []int
	}{
		{name: "all, oldest first", wantIDs: []int{r1.ID, r2.ID, r3.ID}},
		{name: "open only", filters: models.GetReportsFilters{Status: models.ReportStatusOpen}, wantIDs: []int{r1.ID, r3.ID}},
		{name: "dismissed only", filters: models.GetReportsFilters{Status: models.ReportStatusDismissed}, wantIDs: []int{r2.ID}},
		{name: "by target type", filters: models.GetReportsFilters{TargetType: models.ReportTargetMark}, wantIDs: []int{r1.ID, r3.ID}},
		{name: "by reporter", filters: models.GetReportsFilters{ReporterID: fxUserAlice}, wantIDs: []int{r3.ID}},
		{name: "paginated", filters: models.GetReportsFilters{Pagination: models.Pagination{Limit: 1, Offset: 1}}, wantIDs: []int{r2.ID}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.reports.GetReports(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.Equal(tt.wantIDs, reportIDs(page.Items))
			if tt.filters.Pagination.Limit == 0 {
				s.Equal(len(tt.wantIDs), page.Total)
			} else {
				s.Equal(3, page.Total)
			}
		})
	}

	s.Run("resolved report carries who and when", func() {
		got, err := s.reports.GetReportById(s.ctx, r2.ID)
		s.Require().NoError(err)
		s.Equal(models.ReportStatusDismissed, got.Status)
		s.Equal(int64(fxUserBob), got.ResolvedBy.ValueOrZero())
		s.True(got.ResolvedAt.Valid)
	})

	s.Run("resolving a decided report is ErrNotFound", func() {
		s.ErrorIs(s.reports.ResolveReport(s.ctx, r2.ID, models.ReportStatusResolved, fxUserBob), repository.ErrNotFound)
	})

	s.Run("resolving a missing report is ErrNotFound", func() {
		s.ErrorIs(s.reports.ResolveReport(s.ctx, 999, models.ReportStatusResolved, fxUserBob), repository.ErrNotFound)
	})

	s.Run("missing report", func() {
		_, err := s.reports.GetReportById(s.ctx, 999)
		s.ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *PostgresSuite) TestReports_Counts() {
	_, err := s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetMark, fxMarkNear))
	s.Require().NoError(err)
	_, err = s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetMark, fxMarkInside))
	s.Require().NoError(err)
	// A third user so that mark 1 has two open reports.
	_, err = s.db.ExecContext(s.ctx, `INSERT INTO users (name, login, password_hash, role) VALUES ('Carol', 'carol', 'hash', 'user')`)
	s.Require().NoError(err)
	decided, err := s.reports.AddReport(s.ctx, s.newReport(3, models.ReportTargetMark, fxMarkNear))
	s.Require().NoError(err)

	s.Run("open reports per target", func() {
		n, err := s.reports.CountOpenReports(s.ctx, models.ReportTargetMark, fxMarkNear)
		s.Require().NoError(err)
		s.Equal(2, n)

		s.Require().NoError(s.reports.ResolveReport(s.ctx, decided.ID, models.ReportStatusResolved, fxUserBob))
		n, err = s.reports.CountOpenReports(s.ctx, models.ReportTargetMark, fxMarkNear)
		s.Require().NoError(err)
		s.Equal(1, n, "a decided report no longer counts")

		n, err = s.reports.CountOpenReports(s.ctx, models.ReportTargetCheck, fxMarkNear)
		s.Require().NoError(err)
		s.Equal(0, n, "the target type is part of the key")
	})

	s.Run("reports per reporter since", func() {
		n, err := s.reports.CountReportsByReporterSince(s.ctx, fxUserBob, time.Now().Add(-time.Hour))
		s.Require().NoError(err)
		s.Equal(2, n)

		n, err = s.reports.CountReportsByReporterSince(s.ctx, fxUserBob, time.Now().Add(time.Hour))
		s.Require().NoError(err)
		s.Equal(0, n)
	})
}

func (s *PostgresSuite) TestReports_MoveMarkReports() {
	// Bob reported both marks: his report on the source is dropped, Alice's
	// moves; the check report on the same id is untouched.
	_, err := s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetMark, fxMarkNear))
	s.Require().NoError(err)
	_, err = s.reports.AddReport(s.ctx, s.newReport(fxUserBob, models.ReportTargetMark, fxMarkInside))
	s.Require().NoError(err)
	moved, err := s.reports.AddReport(s.ctx, s.newReport(fxUserAlice, models.ReportTargetMark, fxMarkNear))
	s.Require().NoError(err)
	_, err = s.reports.AddReport(s.ctx, s.newReport(fxUserAlice, models.ReportTargetCheck, fxMarkNear))
	s.Require().NoError(err)

	s.Require().NoError(s.reports.MoveMarkReports(s.ctx, fxMarkNear, fxMarkInside))

	s.Equal(0, s.countRows("reports", "target_type = 'mark' AND target_id = $1", fxMarkNear))
	s.Equal(2, s.countRows("reports", "target_type = 'mark' AND target_id = $1", fxMarkInside))
	s.Equal(1, s.countRows("reports", "target_type = 'check' AND target_id = $1", fxMarkNear))

	got, err := s.reports.GetReportById(s.ctx, moved.ID)
	s.Require().NoError(err)
	s.Equal(fxMarkInside, got.TargetID)
}
