package postgres

import (
	"context"
	"fmt"
	"time"

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
var markColumns = "marks.mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, marks.user_id, marks.created_at, marks.updated_at, " +
	"marks.organization_id, marks.sla_due_at, " + overdueColumn + ", " +
	commentsCountColumn + ", marks.hidden, marks.merged_into_id, " + followerColumns

// commentsCountColumn counts the comments of the mark that are not deleted.
const commentsCountColumn = "(SELECT COUNT(*) FROM mark_comments mc WHERE mc.mark_id = marks.mark_id AND mc.deleted_at IS NULL)::int AS comments_count"

// markBriefColumns are the columns of models.MarkBrief.
const markBriefColumns = "marks.mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, marks.user_id, marks.hidden, marks.created_at"

// visibleMarks restricts a query over marks to the ones the viewer (see
// models.ActorFromContext) may see: hidden marks are shown only to their
// author and to moderators. It is applied to every public list; single-row
// reads (GetMarkById) are unfiltered because the use cases need the raw
// row, and the visibility check is done there (models.Mark.VisibleTo).
func visibleMarks(ctx context.Context, q *listQuery) {
	actor := models.ActorFromContext(ctx)
	if actor.IsModerator() {
		return
	}
	q.Where("(NOT marks.hidden OR marks.user_id = ?)", actor.UserID)
}

// overdueColumn computes is_overdue: the SLA deadline has passed while the
// mark is still waiting for the organization (see models.SLAStatuses). It
// is the single definition used by every mark SELECT.
var overdueColumn = fmt.Sprintf("(marks.sla_due_at IS NOT NULL AND marks.sla_due_at < NOW() AND marks.mark_status_id IN (%d, %d)) AS is_overdue",
	models.ConfirmedStatus, models.InProgressStatus)

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

	q := marksListQuery(ctx, filters)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Mark](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// CountMarks returns the number of marks matching the filters (pagination
// is ignored).
func (r *MarksRepository) CountMarks(ctx context.Context, filters models.GetMarksFilters) (int, error) {
	const op = "storage.postgres.CountMarks"

	query, args, err := marksListQuery(ctx, filters).countQuery()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	var count int
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &count, query, args...); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return count, nil
}

