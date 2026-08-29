package achievementsrest

import (
	"context"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Achievements interface {
	ListBadges(ctx context.Context) ([]models.Badge, error)
	GetProfile(ctx context.Context, userId int) (models.UserProfile, error)
}

type handler struct {
	log *slog.Logger
	uc  Achievements
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Achievements) {
	handler := &handler{log: log, uc: uc}

	r.GET("/badges", handler.GetBadges())

	users := r.Group("/users")
	{
		users.GET(":id/profile", handler.GetProfile())
		users.GET("me/profile", authMiddleware.MiddlewareFunc(), handler.GetMyProfile())
	}
}

// GetBadges returns the badge catalogue
//
//	@Summary		Badge catalogue
//	@Description	get every badge that can be earned; names and descriptions are localised by `Accept-Language`
//	@Tags			achievements
//	@Produce		json
//	@Param			Accept-Language	header		string	false	"ru (default) or en"
//	@Success		200				{object}	responses.Response[achievementsrest.GetBadgesResponse]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/badges [get]
func (h *handler) GetBadges() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "achievementsrest.GetBadges"

		badges, err := h.uc.ListBadges(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetBadgesResponse{Badges: badges})
	}
}

// GetProfile returns the gamification profile of a user
//
//	@Summary		Get user profile
//	@Description	get the public profile of a user: rating, level, earned badges and activity counters; level and badge texts are localised by `Accept-Language`
//	@Tags			achievements
//	@Produce		json
//	@Param			Accept-Language	header		string	false	"ru (default) or en"
//	@Param			id				path		int		true	"user id"
//	@Success		200				{object}	responses.Response[achievementsrest.GetProfileResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		404				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/users/{id}/profile [get]
func (h *handler) GetProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "achievementsrest.GetProfile"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.respondProfile(c, op, id)
	}
}

// GetMyProfile returns the gamification profile of the authenticated user
//
//	@Summary		Get current user profile
//	@Description	get the profile of the authenticated user: rating, level, earned badges and activity counters
//	@Tags			achievements
//	@Produce		json
//	@Security		BearerAuth
//	@Param			Accept-Language	header		string	false	"ru (default) or en"
//	@Success		200				{object}	responses.Response[achievementsrest.GetProfileResponse]
//	@Failure		401				{object}	responses.Response[any]
//	@Failure		404				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/users/me/profile [get]
func (h *handler) GetMyProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "achievementsrest.GetMyProfile"

		id, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		h.respondProfile(c, op, id)
	}
}

func (h *handler) respondProfile(c *gin.Context, op string, id int) {
	profile, err := h.uc.GetProfile(c.Request.Context(), id)
	if err != nil {
		responses.FromError(c, h.log, op, err)
		return
	}

	responses.OK(c, GetProfileResponse{Profile: NewProfile(profile)})
}
