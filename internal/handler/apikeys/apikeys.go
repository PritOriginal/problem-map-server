// Package apikeysrest manages the API keys of the open-data endpoints.
package apikeysrest

import (
	"context"
	"log/slog"
	"time"

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

type APIKeys interface {
	Create(ctx context.Context, actor models.Actor, name string, expiresAt null.Time) (models.APIKey, string, error)
	List(ctx context.Context, actor models.Actor, all bool) ([]models.APIKey, error)
	Revoke(ctx context.Context, actor models.Actor, id int) error
}

type handler struct {
	log *slog.Logger
	uc  APIKeys
}

// Register mounts /api-keys for authenticated users.
func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc APIKeys) {
	handler := &handler{log: log, uc: uc}

	keys := r.Group("/api-keys", authMiddleware.MiddlewareFunc())
	{
		keys.POST("", handler.CreateAPIKey())
		keys.GET("", handler.GetAPIKeys())
		keys.DELETE(":id", handler.DeleteAPIKey())
	}
}

// actor builds the acting user from the validated JWT; it writes a 401 and
// returns false when the token carries no usable subject.
func (h *handler) actor(c *gin.Context) (models.Actor, bool) {
	userId, err := middleware.UserIDFromClaims(c)
	if err != nil {
		h.log.Debug("invalid token", logger.Err(err))
		responses.Unauthorized(c, "invalid token")
		return models.Actor{}, false
	}
	return models.Actor{UserID: userId, Role: middleware.RoleFromClaims(c)}, true
}

// CreateAPIKey issues an API key
//
//	@Summary		Create API key
//	@Description	issue a read-only key `pm_live_…` for the open-data endpoints. The key is returned **once** in `payload.key`; only its hash is stored. Scope `read`, 600 requests per minute
//	@Tags			api-keys
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		apikeysrest.CreateAPIKeyRequest	true	"key"
//	@Success		201		{object}	responses.Response[apikeysrest.CreateAPIKeyResponse]
//	@Failure		400		{object}	responses.Response[any]	"empty name or expires_at not in the future"
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/api-keys [post]
func (h *handler) CreateAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "apikeysrest.CreateAPIKey"

		actor, ok := h.actor(c)
		if !ok {
			return
		}

		var req CreateAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}
		var expiresAt null.Time
		if req.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, req.ExpiresAt)
			if err != nil {
				responses.BadRequest(c, "expires_at must be RFC3339")
				return
			}
			expiresAt = null.TimeFrom(t)
		}

		key, raw, err := h.uc.Create(c.Request.Context(), actor, req.Name, expiresAt)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("api key created", slog.Int("api_key_id", key.ID), slog.Int("user_id", actor.UserID))
		responses.Created(c, CreateAPIKeyResponse{APIKey: key, Key: raw})
	}
}

// GetAPIKeys lists API keys
//
//	@Summary		List API keys
//	@Description	keys owned by the current user (hashes are never returned); an admin may pass `all=true` to list every key
//	@Tags			api-keys
//	@Produce		json
//	@Security		BearerAuth
//	@Param			all	query		bool	false	"list every key (admin only)"
//	@Success		200	{object}	responses.Response[apikeysrest.GetAPIKeysResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]	"all=true by a non-admin"
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/api-keys [get]
func (h *handler) GetAPIKeys() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "apikeysrest.GetAPIKeys"

		actor, ok := h.actor(c)
		if !ok {
			return
		}
		var q GetAPIKeysQuery
		if !listquery.Bind(c, h.log, &q) {
			return
		}

		keys, err := h.uc.List(c.Request.Context(), actor, q.All)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetAPIKeysResponse{APIKeys: keys})
	}
}

// DeleteAPIKey revokes an API key
//
//	@Summary		Revoke API key
//	@Description	deactivate the key; the owner or an admin. Requests with the key are answered 401 from now on (another instance may accept it for up to a minute from its cache)
//	@Tags			api-keys
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"api key id"
//	@Success		200	{object}	responses.Response[apikeysrest.DeleteAPIKeyResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/api-keys/{id} [delete]
func (h *handler) DeleteAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "apikeysrest.DeleteAPIKey"

		actor, ok := h.actor(c)
		if !ok {
			return
		}
		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		if err := h.uc.Revoke(c.Request.Context(), actor, id); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("api key revoked", slog.Int("api_key_id", id), slog.Int("user_id", actor.UserID))
		responses.OK(c, DeleteAPIKeyResponse{APIKeyId: id})
	}
}
