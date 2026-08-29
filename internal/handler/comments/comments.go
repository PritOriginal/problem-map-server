// Package commentsrest serves the comments on marks: the thread of a mark
// under /marks/{id}/comments and the edit/delete of one comment under
// /comments/{id}.
package commentsrest

import (
	"context"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/guregu/null/v6"
)

type Comments interface {
	ListComments(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Comment], error)
	AddComment(ctx context.Context, comment models.Comment) (models.Comment, error)
	UpdateComment(ctx context.Context, actor models.Actor, id int, body string) (models.Comment, error)
	DeleteComment(ctx context.Context, actor models.Actor, id int) error
}

type handler struct {
	log *slog.Logger
	uc  Comments
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Comments) {
	handler := &handler{log: log, uc: uc}

	// The viewer is recorded so that is_mine is filled in for authenticated
	// readers; anonymous requests still pass.
	markComments := r.Group("/marks/:id/comments", middleware.OptionalAuth(authMiddleware))
	{
		markComments.GET("", handler.GetComments())
		markComments.POST("", authMiddleware.MiddlewareFunc(), handler.AddComment())
	}

	comments := r.Group("/comments/:id", authMiddleware.MiddlewareFunc())
	{
		comments.PATCH("", handler.UpdateComment())
		comments.DELETE("", handler.DeleteComment())
	}
}

// actorFromClaims builds the acting user from the validated JWT; it writes a
// 401 and returns false when the token carries no usable subject.
func (h *handler) actorFromClaims(c *gin.Context) (models.Actor, bool) {
	userId, err := middleware.UserIDFromClaims(c)
	if err != nil {
		h.log.Debug("invalid token", logger.Err(err))
		responses.Unauthorized(c, "invalid token")
		return models.Actor{}, false
	}
	return models.Actor{UserID: userId, Role: middleware.RoleFromClaims(c)}, true
}

// viewerContext returns the request context with the authenticated user as
// the viewer, so comments returned from mutations carry is_mine.
func viewerContext(c *gin.Context, userId int) context.Context {
	return models.ContextWithViewer(c.Request.Context(), userId)
}

// GetComments lists the comments of a mark
//
//	@Summary		List mark comments
//	@Description	get the comments of the mark, oldest first; pagination info is in the top-level `meta` field. Deleted comments are returned with `deleted=true` and an empty `body` so that replies keep their `parent_id`. With a Bearer token `is_mine` marks the caller's own comments
//	@Tags			comments
//	@Produce		json
//	@Param			id		path		int	true	"mark id"
//	@Param			limit	query		int	false	"page size, 1..500"	default(100)
//	@Param			offset	query		int	false	"page offset"		default(0)
//	@Success		200		{object}	responses.Response[commentsrest.GetCommentsResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/marks/{id}/comments [get]
func (h *handler) GetComments() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "commentsrest.GetComments"

		markId, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		p, ok := listquery.BindPagination(c, h.log)
		if !ok {
			return
		}

		page, err := h.uc.ListComments(c.Request.Context(), markId, p)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetCommentsResponse{Comments: page.Items}, p, page.Total)
	}
}

// AddComment posts a comment on a mark
//
//	@Summary		Add comment
//	@Description	post a comment on the mark; `parent_id` makes it a reply to a top-level comment of the same mark (one level of replies). The body is trimmed and must not be empty. 409 for a duplicate (the same body on the same mark within a minute) or a reply to a deleted comment, 429 when the daily limit (`comments.max-per-day`) is reached
//	@Tags			comments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int								true	"mark id"
//	@Param			request	body		commentsrest.AddCommentRequest	true	"comment"
//	@Success		201		{object}	responses.Response[commentsrest.CommentResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		429		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/marks/{id}/comments [post]
func (h *handler) AddComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "commentsrest.AddComment"

		markId, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		var req AddCommentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed parse body", logger.Err(err))
			responses.BadRequest(c, "invalid body")
			return
		}

		comment := models.Comment{
			MarkID: markId,
			UserID: actor.UserID,
			Body:   req.Body,
		}
		if req.ParentID != nil {
			comment.ParentID = null.IntFrom(int64(*req.ParentID))
		}

		created, err := h.uc.AddComment(viewerContext(c, actor.UserID), comment)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.Created(c, CommentResponse{Comment: created})
	}
}

// UpdateComment edits the caller's own comment
//
//	@Summary		Edit comment
//	@Description	replace the body of the caller's own comment within `comments.edit-window` (15 minutes by default) after its creation. 403 for another user's comment, 409 for a deleted comment or an expired window
//	@Tags			comments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int									true	"comment id"
//	@Param			request	body		commentsrest.UpdateCommentRequest	true	"new body"
//	@Success		200		{object}	responses.Response[commentsrest.CommentResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/comments/{id} [patch]
func (h *handler) UpdateComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "commentsrest.UpdateComment"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		var req UpdateCommentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed parse body", logger.Err(err))
			responses.BadRequest(c, "invalid body")
			return
		}

		comment, err := h.uc.UpdateComment(viewerContext(c, actor.UserID), actor, id, req.Body)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, CommentResponse{Comment: comment})
	}
}

// DeleteComment removes a comment
//
//	@Summary		Delete comment
//	@Description	soft-delete the comment: it stays in the thread with `deleted=true` and an empty body. The owner and moderators may delete; 409 when it is already deleted
//	@Tags			comments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"comment id"
//	@Success		200	{object}	responses.Response[commentsrest.DeleteCommentResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		409	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/comments/{id} [delete]
func (h *handler) DeleteComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "commentsrest.DeleteComment"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		if err := h.uc.DeleteComment(c.Request.Context(), actor, id); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, DeleteCommentResponse{CommentId: id})
	}
}
