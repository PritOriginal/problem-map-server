package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type MarksRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewMarks(db *sqlx.DB, c *trmsqlx.CtxGetter) *MarksRepository {
	return &MarksRepository{
		db:     db,
		getter: c,
	}
}

const markColumns = "mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, created_at, updated_at"

// marksSortColumns is the whitelist of sortable columns; the empty key is
// the default. Only values from this map ever reach the ORDER BY clause.
var marksSortColumns = map[models.MarksSort]string{
	"":                        "created_at",
	models.MarksSortCreatedAt: "created_at",
	models.MarksSortUpdatedAt: "updated_at",
}

// marksOrderBy maps the public sort keys to an ORDER BY clause. mark_id is
// always appended as a tie-breaker so that pagination is stable.
func marksOrderBy(sort models.MarksSort, order models.SortOrder) string {
	column, ok := marksSortColumns[sort]
	if !ok {
		column = marksSortColumns[""]
	}
	dir := "DESC"
	if order == models.SortAsc {
		dir = "ASC"
	}
	return fmt.Sprintf("%s %s, mark_id %s", column, dir, dir)
}

func (r *MarksRepository) GetMarks(ctx context.Context, filters models.GetMarksFilters) (models.Page[models.Mark], error) {
	const op = "storage.postgres.GetMarks"

	q := newListQuery(markColumns, "marks").
		OrderBy(marksOrderBy(filters.Sort, filters.Order)).
		Paginate(filters.Pagination)

	if len(filters.MarkStatusIds) > 0 {
		q.Where("mark_status_id IN (?)", filters.MarkStatusIds)
	}
	if len(filters.MarkTypeIds) > 0 {
		q.Where("type_mark_id IN (?)", filters.MarkTypeIds)
	}
	if filters.UserID > 0 {
		q.Where("user_id = ?", filters.UserID)
	}
	if b := filters.BBox; b != nil {
		// Uses the GiST index on marks.geom.
		q.Where("ST_Intersects(geom, ST_MakeEnvelope(?, ?, ?, ?, 4326))", b.MinLon, b.MinLat, b.MaxLon, b.MaxLat)
	}
	if !filters.CreatedFrom.IsZero() {
		q.Where("created_at >= ?", filters.CreatedFrom)
	}
	if !filters.CreatedTo.IsZero() {
		q.Where("created_at <= ?", filters.CreatedTo)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Mark](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

func (r *MarksRepository) GetMarksNearby(ctx context.Context, filters models.GetMarksNearbyFilters) (models.Page[models.MarkWithDistance], error) {
	const op = "storage.postgres.GetMarksNearby"

	const point = "ST_SetSRID(ST_MakePoint(?, ?), 4326)::geography"

	// ST_DWithin on geography uses idx_marks_geom_geog (see migration 000028).
	q := newListQuery(
		markColumns+", ST_Distance(geom::geography, "+point+") AS distance_m",
		"marks",
	).
		OrderBy("distance_m ASC, mark_id ASC").
		Paginate(filters.Pagination)
	q.ColumnArgs(filters.Lon, filters.Lat)

	q.Where("ST_DWithin(geom::geography, "+point+", ?)", filters.Lon, filters.Lat, filters.RadiusM)
	if len(filters.MarkStatusIds) > 0 {
		q.Where("mark_status_id IN (?)", filters.MarkStatusIds)
	}
	if len(filters.MarkTypeIds) > 0 {
		q.Where("type_mark_id IN (?)", filters.MarkTypeIds)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.MarkWithDistance](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

func (r *MarksRepository) GetMarkById(ctx context.Context, id int) (models.Mark, error) {
	const op = "storage.postgres.GetMarkById"

	mark := models.Mark{}

	query := `SELECT
				mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, created_at, updated_at 
			FROM 
				marks 
			WHERE 
				mark_id = $1
			`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &mark, query, id); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return mark, repository.ErrNotFound
		default:
			return mark, fmt.Errorf("%s: %w", op, err)
		}
	}

	return mark, nil
}

func (r *MarksRepository) GetMarksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error) {
	const op = "storage.postgres.GetMarksByUserId"

	q := newListQuery(markColumns, "marks").
		Where("user_id = ?", userId).
		OrderBy(marksOrderBy(models.MarksSortCreatedAt, models.SortDesc)).
		Paginate(p)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Mark](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

func (r *MarksRepository) AddMark(ctx context.Context, mark models.Mark) (int64, error) {
	const op = "storage.postgres.AddMark"

	var id int64

	query := `
			INSERT INTO 
				marks (description, geom, type_mark_id, user_id) 
			VALUES 
				($1, ST_GeomFromEWKB($2), $3, $4)
			RETURNING mark_id
			`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, mark.Description, &mark.Geom, mark.MarkTypeID, mark.UserID); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (r *MarksRepository) GetMarkTypes(ctx context.Context) ([]models.MarkType, error) {
	const op = "storage.postgres.GetMarkTypes"

	types := []models.MarkType{}

	query := "SELECT * FROM types_marks ORDER BY name"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &types, query); err != nil {
		return types, fmt.Errorf("%s: %w", op, err)
	}

	return types, nil
}

