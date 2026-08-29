package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

// reportsPerDayWindow is the rolling window of the daily report limit.
const reportsPerDayWindow = 24 * time.Hour

type ReportsRepository interface {
	// AddReport stores the report; repository.ErrExists when the reporter
	// already reported the target.
	AddReport(ctx context.Context, report models.Report) (models.Report, error)
	GetReportById(ctx context.Context, id int) (models.Report, error)
	GetReports(ctx context.Context, filters models.GetReportsFilters) (models.Page[models.Report], error)
	// ResolveReport sets a final status on an open report;
	// repository.ErrNotFound when the report is missing or already decided.
	ResolveReport(ctx context.Context, id int, status models.ReportStatus, resolvedBy int) error
	CountOpenReports(ctx context.Context, targetType models.ReportTargetType, targetId int) (int, error)
	CountReportsByReporterSince(ctx context.Context, reporterId int, since time.Time) (int, error)
}

// ReportsMarksRepository is the part of the marks storage the reports need.
type ReportsMarksRepository interface {
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	LockMark(ctx context.Context, markId int) error
	SetMarkHidden(ctx context.Context, markId int, hidden bool) error
	GetMarkBriefs(ctx context.Context, ids []int) (map[int]models.MarkBrief, error)
}

// ReportsChecksRepository resolves the check a report is about.
type ReportsChecksRepository interface {
	GetCheckById(ctx context.Context, id int) (models.Check, error)
}

type ReportsRepositories struct {
	Reports ReportsRepository
	Marks   ReportsMarksRepository
	Checks  ReportsChecksRepository
}

// Reports handles user complaints: filing, the moderation queue and the
// automatic hiding of marks that collected too many open reports.
type Reports struct {
	log       *slog.Logger
	cfg       config.ReportsConfig
	trManager trm.Manager
	repos     ReportsRepositories
	events    events.Publisher
}

func NewReports(log *slog.Logger, cfg config.ReportsConfig, trManager trm.Manager, repos ReportsRepositories) *Reports {
	if cfg.HideThreshold <= 0 {
		cfg.HideThreshold = 5
	}
	if cfg.MaxPerDay <= 0 {
		cfg.MaxPerDay = 20
	}
	return &Reports{
		log:       log,
		cfg:       cfg,
		trManager: trManager,
		repos:     repos,
		events:    events.NoopPublisher{},
	}
}

// WithEvents sets the publisher of mark.hidden events. Without it events
// are dropped.
func (uc *Reports) WithEvents(p events.Publisher) *Reports {
	if p != nil {
		uc.events = p
	}
	return uc
}

// Create files the report of reporter on the target.
//
// Rules: the target must be valid (ErrInvalidArgument); a mark or a check
// must exist (ErrNotFound) and must not belong to the reporter
// (ErrForbidden) — comments are accepted without an existence check
// because their storage lives in another module; a user files at most
// cfg.MaxPerDay reports per rolling 24 hours (ErrTooManyRequests) and one
// report per target (ErrConflict). When the open reports on a mark reach
// cfg.HideThreshold the mark is hidden and mark.hidden is published after
// the commit.
func (uc *Reports) Create(ctx context.Context, report models.Report) (models.Report, error) {
	const op = "usecase.Reports.Create"

	if err := report.Validate(); err != nil {
		return models.Report{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if report.ReporterID <= 0 {
		return models.Report{}, fmt.Errorf("%s: %w", op, ErrUnauthorized)
	}

	if err := uc.checkDailyLimit(ctx, report.ReporterID); err != nil {
		return models.Report{}, fmt.Errorf("%s: %w", op, err)
	}

	var pending events.Pending
	ctx = events.WithPending(ctx, &pending)

	var created models.Report
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		// The mark row is locked so that concurrent reports count the open
		// ones consistently and the threshold fires exactly once.
		var mark models.Mark
		switch report.TargetType {
		case models.ReportTargetMark:
			if err := uc.repos.Marks.LockMark(ctx, report.TargetID); err != nil {
				return err
			}
			var err error
			mark, err = uc.repos.Marks.GetMarkById(ctx, report.TargetID)
			if err != nil {
				return err
			}
			if mark.UserID == report.ReporterID {
				return fmt.Errorf("%w: own mark", ErrForbidden)
			}
		case models.ReportTargetCheck:
			check, err := uc.repos.Checks.GetCheckById(ctx, report.TargetID)
			if err != nil {
				return err
			}
			if check.UserID == report.ReporterID {
				return fmt.Errorf("%w: own check", ErrForbidden)
			}
		case models.ReportTargetComment:
			// Comments are stored by another module: only the id is validated.
		}

		var err error
		created, err = uc.repos.Reports.AddReport(ctx, report)
		if err != nil {
			return err
		}

		if report.TargetType != models.ReportTargetMark || mark.Hidden {
			return nil
		}
		n, err := uc.repos.Reports.CountOpenReports(ctx, models.ReportTargetMark, mark.ID)
		if err != nil {
			return err
		}
		if n < uc.cfg.HideThreshold {
			return nil
		}
		if err := uc.repos.Marks.SetMarkHidden(ctx, mark.ID, true); err != nil {
			return err
		}
		uc.log.Info("mark hidden by reports", slog.Int("mark_id", mark.ID), slog.Int("reports", n))
		events.Collect(ctx, events.NewMarkHidden(mark.ID, mark.UserID, n, 0))
		return nil
	})
	if err != nil {
		return models.Report{}, mapRepoErr(op, err)
	}

	pending.Flush(ctx, uc.log, uc.events)

	return created, nil
}

