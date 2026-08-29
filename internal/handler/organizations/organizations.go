// Package organizationsrest exposes city services (organizations): their
// administration, the queue of assigned marks and the service actions on a
// mark (start / resolve / assign).
package organizationsrest

import (
	"context"
	"io"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

type Organizations interface {
	Create(ctx context.Context, org models.Organization) (models.Organization, error)
	Update(ctx context.Context, id int, upd models.OrganizationUpdate) (models.Organization, error)
	List(ctx context.Context) ([]models.OrganizationRef, error)
	Get(ctx context.Context, id int) (models.OrganizationDetails, error)
	GetMine(ctx context.Context, userId int) (models.OrganizationDetails, error)
	AddMember(ctx context.Context, orgId, userId int) error
	RemoveMember(ctx context.Context, orgId, userId int) error
	AddResponsibility(ctx context.Context, resp models.OrganizationResponsibility) (models.OrganizationResponsibility, error)
	RemoveResponsibility(ctx context.Context, resp models.OrganizationResponsibility) error
	ListMarks(ctx context.Context, actor models.Actor, orgId int, filters models.GetOrganizationMarksFilters) (models.Page[models.Mark], error)
	Start(ctx context.Context, actor models.Actor, markId int) (models.Mark, error)
	Resolve(ctx context.Context, actor models.Actor, markId int, comment string, photos []io.Reader) (models.Mark, error)
	Assign(ctx context.Context, markId, orgId int) (models.Mark, error)
}

type handler struct {
	log *slog.Logger
	uc  Organizations
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Organizations) {
	handler := &handler{log: log, uc: uc}

	orgs := r.Group("/organizations")
	{
		orgs.GET("", handler.List())

		service := orgs.Group("", authMiddleware.MiddlewareFunc(),
			middleware.RequireRole(models.RoleService, models.RoleAdmin))
		{
			service.GET("me", handler.GetMine())
			service.GET(":id/marks", handler.GetMarks())
		}

		admin := orgs.Group("", authMiddleware.MiddlewareFunc(), middleware.RequireRole(models.RoleAdmin))
		{
			admin.POST("", handler.Create())
			admin.GET(":id", handler.Get())
			admin.PATCH(":id", handler.Update())
			admin.POST(":id/members", handler.AddMember())
			admin.DELETE(":id/members/:userId", handler.RemoveMember())
			admin.POST(":id/responsibilities", handler.AddResponsibility())
			admin.DELETE(":id/responsibilities", handler.RemoveResponsibility())
		}
	}

	marks := r.Group("/marks/:id", authMiddleware.MiddlewareFunc())
	{
		service := marks.Group("", middleware.RequireRole(models.RoleService))
		{
			service.POST("start", handler.Start())
			service.POST("resolve", middleware.MaxBodySize(handlers.MaxUploadBodySize), handler.Resolve())
		}
		marks.PATCH("assign", middleware.RequireRole(models.RoleModerator, models.RoleAdmin), handler.Assign())
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

// List lists organizations
//
//	@Summary		List organizations
//	@Description	public list of city services (id and name)
//	@Tags			organizations
//	@Produce		json
//	@Success		200	{object}	responses.Response[organizationsrest.ListOrganizationsResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/organizations [get]
func (h *handler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.List"

		orgs, err := h.uc.List(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, ListOrganizationsResponse{Organizations: orgs})
	}
}

// Create creates an organization
//
//	@Summary		Create organization
//	@Description	create a city service (admin only)
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		organizationsrest.CreateOrganizationRequest	true	"organization"
//	@Success		201		{object}	responses.Response[organizationsrest.OrganizationResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/organizations [post]
func (h *handler) Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.Create"

		var req CreateOrganizationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		org, err := h.uc.Create(c.Request.Context(), req.Model())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("organization created", slog.Int("organization_id", org.ID))
		responses.Created(c, OrganizationResponse{Organization: org})
	}
}

