package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

// commentsPerDayWindow is the rolling window of the daily comment limit.
const commentsPerDayWindow = 24 * time.Hour

// commentDuplicateWindow is how long the same body from the same user on
// the same mark is rejected as a duplicate.
const commentDuplicateWindow = time.Minute

type CommentsRepository interface {
	AddComment(ctx context.Context, comment models.Comment) (int64, error)
	GetCommentById(ctx context.Context, id int) (models.Comment, error)
	GetCommentsByMarkId(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Comment], error)
	UpdateCommentBody(ctx context.Context, id int, body string) error
	SoftDeleteComment(ctx context.Context, id int) error
	CountCommentsByUserIdSince(ctx context.Context, userId int, since time.Time) (int, error)
	HasRecentDuplicate(ctx context.Context, userId, markId int, body string, since time.Time) (bool, error)
}

// CommentsMarksRepository resolves the mark a comment belongs to.
type CommentsMarksRepository interface {
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
}

type CommentsRepositories struct {
	Comments CommentsRepository
	Marks    CommentsMarksRepository
}

// Comments manages the comments on marks.
type Comments struct {
	log    *slog.Logger
	cfg    config.CommentsConfig
	repos  CommentsRepositories
	events events.Publisher
	now    func() time.Time
}

func NewComments(log *slog.Logger, cfg config.CommentsConfig, repos CommentsRepositories) *Comments {
	return &Comments{
		log:    log,
		cfg:    cfg,
		repos:  repos,
		events: events.NoopPublisher{},
		now:    time.Now,
	}
}

// WithEvents sets the publisher of comment.added events. Without it events
// are dropped.
func (uc *Comments) WithEvents(p events.Publisher) *Comments {
	if p != nil {
		uc.events = p
	}
	return uc
}

// ListComments returns a page of the mark's comments, oldest first; the
// mark must exist. Deleted comments are included with Deleted set and an
// empty body so that replies keep their parent.
func (uc *Comments) ListComments(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Comment], error) {
	const op = "usecase.Comments.ListComments"

	if err := p.Validate(); err != nil {
		return models.Page[models.Comment]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	if _, err := uc.repos.Marks.GetMarkById(ctx, markId); err != nil {
		return models.Page[models.Comment]{}, mapRepoErr(op, err)
	}

	page, err := uc.repos.Comments.GetCommentsByMarkId(ctx, markId, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}
	return page, nil
}

// AddComment posts a comment (or a reply, when comment.ParentID is set) on
// the mark and publishes comment.added. The body is trimmed and must be
// non-empty and at most models.MaxCommentBodyLen runes (ErrInvalidArgument);
// a reply must point at a live top-level comment of the same mark
// (ErrInvalidArgument for a wrong parent, ErrConflict for a deleted one).
// A user may post at most cfg.MaxPerDay comments per rolling 24 hours
// (ErrTooManyRequests) and not the same body on the same mark twice within
// a minute (ErrConflict).
func (uc *Comments) AddComment(ctx context.Context, comment models.Comment) (models.Comment, error) {
	const op = "usecase.Comments.AddComment"

	body, err := normalizeCommentBody(comment.Body)
	if err != nil {
		return models.Comment{}, fmt.Errorf("%s: %w", op, err)
	}
	comment.Body = body

	mark, err := uc.repos.Marks.GetMarkById(ctx, comment.MarkID)
	if err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}

	if comment.ParentID.Valid {
		parent, err := uc.repos.Comments.GetCommentById(ctx, int(comment.ParentID.Int64))
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return models.Comment{}, fmt.Errorf("%s: %w: parent comment not found", op, ErrInvalidArgument)
		case err != nil:
			return models.Comment{}, mapRepoErr(op, err)
		case parent.MarkID != comment.MarkID:
			return models.Comment{}, fmt.Errorf("%s: %w: parent comment belongs to another mark", op, ErrInvalidArgument)
		case parent.ParentID.Valid:
			return models.Comment{}, fmt.Errorf("%s: %w: replies to replies are not allowed", op, ErrInvalidArgument)
		case parent.Deleted:
			return models.Comment{}, fmt.Errorf("%s: %w: parent comment is deleted", op, ErrConflict)
		}
	}

	now := uc.now()
	n, err := uc.repos.Comments.CountCommentsByUserIdSince(ctx, comment.UserID, now.Add(-commentsPerDayWindow))
	if err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}
	if n >= uc.cfg.MaxPerDay {
		return models.Comment{}, fmt.Errorf("%s: %w: %d comments per day", op, ErrTooManyRequests, uc.cfg.MaxPerDay)
	}

	duplicate, err := uc.repos.Comments.HasRecentDuplicate(ctx, comment.UserID, comment.MarkID, comment.Body, now.Add(-commentDuplicateWindow))
	if err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}
	if duplicate {
		return models.Comment{}, fmt.Errorf("%s: %w: duplicate comment", op, ErrConflict)
	}

	id, err := uc.repos.Comments.AddComment(ctx, comment)
	if err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}

	created, err := uc.repos.Comments.GetCommentById(ctx, int(id))
	if err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}
	uc.log.Info("comment added", slog.Int("comment_id", created.ID), slog.Int("mark_id", created.MarkID), slog.Int("user_id", created.UserID))

	var parentID *int
	if comment.ParentID.Valid {
		id := int(comment.ParentID.Int64)
		parentID = &id
	}
	events.PublishEvent(ctx, uc.log, uc.events, events.NewCommentAdded(created.ID, created.MarkID, created.UserID, parentID, mark.UserID))

	return created, nil
}

