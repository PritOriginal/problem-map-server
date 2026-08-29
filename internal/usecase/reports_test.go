package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// reportsCfg keeps the thresholds small so the cases read naturally.
var reportsCfg = config.ReportsConfig{HideThreshold: 2, MaxPerDay: 3}

type ReportsSuite struct {
	suite.Suite
	uc        *usecase.Reports
	trManager *usecase.MockManager
	reports   *usecase.MockReportsRepository
	marks     *usecase.MockReportsMarksRepository
	checks    *usecase.MockReportsChecksRepository
	publisher *events.MockPublisher
}

func (suite *ReportsSuite) SetupTest() {
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.reports = usecase.NewMockReportsRepository(suite.T())
	suite.marks = usecase.NewMockReportsMarksRepository(suite.T())
	suite.checks = usecase.NewMockReportsChecksRepository(suite.T())
	suite.publisher = events.NewMockPublisher(suite.T())
	suite.uc = usecase.NewReports(slogdiscard.NewDiscardLogger(), reportsCfg, suite.trManager, usecase.ReportsRepositories{
		Reports: suite.reports,
		Marks:   suite.marks,
		Checks:  suite.checks,
	}).WithEvents(suite.publisher)
}

func TestReports(t *testing.T) {
	suite.Run(t, new(ReportsSuite))
}

func markReport() models.Report {
	return models.Report{ReporterID: 7, TargetType: models.ReportTargetMark, TargetID: 5, Reason: models.ReportReasonSpam}
}

func (suite *ReportsSuite) expectTx(commitErr error) {
	suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(commitErr))
}

func (suite *ReportsSuite) TestCreate() {
	created := markReport()
	created.ID = 11
	created.Status = models.ReportStatusOpen

	tests := []struct {
		name    string
		report  models.Report
		setup   func()
		wantErr error
	}{
		{
			name:    "ErrInvalidTargetType",
			report:  models.Report{ReporterID: 7, TargetType: "video", TargetID: 5, Reason: models.ReportReasonSpam},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrInvalidTargetID",
			report:  models.Report{ReporterID: 7, TargetType: models.ReportTargetComment, TargetID: 0, Reason: models.ReportReasonSpam},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrInvalidReason",
			report:  models.Report{ReporterID: 7, TargetType: models.ReportTargetMark, TargetID: 5, Reason: "boring"},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:   "ErrTooManyRequests",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(reportsCfg.MaxPerDay, nil)
			},
			wantErr: usecase.ErrTooManyRequests,
		},
		{
			name:   "ErrMarkNotFound",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(repository.ErrNotFound)
			},
			wantErr: usecase.ErrNotFound,
		},
		{
			name:   "ErrOwnMark",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 7}, nil)
			},
			wantErr: usecase.ErrForbidden,
		},
		{
			name:   "ErrRepeatedReport",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(1, nil)
				suite.expectTx(nil)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3}, nil)
				suite.reports.On("AddReport", mock.Anything, markReport()).Once().Return(models.Report{}, repository.ErrExists)
			},
			wantErr: usecase.ErrConflict,
		},
		{
			name:   "OkBelowThreshold",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3}, nil)
				suite.reports.On("AddReport", mock.Anything, markReport()).Once().Return(created, nil)
				suite.reports.On("CountOpenReports", mock.Anything, models.ReportTargetMark, 5).Once().Return(reportsCfg.HideThreshold-1, nil)
			},
		},
		{
			name:   "OkHidesAtThreshold",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3}, nil)
				suite.reports.On("AddReport", mock.Anything, markReport()).Once().Return(created, nil)
				suite.reports.On("CountOpenReports", mock.Anything, models.ReportTargetMark, 5).Once().Return(reportsCfg.HideThreshold, nil)
				suite.marks.On("SetMarkHidden", mock.Anything, 5, true).Once().Return(nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkHidden, mock.MatchedBy(func(ev events.MarkHidden) bool {
					return ev.EventID != "" && ev.MarkID == 5 && ev.AuthorID == 3 && ev.ReportsCount == reportsCfg.HideThreshold && ev.ModeratorID == 0
				})).Once().Return(nil)
			},
		},
		{
			name:   "OkAlreadyHidden",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3, Hidden: true}, nil)
				suite.reports.On("AddReport", mock.Anything, markReport()).Once().Return(created, nil)
			},
		},
		{
			name:   "RolledBackHideIsNotPublished",
			report: markReport(),
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(errRepo)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3}, nil)
				suite.reports.On("AddReport", mock.Anything, markReport()).Once().Return(created, nil)
				suite.reports.On("CountOpenReports", mock.Anything, models.ReportTargetMark, 5).Once().Return(reportsCfg.HideThreshold, nil)
				suite.marks.On("SetMarkHidden", mock.Anything, 5, true).Once().Return(nil)
			},
			wantErr: errRepo,
		},
		{
			name:   "OkCheck",
			report: models.Report{ReporterID: 7, TargetType: models.ReportTargetCheck, TargetID: 9, Reason: models.ReportReasonOffensive},
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.checks.On("GetCheckById", mock.Anything, 9).Once().Return(models.Check{ID: 9, UserID: 3}, nil)
				suite.reports.On("AddReport", mock.Anything, mock.Anything).Once().Return(created, nil)
			},
		},
		{
			name:   "ErrOwnCheck",
			report: models.Report{ReporterID: 7, TargetType: models.ReportTargetCheck, TargetID: 9, Reason: models.ReportReasonOffensive},
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.checks.On("GetCheckById", mock.Anything, 9).Once().Return(models.Check{ID: 9, UserID: 7}, nil)
			},
			wantErr: usecase.ErrForbidden,
		},
		{
			name:   "ErrCheckNotFound",
			report: models.Report{ReporterID: 7, TargetType: models.ReportTargetCheck, TargetID: 9, Reason: models.ReportReasonOffensive},
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.checks.On("GetCheckById", mock.Anything, 9).Once().Return(models.Check{}, repository.ErrNotFound)
			},
			wantErr: usecase.ErrNotFound,
		},
		{
			// Comments live in another module: the report is stored by id only.
			name:   "OkCommentWithoutLookup",
			report: models.Report{ReporterID: 7, TargetType: models.ReportTargetComment, TargetID: 42, Reason: models.ReportReasonOther, Comment: "text"},
			setup: func() {
				suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, nil)
				suite.expectTx(nil)
				suite.reports.On("AddReport", mock.Anything, mock.Anything).Once().Return(created, nil)
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

			got, err := suite.uc.Create(context.Background(), tt.report)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
				suite.Equal(created, got)
			}
			suite.reports.AssertExpectations(suite.T())
			suite.marks.AssertExpectations(suite.T())
			suite.publisher.AssertExpectations(suite.T())
		})
	}
}

