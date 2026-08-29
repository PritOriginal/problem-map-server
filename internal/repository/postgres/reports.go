package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
)

// ReportsRepository stores user reports (moderation).
type ReportsRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewReports(db *sqlx.DB, c *trmsqlx.CtxGetter) *ReportsRepository {
	return &ReportsRepository{
		db:     db,
		getter: c,
	}
}

const reportColumns = "report_id, reporter_id, target_type, target_id, reason, comment, status, resolved_by, resolved_at, created_at"

// AddReport stores the report and returns it as written. A second report
// by the same user on the same target is repository.ErrExists.
func (r *ReportsRepository) AddReport(ctx context.Context, report models.Report) (models.Report, error) {
	const op = "storage.postgres.AddReport"

	query := `
		INSERT INTO reports (reporter_id, target_type, target_id, reason, comment)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + reportColumns

	var created models.Report
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &created, query, report.ReporterID, report.TargetType, report.TargetID, report.Reason, report.Comment); err != nil {
		return models.Report{}, wrapPgError(op, err)
	}

	return created, nil
}

func (r *ReportsRepository) GetReportById(ctx context.Context, id int) (models.Report, error) {
	const op = "storage.postgres.GetReportById"

	var report models.Report
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &report, "SELECT "+reportColumns+" FROM reports WHERE report_id = $1", id); err != nil {
		return report, wrapPgError(op, err)
	}

	return report, nil
}

// GetReports returns a page of reports matching the filters, oldest first
// (the moderation queue is worked through in order of arrival).
func (r *ReportsRepository) GetReports(ctx context.Context, filters models.GetReportsFilters) (models.Page[models.Report], error) {
	const op = "storage.postgres.GetReports"

	q := newListQuery(reportColumns, "reports").
		OrderBy("created_at ASC, report_id ASC").
		Paginate(filters.Pagination)

	if filters.Status != "" {
		q.Where("status = ?", filters.Status)
	}
	if filters.TargetType != "" {
		q.Where("target_type = ?", filters.TargetType)
	}
	if filters.ReporterID > 0 {
		q.Where("reporter_id = ?", filters.ReporterID)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Report](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// ResolveReport sets a final status of the report and records who decided
// it and when. Only an open report is updated: repository.ErrNotFound
// when the report does not exist or is already decided.
func (r *ReportsRepository) ResolveReport(ctx context.Context, id int, status models.ReportStatus, resolvedBy int) error {
	const op = "storage.postgres.ResolveReport"

	query := `
		UPDATE reports SET status = $2, resolved_by = $3, resolved_at = NOW()
		WHERE report_id = $1 AND status = $4`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, id, status, resolvedBy, models.ReportStatusOpen)
	if err != nil {
		return wrapPgError(op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// CountOpenReports returns the number of open reports on the target.
func (r *ReportsRepository) CountOpenReports(ctx context.Context, targetType models.ReportTargetType, targetId int) (int, error) {
	const op = "storage.postgres.CountOpenReports"

	var n int
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &n,
		"SELECT COUNT(*) FROM reports WHERE target_type = $1 AND target_id = $2 AND status = $3",
		targetType, targetId, models.ReportStatusOpen); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return n, nil
}

// CountReportsByReporterSince returns how many reports the user filed
// since the moment (the daily limit).
func (r *ReportsRepository) CountReportsByReporterSince(ctx context.Context, reporterId int, since time.Time) (int, error) {
	const op = "storage.postgres.CountReportsByReporterSince"

	var n int
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &n,
		"SELECT COUNT(*) FROM reports WHERE reporter_id = $1 AND created_at >= $2", reporterId, since); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return n, nil
}

// MoveMarkReports moves the reports on the mark to target (a merge of
// duplicates); a reporter who already reported the target keeps a single
// report. Call it inside a transaction.
func (r *ReportsRepository) MoveMarkReports(ctx context.Context, markId, targetId int) error {
	const op = "storage.postgres.MoveMarkReports"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, `
		DELETE FROM reports r
		WHERE r.target_type = $3 AND r.target_id = $1
			AND EXISTS (SELECT 1 FROM reports r2 WHERE r2.target_type = $3 AND r2.target_id = $2 AND r2.reporter_id = r.reporter_id)`,
		markId, targetId, models.ReportTargetMark); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if _, err := tr.ExecContext(ctx,
		"UPDATE reports SET target_id = $2 WHERE target_type = $3 AND target_id = $1",
		markId, targetId, models.ReportTargetMark); err != nil {
		return wrapPgError(op, err)
	}

	return nil
}