// checkDailyLimit returns ErrTooManyRequests when the user has already
// filed cfg.MaxPerDay reports in the last 24 hours.
func (uc *Reports) checkDailyLimit(ctx context.Context, reporterId int) error {
	n, err := uc.repos.Reports.CountReportsByReporterSince(ctx, reporterId, time.Now().Add(-reportsPerDayWindow))
	if err != nil {
		return mapRepoErr("usecase.Reports.checkDailyLimit", err)
	}
	if n >= uc.cfg.MaxPerDay {
		return fmt.Errorf("%w: %d reports per day", ErrTooManyRequests, uc.cfg.MaxPerDay)
	}
	return nil
}

// ListQueue returns a page of reports for moderators together with their
// targets (a mark brief for mark reports).
func (uc *Reports) ListQueue(ctx context.Context, filters models.GetReportsFilters) (models.Page[models.ReportWithTarget], error) {
	const op = "usecase.Reports.ListQueue"

	if err := filters.Validate(); err != nil {
		return models.Page[models.ReportWithTarget]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Reports.GetReports(ctx, filters)
	if err != nil {
		return models.Page[models.ReportWithTarget]{}, mapRepoErr(op, err)
	}

	markIDs := make([]int, 0, len(page.Items))
	for _, r := range page.Items {
		if r.TargetType == models.ReportTargetMark {
			markIDs = append(markIDs, r.TargetID)
		}
	}
	briefs, err := uc.repos.Marks.GetMarkBriefs(ctx, markIDs)
	if err != nil {
		return models.Page[models.ReportWithTarget]{}, mapRepoErr(op, err)
	}

	out := models.Page[models.ReportWithTarget]{Items: make([]models.ReportWithTarget, 0, len(page.Items)), Total: page.Total}
	for _, r := range page.Items {
		item := models.ReportWithTarget{Report: r, Target: models.ReportTarget{Type: r.TargetType, ID: r.TargetID}}
		if r.TargetType == models.ReportTargetMark {
			if brief, ok := briefs[r.TargetID]; ok {
				item.Target.Mark = &brief
			}
		}
		out.Items = append(out.Items, item)
	}

	return out, nil
}

// ListMine returns a page of the user's own reports.
func (uc *Reports) ListMine(ctx context.Context, reporterId int, p models.Pagination) (models.Page[models.Report], error) {
	const op = "usecase.Reports.ListMine"

	if err := p.Validate(); err != nil {
		return models.Page[models.Report]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Reports.GetReports(ctx, models.GetReportsFilters{ReporterID: reporterId, Pagination: p})
	if err != nil {
		return page, mapRepoErr(op, err)
	}
	return page, nil
}

// Resolve is the moderator's decision on an open report: status must be
// resolved or dismissed (ErrInvalidArgument); a report that is already
// decided is ErrConflict. The decision does not touch the target: a mark
// that was auto-hidden stays hidden even when every report on it is
// dismissed, the moderator shows it again with Marks.SetHidden.
func (uc *Reports) Resolve(ctx context.Context, actor models.Actor, id int, status models.ReportStatus) (models.Report, error) {
	const op = "usecase.Reports.Resolve"

	if !status.Final() {
		return models.Report{}, fmt.Errorf("%s: %w: status must be resolved or dismissed", op, ErrInvalidArgument)
	}
	if !actor.IsModerator() {
		return models.Report{}, fmt.Errorf("%s: %w", op, ErrForbidden)
	}

	report, err := uc.repos.Reports.GetReportById(ctx, id)
	if err != nil {
		return models.Report{}, mapRepoErr(op, err)
	}
	if report.Status != models.ReportStatusOpen {
		return models.Report{}, fmt.Errorf("%s: %w: report is already %s", op, ErrConflict, report.Status)
	}

	if err := uc.repos.Reports.ResolveReport(ctx, id, status, actor.UserID); err != nil {
		// Decided by another moderator between the read and the update.
		if errors.Is(err, repository.ErrNotFound) {
			return models.Report{}, fmt.Errorf("%s: %w: report is already decided", op, ErrConflict)
		}
		return models.Report{}, mapRepoErr(op, err)
	}

	report, err = uc.repos.Reports.GetReportById(ctx, id)
	if err != nil {
		return models.Report{}, mapRepoErr(op, err)
	}
	uc.log.Info("report decided", slog.Int("report_id", id), slog.String("status", string(status)), slog.Int("user_id", actor.UserID))

	return report, nil
}