// UpdateComment replaces the body of the actor's own comment within
// cfg.EditWindow after its creation: ErrForbidden for another user's
// comment (moderators included), ErrConflict for a deleted comment or an
// expired window.
func (uc *Comments) UpdateComment(ctx context.Context, actor models.Actor, id int, body string) (models.Comment, error) {
	const op = "usecase.Comments.UpdateComment"

	body, err := normalizeCommentBody(body)
	if err != nil {
		return models.Comment{}, fmt.Errorf("%s: %w", op, err)
	}

	comment, err := uc.repos.Comments.GetCommentById(ctx, id)
	if err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}
	if comment.UserID != actor.UserID {
		return models.Comment{}, fmt.Errorf("%s: %w", op, ErrForbidden)
	}
	if comment.Deleted {
		return models.Comment{}, fmt.Errorf("%s: %w: comment is deleted", op, ErrConflict)
	}
	if uc.now().Sub(comment.CreatedAt) > uc.cfg.EditWindow {
		return models.Comment{}, fmt.Errorf("%s: %w: edit window of %s expired", op, ErrConflict, uc.cfg.EditWindow)
	}

	if err := uc.repos.Comments.UpdateCommentBody(ctx, id, body); err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}

	comment, err = uc.repos.Comments.GetCommentById(ctx, id)
	if err != nil {
		return models.Comment{}, mapRepoErr(op, err)
	}
	return comment, nil
}

// DeleteComment soft-deletes the comment: the owner and moderators may
// delete it (ErrForbidden otherwise), an already deleted comment is
// ErrConflict.
func (uc *Comments) DeleteComment(ctx context.Context, actor models.Actor, id int) error {
	const op = "usecase.Comments.DeleteComment"

	comment, err := uc.repos.Comments.GetCommentById(ctx, id)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if comment.UserID != actor.UserID && !actor.IsModerator() {
		return fmt.Errorf("%s: %w", op, ErrForbidden)
	}
	if comment.Deleted {
		return fmt.Errorf("%s: %w: comment is already deleted", op, ErrConflict)
	}

	if err := uc.repos.Comments.SoftDeleteComment(ctx, id); err != nil {
		return mapRepoErr(op, err)
	}
	uc.log.Info("comment deleted", slog.Int("comment_id", id), slog.Int("mark_id", comment.MarkID), slog.Int("user_id", actor.UserID))

	return nil
}

// normalizeCommentBody trims the body and checks that it is non-empty,
// valid UTF-8 without control characters (line breaks and tabs are kept;
// a NUL byte would be rejected by PostgreSQL anyway) and within
// models.MaxCommentBodyLen runes (ErrInvalidArgument otherwise).
func normalizeCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("%w: empty comment", ErrInvalidArgument)
	}
	if !utf8.ValidString(body) {
		return "", fmt.Errorf("%w: comment is not valid UTF-8", ErrInvalidArgument)
	}
	if strings.ContainsFunc(body, isForbiddenCommentRune) {
		return "", fmt.Errorf("%w: comment contains control characters", ErrInvalidArgument)
	}
	if utf8.RuneCountInString(body) > models.MaxCommentBodyLen {
		return "", fmt.Errorf("%w: comment longer than %d characters", ErrInvalidArgument, models.MaxCommentBodyLen)
	}
	return body, nil
}

// isForbiddenCommentRune reports a control character other than a line
// break or a tab.
func isForbiddenCommentRune(r rune) bool {
	return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
}
