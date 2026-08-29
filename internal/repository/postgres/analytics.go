package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/guregu/null/v6"
	"github.com/jmoiron/sqlx"
)

// AnalyticsRepository computes aggregates over marks and their status
// history. Timestamps are TIMESTAMPTZ, so bounds are bound as instants; the
// periods of the timeline are truncated in the session time zone, which the
// DSN pins to UTC (config.DatabaseConfig.DSN).
type AnalyticsRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewAnalytics(db *sqlx.DB, c *trmsqlx.CtxGetter) *AnalyticsRepository {
	return &AnalyticsRepository{
		db:     db,
		getter: c,
	}
}

// markConds renders the conditions selecting the marks (aliased m) the
// analytics are computed over; args are appended and placeholders numbered
// from the current length of args.
func markConds(args *[]any, boundaryID, markTypeID int, r models.DateRange) []string {
	// Hidden marks (spam under moderation) never enter public statistics.
	conds := []string{"NOT m.hidden"}
	if boundaryID > 0 {
		*args = append(*args, boundaryID)
		conds = append(conds, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM admin_boundaries b WHERE b.id = $%d AND ST_Contains(b.geom, m.geom))", len(*args)))
	}
	if markTypeID > 0 {
		*args = append(*args, markTypeID)
		conds = append(conds, fmt.Sprintf("m.type_mark_id = $%d", len(*args)))
	}
	if !r.From.IsZero() {
		*args = append(*args, r.From)
		conds = append(conds, fmt.Sprintf("m.created_at >= $%d", len(*args)))
	}
	if !r.To.IsZero() {
		*args = append(*args, r.To)
		conds = append(conds, fmt.Sprintf("m.created_at <= $%d", len(*args)))
	}
	return conds
}

type kpiRow struct {
	Total              int        `db:"total"`
	Refuted            int        `db:"refuted"`
	OpenOlderThan30d   int        `db:"open_older_than_30d"`
	AvgConfirmHours    null.Float `db:"avg_confirm_hours"`
	MedianConfirmHours null.Float `db:"median_confirm_hours"`
	AvgCloseHours      null.Float `db:"avg_close_hours"`
}

type statusCountRow struct {
	StatusID int `db:"mark_status_id"`
	Count    int `db:"count"`
}

func (r *AnalyticsRepository) GetKPI(ctx context.Context, filters models.AnalyticsFilters) (models.KPI, error) {
	const op = "storage.postgres.GetKPI"

	var args []any
	where := strings.Join(markConds(&args, filters.BoundaryID, filters.MarkTypeID, filters.DateRange), " AND ")

	// Per mark: the first moment it was unconfirmed (its creation, logged by
	// the insert trigger; falls back to created_at for marks predating the
	// trigger) and the first moments it became confirmed / closed.
	query := fmt.Sprintf(`
		WITH fm AS (
			SELECT m.mark_id, m.mark_status_id, m.created_at
			FROM marks m
			WHERE %s
		),
		durations AS (
			SELECT
				fm.mark_id,
				COALESCE(MIN(h.changed_at) FILTER (WHERE h.new_mark_status_id = %d), fm.created_at) AS unconfirmed_at,
				MIN(h.changed_at) FILTER (WHERE h.new_mark_status_id = %d) AS confirmed_at,
				MIN(h.changed_at) FILTER (WHERE h.new_mark_status_id = %d) AS closed_at
			FROM fm
			LEFT JOIN mark_status_history h ON h.mark_id = fm.mark_id
			GROUP BY fm.mark_id, fm.created_at
		)
		SELECT
			(SELECT COUNT(*) FROM fm) AS total,
			(SELECT COUNT(*) FROM fm WHERE mark_status_id = %d) AS refuted,
			(SELECT COUNT(*) FROM fm
				WHERE mark_status_id NOT IN (%d, %d) AND created_at < NOW() - INTERVAL '30 days') AS open_older_than_30d,
			AVG(EXTRACT(EPOCH FROM (confirmed_at - unconfirmed_at)) / 3600) AS avg_confirm_hours,
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (confirmed_at - unconfirmed_at)) / 3600) AS median_confirm_hours,
			AVG(EXTRACT(EPOCH FROM (closed_at - unconfirmed_at)) / 3600) AS avg_close_hours
		FROM durations
	`, where,
		models.UnconfirmedStatus, models.ConfirmedStatus, models.ClosedStatus,
		models.RefutedStatus, models.ClosedStatus, models.RefutedStatus)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)

	var row kpiRow
	if err := tr.GetContext(ctx, &row, query, args...); err != nil {
		return models.KPI{}, fmt.Errorf("%s: %w", op, err)
	}

	byStatusQuery := fmt.Sprintf(`
		SELECT m.mark_status_id, COUNT(*) AS count
		FROM marks m
		WHERE %s
		GROUP BY m.mark_status_id
	`, where)

	var statuses []statusCountRow
	if err := tr.SelectContext(ctx, &statuses, byStatusQuery, args...); err != nil {
		return models.KPI{}, fmt.Errorf("%s: %w", op, err)
	}

	// Organization load: marks assigned to a service and those overdue
	// (see overdueColumn).
	byOrgQuery := fmt.Sprintf(`
		SELECT
			o.organization_id,
			o.name,
			COUNT(m.mark_id)::int AS total,
			COUNT(m.mark_id) FILTER (WHERE m.sla_due_at < NOW() AND m.mark_status_id IN (%d, %d))::int AS overdue
		FROM organizations o
		LEFT JOIN marks m ON m.organization_id = o.organization_id AND %s
		GROUP BY o.organization_id, o.name
		ORDER BY o.name, o.organization_id
	`, models.ConfirmedStatus, models.InProgressStatus, where)

	byOrg := []models.OrganizationKPI{}
	if err := tr.SelectContext(ctx, &byOrg, byOrgQuery, args...); err != nil {
		return models.KPI{}, fmt.Errorf("%s: %w", op, err)
	}

	kpi := models.KPI{
		Total:              row.Total,
		ByOrganization:     byOrg,
		ByStatus:           make(map[int]int, len(statuses)),
		AvgConfirmHours:    row.AvgConfirmHours,
		MedianConfirmHours: row.MedianConfirmHours,
		AvgCloseHours:      row.AvgCloseHours,
		OpenOlderThan30d:   row.OpenOlderThan30d,
	}
	for _, s := range statuses {
		kpi.ByStatus[s.StatusID] = s.Count
	}
	if row.Total > 0 {
		kpi.RefutedShare = float64(row.Refuted) / float64(row.Total)
	}
	assigned, overdue := 0, 0
	for _, o := range byOrg {
		assigned += o.Total
		overdue += o.Overdue
	}
	if assigned > 0 {
		kpi.SLABreachShare = float64(overdue) / float64(assigned)
	}
	return kpi, nil
}

