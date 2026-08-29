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

// markColumns lists the mark columns plus the follower aggregates. The
// placeholder is the viewer id (see models.ViewerFromContext); it must be
// bound through listQuery.ColumnArgs or numbered explicitly in raw queries.
const markColumns = "marks.mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, marks.user_id, marks.created_at, marks.updated_at, " +
	followerColumns

const followerColumns = "(SELECT COUNT(*) FROM mark_followers f WHERE f.mark_id = marks.mark_id)::int AS followers_count, " +
	"EXISTS(SELECT 1 FROM mark_followers f WHERE f.mark_id = marks.mark_id AND f.user_id = ?) AS is_following"

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
	q.ColumnArgs(models.ViewerFromContext(ctx))

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
	q.ColumnArgs(models.ViewerFromContext(ctx), filters.Lon, filters.Lat)

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

	query, args, err := bind("SELECT "+markColumns+" FROM marks WHERE mark_id = ?", []any{models.ViewerFromContext(ctx), id})
	if err != nil {
		return mark, fmt.Errorf("%s: %w", op, err)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &mark, query, args...); err != nil {
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
		Where("marks.user_id = ?", userId).
		OrderBy(marksOrderBy(models.MarksSortCreatedAt, models.SortDesc)).
		Paginate(p)
	q.ColumnArgs(models.ViewerFromContext(ctx))

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

// GetSimilarMarks returns active marks of the same type within
// filters.RadiusM meters of the point, nearest first (duplicate detection).
func (r *MarksRepository) GetSimilarMarks(ctx context.Context, filters models.GetSimilarMarksFilters) ([]models.MarkWithDistance, error) {
	const op = "storage.postgres.GetSimilarMarks"

	const point = "ST_SetSRID(ST_MakePoint(?, ?), 4326)::geography"

	// Cap the result: a client only needs a handful of candidates.
	q := newListQuery(
		markColumns+", ST_Distance(geom::geography, "+point+") AS distance_m",
		"marks",
	).
		OrderBy("distance_m ASC, mark_id ASC").
		Paginate(models.Pagination{Limit: similarMarksLimit})
	q.ColumnArgs(models.ViewerFromContext(ctx), filters.Lon, filters.Lat)

	q.Where("ST_DWithin(geom::geography, "+point+", ?)", filters.Lon, filters.Lat, filters.RadiusM)
	q.Where("type_mark_id = ?", filters.MarkTypeID)
	q.Where("mark_status_id IN (?)", models.ActiveMarkStatuses())
	if filters.ExcludeMarkID > 0 {
		q.Where("marks.mark_id <> ?", filters.ExcludeMarkID)
	}

	query, args, err := q.selectQuery()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	marks := []models.MarkWithDistance{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &marks, query, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return marks, nil
}

// similarMarksLimit bounds the number of duplicate candidates returned.
const similarMarksLimit = 20

// UpdateMark changes the given fields and bumps updated_at.
func (r *MarksRepository) UpdateMark(ctx context.Context, markId int, upd models.MarkUpdate) error {
	const op = "storage.postgres.UpdateMark"

	query := `
		UPDATE marks SET
			description = COALESCE($2, description),
			type_mark_id = COALESCE($3, type_mark_id),
			updated_at = NOW()
		WHERE mark_id = $1
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, markId, upd.Description, upd.MarkTypeID)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%s: %w: unknown mark type", op, repository.ErrInvalidReference)
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// DeleteMark removes the mark together with its checks, tasks, status
// history and followers. Those tables reference marks without ON DELETE
// CASCADE (except mark_followers), so the rows are deleted explicitly;
// call it inside a transaction.
func (r *MarksRepository) DeleteMark(ctx context.Context, markId int) error {
	const op = "storage.postgres.DeleteMark"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	for _, query := range []string{
		"DELETE FROM checks WHERE mark_id = $1",
		"DELETE FROM tasks WHERE mark_id = $1",
		"DELETE FROM mark_status_history WHERE mark_id = $1",
		"DELETE FROM mark_followers WHERE mark_id = $1",
	} {
		if _, err := tr.ExecContext(ctx, query, markId); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	res, err := tr.ExecContext(ctx, "DELETE FROM marks WHERE mark_id = $1", markId)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// FollowMark subscribes the user to the mark; already following is not an error.
func (r *MarksRepository) FollowMark(ctx context.Context, userId, markId int) error {
	const op = "storage.postgres.FollowMark"

	query := "INSERT INTO mark_followers (user_id, mark_id) VALUES ($1, $2) ON CONFLICT DO NOTHING"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, query, userId, markId); err != nil {
		if isForeignKeyViolation(err) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// UnfollowMark removes the subscription; not following is not an error.
func (r *MarksRepository) UnfollowMark(ctx context.Context, userId, markId int) error {
	const op = "storage.postgres.UnfollowMark"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, "DELETE FROM mark_followers WHERE user_id = $1 AND mark_id = $2", userId, markId); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// GetFollowedMarks returns the marks the user follows, newest subscription first.
func (r *MarksRepository) GetFollowedMarks(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error) {
	const op = "storage.postgres.GetFollowedMarks"

	q := newListQuery(markColumns, "marks JOIN mark_followers mf ON mf.mark_id = marks.mark_id").
		Where("mf.user_id = ?", userId).
		OrderBy("mf.created_at DESC, marks.mark_id DESC").
		Paginate(p)
	q.ColumnArgs(models.ViewerFromContext(ctx))

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Mark](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// GetFollowerIDs returns the ids of users following the mark (for notifications).
func (r *MarksRepository) GetFollowerIDs(ctx context.Context, markId int) ([]int, error) {
	const op = "storage.postgres.GetFollowerIDs"

	ids := []int{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &ids, "SELECT user_id FROM mark_followers WHERE mark_id = $1 ORDER BY user_id", markId); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return ids, nil
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

// LockMark takes a row lock on the mark until the surrounding transaction
// ends, serialising the vote counting and stage resolution of the mark.
func (r *MarksRepository) LockMark(ctx context.Context, markId int) error {
	const op = "storage.postgres.LockMark"

	var id int

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, "SELECT mark_id FROM marks WHERE mark_id = $1 FOR UPDATE", markId); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return repository.ErrNotFound
		default:
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
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