// Update edits an organization
//
//	@Summary		Update organization
//	@Description	change `name` and/or `description` (admin only)
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int											true	"organization id"
//	@Param			request	body		organizationsrest.UpdateOrganizationRequest	true	"fields to change"
//	@Success		200		{object}	responses.Response[organizationsrest.OrganizationResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/organizations/{id} [patch]
func (h *handler) Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.Update"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req UpdateOrganizationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		org, err := h.uc.Update(c.Request.Context(), id, req.Model())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, OrganizationResponse{Organization: org})
	}
}

// Get returns an organization with members and responsibilities
//
//	@Summary		Get organization
//	@Description	organization with its members and responsibilities (admin only)
//	@Tags			organizations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"organization id"
//	@Success		200	{object}	responses.Response[organizationsrest.OrganizationDetailsResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/organizations/{id} [get]
func (h *handler) Get() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.Get"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		org, err := h.uc.Get(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, OrganizationDetailsResponse{Organization: org})
	}
}

// GetMine returns the caller's organization
//
//	@Summary		Get my organization
//	@Description	the organization the authenticated service user is a member of (404 when none)
//	@Tags			organizations
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	responses.Response[organizationsrest.OrganizationDetailsResponse]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/organizations/me [get]
func (h *handler) GetMine() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.GetMine"

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		org, err := h.uc.GetMine(c.Request.Context(), actor.UserID)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, OrganizationDetailsResponse{Organization: org})
	}
}

// AddMember adds a user to an organization
//
//	@Summary		Add organization member
//	@Description	add the user to the organization and give them the `service` role (admin only). Moderators/admins and users already in an organization are rejected with 409. The user's sessions are revoked so the role applies on the next sign-in
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int									true	"organization id"
//	@Param			request	body		organizationsrest.AddMemberRequest	true	"user"
//	@Success		201		{object}	responses.Response[organizationsrest.MemberResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/organizations/{id}/members [post]
func (h *handler) AddMember() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.AddMember"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req AddMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		if err := h.uc.AddMember(c.Request.Context(), id, req.UserID); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.Created(c, MemberResponse{OrganizationID: id, UserID: req.UserID, Member: true})
	}
}

// RemoveMember removes a user from an organization
//
//	@Summary		Remove organization member
//	@Description	remove the user from the organization and return them to the `user` role (admin only)
//	@Tags			organizations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int	true	"organization id"
//	@Param			userId	path		int	true	"user id"
//	@Success		200		{object}	responses.Response[organizationsrest.MemberResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/organizations/{id}/members/{userId} [delete]
func (h *handler) RemoveMember() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.RemoveMember"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}
		userId, err := handlers.ParamInt(c, "userId")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		if err := h.uc.RemoveMember(c.Request.Context(), id, userId); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, MemberResponse{OrganizationID: id, UserID: userId, Member: false})
	}
}

// AddResponsibility adds a (mark type, boundary) responsibility
//
//	@Summary		Add responsibility
//	@Description	make the organization responsible for marks of `mark_type_id` inside admin boundary `boundary_id` (admin only); a duplicate pair is 409
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int										true	"organization id"
//	@Param			request	body		organizationsrest.ResponsibilityRequest	true	"responsibility"
//	@Success		201		{object}	responses.Response[organizationsrest.ResponsibilityResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/organizations/{id}/responsibilities [post]
func (h *handler) AddResponsibility() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.AddResponsibility"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req ResponsibilityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		resp, err := h.uc.AddResponsibility(c.Request.Context(), req.Model(id))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.Created(c, ResponsibilityResponse{Responsibility: resp})
	}
}

// RemoveResponsibility removes a (mark type, boundary) responsibility
//
//	@Summary		Remove responsibility
//	@Description	remove the (mark type, boundary) pair from the organization (admin only)
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int										true	"organization id"
//	@Param			request	body		organizationsrest.ResponsibilityRequest	true	"responsibility"
//	@Success		200		{object}	responses.Response[organizationsrest.RemoveResponsibilityResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/organizations/{id}/responsibilities [delete]
func (h *handler) RemoveResponsibility() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.RemoveResponsibility"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req ResponsibilityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		if err := h.uc.RemoveResponsibility(c.Request.Context(), req.Model(id)); err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, RemoveResponsibilityResponse{OrganizationID: id, MarkTypeID: req.MarkTypeID, BoundaryID: req.BoundaryID})
	}
}