// stepIntervals maps a validated step to the generate_series interval; the
// step itself is also a date_trunc field name.
var stepIntervals = map[models.TimeseriesStep]string{
	models.StepDay:   "1 day",
	models.StepWeek:  "1 week",
	models.StepMonth: "1 month",
}

func (r *AnalyticsRepository) GetTimeseries(ctx context.Context, filters models.TimeseriesFilters) ([]models.TimeseriesPoint, error) {
	const op = "storage.postgres.GetTimeseries"

	interval, ok := stepIntervals[filters.Step]
	if !ok {
		return nil, fmt.Errorf("%s: unknown step %q", op, filters.Step)
	}
	step := string(filters.Step)

	// The range bounds the events (creation / transition time), not the
	// marks' creation, so the mark set is filtered by boundary and type only.
	args := []any{filters.From, filters.To}
	where := strings.Join(markConds(&args, filters.BoundaryID, filters.MarkTypeID, models.DateRange{}), " AND ")

	query := fmt.Sprintf(`
		WITH periods AS (
			SELECT generate_series(
				date_trunc('%[1]s', $1::timestamptz),
				date_trunc('%[1]s', $2::timestamptz),
				INTERVAL '%[2]s'
			) AS period
		),
		fm AS (
			SELECT m.mark_id, m.created_at
			FROM marks m
			WHERE %[3]s
		),
		created AS (
			SELECT date_trunc('%[1]s', created_at) AS period, COUNT(*) AS created
			FROM fm
			WHERE created_at >= $1 AND created_at <= $2
			GROUP BY 1
		),
		transitions AS (
			SELECT
				date_trunc('%[1]s', h.changed_at) AS period,
				COUNT(*) FILTER (WHERE h.new_mark_status_id = %[4]d) AS confirmed,
				COUNT(*) FILTER (WHERE h.new_mark_status_id = %[5]d) AS closed,
				COUNT(*) FILTER (WHERE h.new_mark_status_id = %[6]d) AS refuted
			FROM mark_status_history h
			JOIN fm ON fm.mark_id = h.mark_id
			WHERE h.changed_at >= $1 AND h.changed_at <= $2
			GROUP BY 1
		)
		SELECT
			p.period,
			COALESCE(c.created, 0) AS created,
			COALESCE(t.confirmed, 0) AS confirmed,
			COALESCE(t.closed, 0) AS closed,
			COALESCE(t.refuted, 0) AS refuted
		FROM periods p
		LEFT JOIN created c ON c.period = p.period
		LEFT JOIN transitions t ON t.period = p.period
		ORDER BY p.period
	`, step, interval, where, models.ConfirmedStatus, models.ClosedStatus, models.RefutedStatus)

	points := []models.TimeseriesPoint{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &points, query, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	for i := range points {
		points[i].Period = points[i].Period.UTC()
	}
	return points, nil
}

func (r *AnalyticsRepository) GetTopTypes(ctx context.Context, filters models.TopTypesFilters) ([]models.TopType, error) {
	const op = "storage.postgres.GetTopTypes"

	var args []any
	where := strings.Join(markConds(&args, filters.BoundaryID, 0, filters.DateRange), " AND ")
	args = append(args, filters.Limit)

	query := fmt.Sprintf(`
		WITH fm AS (
			SELECT m.type_mark_id
			FROM marks m
			WHERE %s
		),
		total AS (SELECT COUNT(*) AS n FROM fm)
		SELECT
			t.type_mark_id AS mark_type_id,
			t.name,
			COUNT(fm.type_mark_id) AS count,
			CASE WHEN total.n = 0 THEN 0 ELSE COUNT(fm.type_mark_id)::float8 / total.n END AS share
		FROM types_marks t
		CROSS JOIN total
		LEFT JOIN fm ON fm.type_mark_id = t.type_mark_id
		GROUP BY t.type_mark_id, t.name, total.n
		ORDER BY count DESC, t.type_mark_id
		LIMIT $%d
	`, where, len(args))

	types := []models.TopType{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &types, query, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return types, nil
}