func (suite *ReportsSuite) TestListQueue() {
	reports := []models.Report{
		{ID: 1, TargetType: models.ReportTargetMark, TargetID: 5},
		{ID: 2, TargetType: models.ReportTargetCheck, TargetID: 9},
		{ID: 3, TargetType: models.ReportTargetMark, TargetID: 6},
	}
	brief := models.MarkBrief{ID: 5, Description: "Свалка", UserID: 3, Hidden: true}

	tests := []struct {
		name       string
		filters    models.GetReportsFilters
		setup      func()
		wantErr    error
		wantChecks func(page models.Page[models.ReportWithTarget])
	}{
		{
			name:    "ErrInvalidStatus",
			filters: models.GetReportsFilters{Status: "weird"},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrInvalidPagination",
			filters: models.GetReportsFilters{Pagination: models.Pagination{Limit: models.MaxLimit + 1}},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrRepo",
			filters: models.GetReportsFilters{Status: models.ReportStatusOpen},
			setup: func() {
				suite.reports.On("GetReports", mock.Anything, models.GetReportsFilters{Status: models.ReportStatusOpen}).Once().
					Return(models.Page[models.Report]{}, errRepo)
			},
			wantErr: errRepo,
		},
		{
			name:    "OkWithTargets",
			filters: models.GetReportsFilters{Status: models.ReportStatusOpen, Pagination: models.Pagination{Limit: 10}},
			setup: func() {
				suite.reports.On("GetReports", mock.Anything, mock.Anything).Once().Return(models.Page[models.Report]{Items: reports, Total: 3}, nil)
				// Only mark ids are looked up; mark 6 no longer exists.
				suite.marks.On("GetMarkBriefs", mock.Anything, []int{5, 6}).Once().Return(map[int]models.MarkBrief{5: brief}, nil)
			},
			wantChecks: func(page models.Page[models.ReportWithTarget]) {
				suite.Equal(3, page.Total)
				suite.Require().Len(page.Items, 3)
				suite.Equal(models.ReportTarget{Type: models.ReportTargetMark, ID: 5, Mark: &brief}, page.Items[0].Target)
				suite.Equal(models.ReportTarget{Type: models.ReportTargetCheck, ID: 9}, page.Items[1].Target)
				suite.Equal(models.ReportTarget{Type: models.ReportTargetMark, ID: 6}, page.Items[2].Target)
			},
		},
		{
			name:    "OkEmpty",
			filters: models.GetReportsFilters{},
			setup: func() {
				suite.reports.On("GetReports", mock.Anything, mock.Anything).Once().Return(models.Page[models.Report]{Items: []models.Report{}}, nil)
				suite.marks.On("GetMarkBriefs", mock.Anything, []int{}).Once().Return(map[int]models.MarkBrief{}, nil)
			},
			wantChecks: func(page models.Page[models.ReportWithTarget]) {
				suite.NotNil(page.Items)
				suite.Empty(page.Items)
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

			page, err := suite.uc.ListQueue(context.Background(), tt.filters)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			tt.wantChecks(page)
		})
	}
}