// IterateMarks streams the marks matching the filters to fn one by one
// without materialising the result set; filters.Pagination.Limit (when
// set) caps the rows read. Iteration stops at the first error of fn,
// which is returned as is.
func (r *MarksRepository) IterateMarks(ctx context.Context, filters models.GetMarksFilters, fn func(models.Mark) error) error {
	const op = "storage.postgres.IterateMarks"

	query, args, err := marksListQuery(ctx, filters).selectQuery()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	rows, err := tr.QueryxContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var mark models.Mark
		if err := rows.StructScan(&mark); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		if err := fn(mark); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// marksListQuery builds the filtered, ordered and paginated marks query
// shared by GetMarks, CountMarks and IterateMarks.
func marksListQuery(ctx context.Context, filters models.GetMarksFilters) *listQuery {
	q := newListQuery(markColumns, "marks").
		OrderBy(marksOrderBy(filters.Sort, filters.Order)).
		Paginate(filters.Pagination)
	q.ColumnArgs(models.ViewerFromContext(ctx))
	visibleMarks(ctx, q)

	if len(filters.IDs) > 0 {
		q.Where("mark_id IN (?)", filters.IDs)
	}
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
	if !filters.UpdatedSince.IsZero() {
		q.Where("updated_at > ?", filters.UpdatedSince)
	}

	return q
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
	visibleMarks(ctx, q)

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
		return mark, wrapPgError(op, err)
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
	visibleMarks(ctx, q)

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
	visibleMarks(ctx, q)

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
		// The only foreign key touched here is the mark type.
		return wrapPgError(op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// DeleteMark removes the mark together with its checks, tasks, status
// history and followers, and leaves a tombstone (mark_tombstones) for
// incremental sync. Those tables reference marks without ON DELETE
// CASCADE (except mark_followers), so the rows are deleted explicitly;
// call it inside a transaction.
func (r *MarksRepository) DeleteMark(ctx context.Context, markId int) error {
	const op = "storage.postgres.DeleteMark"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)

	// The tombstone goes first so that the transaction fails early when the
	// mark does not exist (nothing to delete, nothing to record).
	res, err := tr.ExecContext(ctx, `
		INSERT INTO mark_tombstones (mark_id, deleted_at)
		SELECT mark_id, NOW() FROM marks WHERE mark_id = $1
		ON CONFLICT (mark_id) DO UPDATE SET deleted_at = EXCLUDED.deleted_at`, markId)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	for _, query := range []string{
		"DELETE FROM checks WHERE mark_id = $1",
		"DELETE FROM tasks WHERE mark_id = $1",
		"DELETE FROM mark_status_history WHERE mark_id = $1",
		"DELETE FROM mark_followers WHERE mark_id = $1",
		"DELETE FROM marks WHERE mark_id = $1",
	} {
		if _, err := tr.ExecContext(ctx, query, markId); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

// GetDeletedMarkIDs returns the ids of marks deleted strictly after since
// (from mark_tombstones), oldest deletion first, as a page with the total.
func (r *MarksRepository) GetDeletedMarkIDs(ctx context.Context, since time.Time, p models.Pagination) (models.Page[int], error) {
	const op = "storage.postgres.GetDeletedMarkIDs"

	page := models.Page[int]{Items: []int{}}
	q := newListQuery("mark_id", "mark_tombstones").
		Where("deleted_at > ?", since).
		OrderBy("deleted_at ASC, mark_id ASC").
		Paginate(p)
	if err := r.readIDPage(ctx, op, q, p, &page); err != nil {
		return page, err
	}

	return page, nil
}

// GetHiddenMarkIDs returns the ids of the hidden marks changed strictly
// after since (hiding bumps updated_at), oldest change first, as a page
// with the total. Unlike the lists it is not filtered by the viewer: a
// client uses it to drop its copies, whoever it is.
func (r *MarksRepository) GetHiddenMarkIDs(ctx context.Context, since time.Time, p models.Pagination) (models.Page[int], error) {
	const op = "storage.postgres.GetHiddenMarkIDs"

	page := models.Page[int]{Items: []int{}}
	q := newListQuery("mark_id", "marks").
		Where("hidden").
		Where("updated_at > ?", since).
		OrderBy("updated_at ASC, mark_id ASC").
		Paginate(p)
	if err := r.readIDPage(ctx, op, q, p, &page); err != nil {
		return page, err
	}

	return page, nil
}

// readIDPage runs an id-only list query into page (items and total; the
// total of an empty page beyond the first comes from a count query).
func (r *MarksRepository) readIDPage(ctx context.Context, op string, q *listQuery, p models.Pagination, page *models.Page[int]) error {
	query, args, err := q.pageQuery()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	rows, err := tr.QueryxContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id, &page.Total); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		page.Items = append(page.Items, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	// An empty page beyond the first carries no row to read the total from.
	if len(page.Items) == 0 && p.Limit > 0 && p.Offset > 0 {
		countQuery, countArgs, err := q.countQuery()
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		if err := tr.GetContext(ctx, &page.Total, countQuery, countArgs...); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
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
	visibleMarks(ctx, q)

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

// markTypeColumns are the columns of a mark type followed by the localised
// name (see translatedName).
const markTypeColumns = `t.type_mark_id, t.code, t.sla_hours, t.icon, t.color, t.active, t.sort_order, `

// markTypeEntity is the translations.entity of mark types.
const markTypeEntity = "mark_type"

// markTypeNames are the stored translations selected for the admin
// endpoints, in addition to markTypeColumns.
const markTypeNames = `,
		COALESCE((SELECT name FROM translations WHERE entity = '` + markTypeEntity + `' AND entity_id = t.type_mark_id AND lang = 'ru'), t.name) AS name_ru,
		COALESCE((SELECT name FROM translations WHERE entity = '` + markTypeEntity + `' AND entity_id = t.type_mark_id AND lang = 'en'), '') AS name_en`

// GetMarkTypes lists the active mark types with names in lang (falling back
// to the default language, then to the raw name), sorted by sort_order and
// then by the localised name.
func (r *MarksRepository) GetMarkTypes(ctx context.Context, lang models.Lang) ([]models.MarkType, error) {
	const op = "storage.postgres.GetMarkTypes"

	return r.listMarkTypes(ctx, op, lang, "WHERE t.active", "")
}

// GetAllMarkTypes lists every mark type, inactive ones included, with both
// stored names (admin dictionary), sorted like GetMarkTypes.
func (r *MarksRepository) GetAllMarkTypes(ctx context.Context, lang models.Lang) ([]models.MarkType, error) {
	const op = "storage.postgres.GetAllMarkTypes"

	return r.listMarkTypes(ctx, op, lang, "", markTypeNames)
}

func (r *MarksRepository) listMarkTypes(ctx context.Context, op string, lang models.Lang, where, extraColumns string) ([]models.MarkType, error) {
	types := []models.MarkType{}

	query := `SELECT ` + markTypeColumns + translatedName("t.name") + extraColumns + `
		FROM types_marks t
		` + translationJoins(markTypeEntity, "t.type_mark_id") + `
		` + where + `
		ORDER BY t.sort_order, name, t.type_mark_id`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &types, query, lang, models.DefaultLang); err != nil {
		return types, fmt.Errorf("%s: %w", op, err)
	}

	return types, nil
}

// GetMarkTypeById returns one mark type (active or not) with its name in
// lang and both stored names (admin endpoints).
func (r *MarksRepository) GetMarkTypeById(ctx context.Context, id int, lang models.Lang) (models.MarkType, error) {
	const op = "storage.postgres.GetMarkTypeById"

	var t models.MarkType
	query := `SELECT ` + markTypeColumns + translatedName("t.name") + markTypeNames + `
		FROM types_marks t
		` + translationJoins(markTypeEntity, "t.type_mark_id") + `
		WHERE t.type_mark_id = $3`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &t, query, lang, models.DefaultLang, id); err != nil {
		return t, wrapPgError(op, err)
	}

	return t, nil
}

// AddMarkType inserts a mark type with its Russian and (optional) English
// names; repository.ErrExists when the code is taken.
func (r *MarksRepository) AddMarkType(ctx context.Context, t models.MarkTypeCreate) (int64, error) {
	const op = "storage.postgres.AddMarkType"

	var id int64
	query := `INSERT INTO types_marks (name, code, sla_hours, icon, color) VALUES ($1, $2, $3, $4, $5)
		RETURNING type_mark_id`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, t.NameRU, t.Code, t.SLAHours, t.Icon, t.Color); err != nil {
		return 0, wrapPgError(op, err)
	}

	if err := r.SetTranslation(ctx, markTypeEntity, int(id), models.LangRU, t.NameRU); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	if t.NameEN != "" {
		if err := r.SetTranslation(ctx, markTypeEntity, int(id), models.LangEN, t.NameEN); err != nil {
			return 0, fmt.Errorf("%s: %w", op, err)
		}
	}

	return id, nil
}

// UpdateMarkType applies the non-nil fields of upd to the mark type
// (repository.ErrNotFound when it does not exist, repository.ErrExists when
// the new code is taken). An empty Icon/Color clears the column.
func (r *MarksRepository) UpdateMarkType(ctx context.Context, id int, upd models.MarkTypeUpdate) error {
	const op = "storage.postgres.UpdateMarkType"

	query := `
		UPDATE types_marks SET
			code = COALESCE($2, code),
			name = COALESCE($3, name),
			sla_hours = COALESCE($4, sla_hours),
			icon = CASE WHEN $5::text IS NULL THEN icon ELSE NULLIF($5, '') END,
			color = CASE WHEN $6::text IS NULL THEN color ELSE NULLIF($6, '') END,
			active = COALESCE($7, active),
			sort_order = COALESCE($8, sort_order)
		WHERE type_mark_id = $1`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, id, upd.Code, upd.NameRU, upd.SLAHours, upd.Icon, upd.Color, upd.Active, upd.SortOrder)
	if err != nil {
		return wrapPgError(op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%s: %w", op, repository.ErrNotFound)
	}

	if upd.NameRU != nil {
		if err := r.SetTranslation(ctx, markTypeEntity, id, models.LangRU, *upd.NameRU); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}
	if upd.NameEN != nil {
		if err := r.SetTranslation(ctx, markTypeEntity, id, models.LangEN, *upd.NameEN); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

// GetMarkStatuses lists the mark statuses with names in lang (falling back
// to the default language, then to the raw name).
func (r *MarksRepository) GetMarkStatuses(ctx context.Context, lang models.Lang) ([]models.MarkStatus, error) {
	const op = "storage.postgres.GetMarkStatuses"

	statuses := []models.MarkStatus{}

	query := `SELECT s.mark_status_id, s.parent_id, s.code, ` + translatedName("s.name") + `
		FROM mark_statuses s
		` + translationJoins("mark_status", "s.mark_status_id") + `
		ORDER BY s.mark_status_id`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &statuses, query, lang, models.DefaultLang); err != nil {
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
		return wrapPgError(op, err)
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
		return historyItem, wrapPgError(op, err)
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
		return historyItem, wrapPgError(op, err)
	}

	return historyItem, nil
}

func (r *MarksRepository) GetDistancesFromMarkToPoint(ctx context.Context, filters models.GetDistanceFromMarkToPointFilters) ([]models.DistanceFromMarkToPoint, error) {
	const op = "storage.postgres.GetDistancesFromMarkToPoint"

	distances := []models.DistanceFromMarkToPoint{}

	// ST_DWithin on geography lets the planner use the GiST indexes on
	// marks.geom and users.home_point instead of a full cross join. The
	// rows are unordered and unrounded: the only consumer (the tasker) keys
	// them by (user, mark) and feeds the distance into a formula.
	query := `
		SELECT
			m.mark_id,
			u.user_id,
			ST_DistanceSphere(m.geom, u.home_point) / 1000.0 AS distance_km
		FROM
			marks m
		JOIN
			users u ON ST_DWithin(m.geom::geography, u.home_point::geography, $2)
		WHERE
			m.mark_status_id = ANY($1) AND NOT m.hidden
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &distances, query, pq.Array(filters.MarkStatusIds), filters.MaxRadius); err != nil {
		return distances, fmt.Errorf("%s: %w", op, err)
	}

	return distances, nil
}

// SetMarkHidden shows or hides the mark (repository.ErrNotFound when it
// does not exist).
func (r *MarksRepository) SetMarkHidden(ctx context.Context, markId int, hidden bool) error {
	const op = "storage.postgres.SetMarkHidden"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, "UPDATE marks SET hidden = $2, updated_at = NOW() WHERE mark_id = $1", markId, hidden)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// MergeMark marks the mark as a duplicate of target: the status becomes
// DuplicateStatus (the trigger records it in the history) and
// merged_into_id points at the target.
func (r *MarksRepository) MergeMark(ctx context.Context, markId, targetId int) error {
	const op = "storage.postgres.MergeMark"

	query := "UPDATE marks SET mark_status_id = $3, merged_into_id = $2, updated_at = NOW() WHERE mark_id = $1"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, markId, targetId, models.DuplicateStatus)
	if err != nil {
		return wrapPgError(op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// MoveFollowers moves the followers of the mark to target: a user already
// following the target keeps a single subscription. Call it inside a
// transaction.
func (r *MarksRepository) MoveFollowers(ctx context.Context, markId, targetId int) error {
	const op = "storage.postgres.MoveFollowers"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, `
		INSERT INTO mark_followers (user_id, mark_id, created_at)
		SELECT user_id, $2, created_at FROM mark_followers WHERE mark_id = $1
		ON CONFLICT DO NOTHING`, markId, targetId); err != nil {
		return wrapPgError(op, err)
	}
	if _, err := tr.ExecContext(ctx, "DELETE FROM mark_followers WHERE mark_id = $1", markId); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// GetMarkBriefs returns the short form of the marks with the given ids
// (missing ids are absent from the map). It is unfiltered by visibility:
// only moderators read it (the moderation queue).
func (r *MarksRepository) GetMarkBriefs(ctx context.Context, ids []int) (map[int]models.MarkBrief, error) {
	const op = "storage.postgres.GetMarkBriefs"

	briefs := map[int]models.MarkBrief{}
	if len(ids) == 0 {
		return briefs, nil
	}

	query, args, err := bind("SELECT "+markBriefColumns+" FROM marks WHERE marks.mark_id IN (?)", []any{ids})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var rows []models.MarkBrief
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	for _, m := range rows {
		briefs[m.ID] = m
	}

	return briefs, nil
}
