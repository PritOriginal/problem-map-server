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

type CommentsRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewComments(db *sqlx.DB, c *trmsqlx.CtxGetter) *CommentsRepository {
	return &CommentsRepository{
		db:     db,
		getter: c,
	}
}

// commentColumns lists the comment columns as served to clients: a deleted
// comment keeps its row (replies point at it) but its body is blanked. The
// placeholder is the viewer id (see models.ViewerFromContext).
const (
	commentColumns = "c.comment_id, c.mark_id, c.user_id, u.name AS username, " +
		"CASE WHEN c.deleted_at IS NULL THEN c.body ELSE '' END AS body, " +
		"c.parent_id, c.deleted_at IS NOT NULL AS deleted, c.user_id = ? AS is_mine, " +
		"c.created_at, c.updated_at"
	commentsFrom = "mark_comments AS c JOIN users AS u ON c.user_id = u.user_id"
)

// AddComment inserts the comment and returns its id. A missing mark, user
// or parent is repository.ErrInvalidReference.
func (r *CommentsRepository) AddComment(ctx context.Context, comment models.Comment) (int64, error) {
	const op = "storage.postgres.AddComment"

	var id int64

	query := `
		INSERT INTO mark_comments (mark_id, user_id, body, parent_id)
		VALUES ($1, $2, $3, $4)
		RETURNING comment_id
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, comment.MarkID, comment.UserID, comment.Body, comment.ParentID); err != nil {
		return 0, wrapPgError(op, err)
	}

	return id, nil
}

// GetCommentById returns the comment (deleted ones included, with an
// empty body) or repository.ErrNotFound.
func (r *CommentsRepository) GetCommentById(ctx context.Context, id int) (models.Comment, error) {
	const op = "storage.postgres.GetCommentById"

	var comment models.Comment

	query, args, err := bind("SELECT "+commentColumns+" FROM "+commentsFrom+" WHERE c.comment_id = ?",
		[]any{models.ViewerFromContext(ctx), id})
	if err != nil {
		return comment, fmt.Errorf("%s: %w", op, err)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &comment, query, args...); err != nil {
		return comment, wrapPgError(op, err)
	}

	return comment, nil
}

// GetCommentsByMarkId returns a page of the mark's comments, oldest first,
// deleted ones included (Deleted set, empty body) so replies keep their
// parent.
func (r *CommentsRepository) GetCommentsByMarkId(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Comment], error) {
	const op = "storage.postgres.GetCommentsByMarkId"

	q := newListQuery(commentColumns, commentsFrom).
		Where("c.mark_id = ?", markId).
		OrderBy("c.created_at ASC, c.comment_id ASC").
		Paginate(p)
	q.ColumnArgs(models.ViewerFromContext(ctx))

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Comment](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// UpdateCommentBody replaces the body of a live comment and bumps
// updated_at; a missing or deleted comment is repository.ErrNotFound.
func (r *CommentsRepository) UpdateCommentBody(ctx context.Context, id int, body string) error {
	const op = "storage.postgres.UpdateCommentBody"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx,
		"UPDATE mark_comments SET body = $2, updated_at = NOW() WHERE comment_id = $1 AND deleted_at IS NULL", id, body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// SoftDeleteComment marks the comment deleted; a missing or already
// deleted comment is repository.ErrNotFound.
func (r *CommentsRepository) SoftDeleteComment(ctx context.Context, id int) error {
	const op = "storage.postgres.SoftDeleteComment"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx,
		"UPDATE mark_comments SET deleted_at = NOW(), updated_at = NOW() WHERE comment_id = $1 AND deleted_at IS NULL", id)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// CountCommentsByUserIdSince counts the comments (deleted ones included)
// the user posted after since; used by the daily limit.
func (r *CommentsRepository) CountCommentsByUserIdSince(ctx context.Context, userId int, since time.Time) (int, error) {
	const op = "storage.postgres.CountCommentsByUserIdSince"

	var n int

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &n,
		"SELECT COUNT(*) FROM mark_comments WHERE user_id = $1 AND created_at >= $2", userId, since); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return n, nil
}

// HasRecentDuplicate reports whether the user posted the same body on the
// mark after since (deleted comments count too).
func (r *CommentsRepository) HasRecentDuplicate(ctx context.Context, userId, markId int, body string, since time.Time) (bool, error) {
	const op = "storage.postgres.HasRecentDuplicate"

	var exists bool

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &exists,
		"SELECT EXISTS(SELECT 1 FROM mark_comments WHERE user_id = $1 AND mark_id = $2 AND body = $3 AND created_at >= $4)",
		userId, markId, body, since); err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	return exists, nil
}