func (r *MarksRepository) GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error) {
	const op = "storage.postgres.GetMarkTypes"

	statuses := []models.MarkStatus{}

	query := "SELECT * FROM mark_statuses ORDER BY mark_status_id"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &statuses, query); err != nil {
		return statuses, fmt.Errorf("%s: %w", op, err)
	}

	return statuses, nil
}

func (r *MarksRepository) UpdateMarkStatus(ctx context.Context, markId int, markStatusId models.MarkStatusType) error {
	const op = "storage.postgres.UpdateMarkStatus"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, "UPDATE marks SET mark_status_id = $1 WHERE mark_id = $2", markStatusId, markId); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (r *MarksRepository) GetMarkStatusHistoryByMarkId(ctx context.Context, markId int) ([]models.MarkStatusHistoryItem, error) {
	const op = "storage.postgres.GetMarkStatusHistoryByMarkId"

	historyItems := []models.MarkStatusHistoryItem{}

	query := `
		SELECT 
			* 
		FROM 
			mark_status_history 
		WHERE
			mark_id = $1 
		ORDER BY
			changed_at ASC, id ASC
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &historyItems, query, markId); err != nil {
		return historyItems, fmt.Errorf("%s: %w", op, err)
	}

	return historyItems, nil
}

func (r *MarksRepository) GetLastMarkStatusHistoryItemWithStatus(ctx context.Context, markId int, newMarkStatusId models.MarkStatusType) (models.MarkStatusHistoryItem, error) {
	const op = "storage.postgres.GetLastMarkStatusHistoryItemWithStatus"

	var historyItem models.MarkStatusHistoryItem

	query := `
		SELECT 
			* 
		FROM 
			mark_status_history 
		WHERE 
			mark_id = $1 AND new_mark_status_id = $2 
		ORDER BY 
			changed_at DESC, id DESC 
		LIMIT 1
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &historyItem, query, markId, newMarkStatusId); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return historyItem, repository.ErrNotFound
		default:
			return historyItem, fmt.Errorf("%s: %w", op, err)
		}
	}

	return historyItem, nil
}

func (r *MarksRepository) GetLastMarkStatusHistoryItem(ctx context.Context, markId int) (models.MarkStatusHistoryItem, error) {
	const op = "storage.postgres.GetLastMarkStatusHistoryItemWithStatus"

	var historyItem models.MarkStatusHistoryItem

	query := `
		SELECT 
			* 
		FROM 
			mark_status_history 
		WHERE 
			mark_id = $1 
		ORDER BY 
			changed_at DESC, id DESC 
		LIMIT 1
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &historyItem, query, markId); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return historyItem, repository.ErrNotFound
		default:
			return historyItem, fmt.Errorf("%s: %w", op, err)
		}
	}

	return historyItem, nil
}

func (r *MarksRepository) GetDistancesFromMarkToPoint(ctx context.Context, filters models.GetDistanceFromMarkToPointFilters) ([]models.DistanceFromMarkToPoint, error) {
	const op = "storage.postgres.GetDistancesFromMarkToPoint"

	distances := []models.DistanceFromMarkToPoint{}

	// ST_DWithin on geography lets the planner use the GiST indexes on
	// marks.geom and users.home_point instead of a full cross join.
	query := `
		SELECT
			m.mark_id,
			u.user_id,
			ROUND((ST_DistanceSphere(m.geom, u.home_point) / 1000.0)::numeric, 2) AS distance_km
		FROM
			marks m
		JOIN
			users u ON ST_DWithin(m.geom::geography, u.home_point::geography, $2)
		WHERE
			m.mark_status_id = ANY($1)
		ORDER BY m.mark_id, u.user_id
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &distances, query, pq.Array(filters.MarkStatusIds), filters.MaxRadius); err != nil {
		return distances, fmt.Errorf("%s: %w", op, err)
	}

	return distances, nil
}