// GetMarks lists the organization's queue
//
//	@Summary		Organization queue
//	@Description	marks assigned to the organization, overdue first then by the nearest `sla_due_at`; members of the organization and admins only; pagination info is in the top-level `meta` field
//	@Tags			organizations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		int		true	"organization id"
//	@Param			status_ids	query		string	false	"filter by mark statuses, comma-separated ids"
//	@Param			overdue		query		boolean	false	"only marks past their SLA deadline"	default(false)
//	@Param			limit		query		int		false	"page size, 1..500"						default(100)
//	@Param			offset		query		int		false	"page offset"							default(0)
//	@Success		200			{object}	responses.Response[organizationsrest.GetOrganizationMarksResponse]
//	@Failure		400			{object}	responses.Response[any]
//	@Failure		401			{object}	responses.Response[any]
//	@Failure		403			{object}	responses.Response[any]
//	@Failure		404			{object}	responses.Response[any]
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/organizations/{id}/marks [get]
func (h *handler) GetMarks() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.GetMarks"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req GetOrganizationMarksRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			h.log.Debug("failed parse filters", logger.Err(err))
			responses.BadRequest(c, err.Error())
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		page, err := h.uc.ListMarks(models.ContextWithViewer(c.Request.Context(), actor.UserID), actor, id, filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetOrganizationMarksResponse{Marks: page.Items}, filters.Pagination, page.Total)
	}
}

// Start moves a mark to "in progress"
//
//	@Summary		Start work on a mark
//	@Description	Подтверждённая → В работе; only a member of the organization the mark is assigned to (403); a mark in another status is 409
//	@Tags			organizations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"mark id"
//	@Success		200	{object}	responses.Response[organizationsrest.MarkResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		401	{object}	responses.Response[any]
//	@Failure		403	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		409	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/marks/{id}/start [post]
func (h *handler) Start() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.Start"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		mark, err := h.uc.Start(models.ContextWithViewer(c.Request.Context(), actor.UserID), actor, id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, MarkResponse{Mark: mark})
	}
}

// Resolve reports a mark as fixed
//
//	@Summary		Resolve a mark
//	@Description	В работе → На проверке with a report (comment + photos) stored as a check of the service user; only a member of the organization the mark is assigned to (403); a mark in another status is 409
//	@Tags			organizations
//	@Accept			mpfd
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int		true	"mark id"
//	@Param			photos	formData	file	true	"Photos of the result"
//	@Param			comment	formData	string	false	"Report (max 1000 chars)"
//	@Success		200		{object}	responses.Response[organizationsrest.MarkResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/marks/{id}/resolve [post]
func (h *handler) Resolve() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.Resolve"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req ResolveMarkRequest
		if err := c.ShouldBind(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		photos, err := handlers.ParsePhotos(req.Photos)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		mark, err := h.uc.Resolve(models.ContextWithViewer(c.Request.Context(), actor.UserID), actor, id, req.Comment, photos)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, MarkResponse{Mark: mark})
	}
}

// Assign (re)assigns a mark to an organization
//
//	@Summary		Assign mark to organization
//	@Description	manual (re)assignment by a moderator/admin; the mark must be confirmed or in progress (409); the SLA deadline is reset
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int									true	"mark id"
//	@Param			request	body		organizationsrest.AssignMarkRequest	true	"organization"
//	@Success		200		{object}	responses.Response[organizationsrest.MarkResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		404		{object}	responses.Response[any]
//	@Failure		409		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/marks/{id}/assign [patch]
func (h *handler) Assign() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "organizationsrest.Assign"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req AssignMarkRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		actor, ok := h.actorFromClaims(c)
		if !ok {
			return
		}

		mark, err := h.uc.Assign(models.ContextWithViewer(c.Request.Context(), actor.UserID), id, req.OrganizationID)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("mark assigned manually", slog.Int("mark_id", id), slog.Int("organization_id", req.OrganizationID), slog.Int("user_id", actor.UserID))
		responses.OK(c, MarkResponse{Mark: mark})
	}
}