func (suite *ReportsSuite) TestListMine() {
	suite.Run("ErrInvalidPagination", func() {
		_, err := suite.uc.ListMine(context.Background(), 7, models.Pagination{Offset: -1})
		suite.ErrorIs(err, usecase.ErrInvalidArgument)
	})
	suite.Run("Ok", func() {
		want := models.Page[models.Report]{Items: []models.Report{{ID: 1, ReporterID: 7}}, Total: 1}
		suite.reports.On("GetReports", mock.Anything, models.GetReportsFilters{ReporterID: 7, Pagination: models.Pagination{Limit: 10}}).Once().Return(want, nil)
		got, err := suite.uc.ListMine(context.Background(), 7, models.Pagination{Limit: 10})
		suite.NoError(err)
		suite.Equal(want, got)
	})
}

func (suite *ReportsSuite) TestResolve() {
	moderator := models.Actor{UserID: 2, Role: models.RoleModerator}
	open := models.Report{ID: 11, Status: models.ReportStatusOpen}
	resolved := models.Report{ID: 11, Status: models.ReportStatusResolved}

	tests := []struct {
		name    string
		actor   models.Actor
		status  models.ReportStatus
		setup   func()
		wantErr error
	}{
		{name: "ErrOpenIsNotADecision", actor: moderator, status: models.ReportStatusOpen, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrUnknownStatus", actor: moderator, status: "maybe", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrUserForbidden", actor: models.Actor{UserID: 7, Role: models.RoleUser}, status: models.ReportStatusResolved, wantErr: usecase.ErrForbidden},
		{
			name: "ErrNotFound", actor: moderator, status: models.ReportStatusResolved,
			setup: func() {
				suite.reports.On("GetReportById", mock.Anything, 11).Once().Return(models.Report{}, repository.ErrNotFound)
			},
			wantErr: usecase.ErrNotFound,
		},
		{
			name: "ErrAlreadyDecided", actor: moderator, status: models.ReportStatusDismissed,
			setup: func() {
				suite.reports.On("GetReportById", mock.Anything, 11).Once().Return(resolved, nil)
			},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrDecidedConcurrently", actor: moderator, status: models.ReportStatusResolved,
			setup: func() {
				suite.reports.On("GetReportById", mock.Anything, 11).Once().Return(open, nil)
				suite.reports.On("ResolveReport", mock.Anything, 11, models.ReportStatusResolved, 2).Once().Return(repository.ErrNotFound)
			},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "Ok", actor: moderator, status: models.ReportStatusResolved,
			setup: func() {
				suite.reports.On("GetReportById", mock.Anything, 11).Once().Return(open, nil)
				suite.reports.On("ResolveReport", mock.Anything, 11, models.ReportStatusResolved, 2).Once().Return(nil)
				suite.reports.On("GetReportById", mock.Anything, 11).Once().Return(resolved, nil)
			},
		},
		{
			name: "OkAdmin", actor: models.Actor{UserID: 1, Role: models.RoleAdmin}, status: models.ReportStatusDismissed,
			setup: func() {
				suite.reports.On("GetReportById", mock.Anything, 11).Once().Return(open, nil)
				suite.reports.On("ResolveReport", mock.Anything, 11, models.ReportStatusDismissed, 1).Once().Return(nil)
				suite.reports.On("GetReportById", mock.Anything, 11).Once().Return(resolved, nil)
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

			got, err := suite.uc.Resolve(context.Background(), tt.actor, 11, tt.status)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(resolved, got)
		})
	}
}

// TestCreate_DailyLimitRepoError: a failing count is an internal error,
// not a limit.
func (suite *ReportsSuite) TestCreate_DailyLimitRepoError() {
	suite.reports.On("CountReportsByReporterSince", mock.Anything, 7, mock.Anything).Once().Return(0, errRepo)

	_, err := suite.uc.Create(context.Background(), markReport())

	suite.ErrorIs(err, errRepo)
	suite.False(errors.Is(err, usecase.ErrTooManyRequests))
}
