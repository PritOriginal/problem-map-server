package tasksrest

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
)

type Tasks interface {
	ListTasks(ctx context.Context, filters models.GetTasksFilters) (models.Page[models.Task], error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	ListTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) (models.Page[models.Task], error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
}

type handler struct {
	log *slog.Logger
	uc  Tasks
}

func Register(r *gin.Engine, log *slog.Logger, authMiddleware *jwt.GinJWTMiddleware, uc Tasks) {
	handler := &handler{log: log, uc: uc}

	tasks := r.Group("/tasks")
	{
		tasks.GET("", handler.GetTasks())
		tasks.GET(":id", handler.GetTaskById())
		tasks.GET("user/:id", handler.GetTasksByUserId())
		auth := tasks.Group("", authMiddleware.MiddlewareFunc(),
			middleware.RequireRole(models.RoleModerator, models.RoleAdmin))
		{
			auth.POST("", handler.AddTask())
		}
	}
}

// GetTasks lists tasks, paginated
//
//	@Summary		List tasks
//	@Description	get tasks page; pagination info is returned in the top-level `meta` field ({limit, offset, total})
//	@Tags			tasks
//	@Produce		json
//	@Param			statuses	query		string	false	"filter by statuses, comma-separated ids"
//	@Param			limit		query		int		false	"page size, 1..500"	default(100)
//	@Param			offset		query		int		false	"page offset"		default(0)
//	@Success		200			{object}	responses.Response[tasksrest.GetTasksResponse]
//	@Failure		400			{object}	responses.Response[any]
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/tasks [get]
func (h *handler) GetTasks() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "tasksrest.GetTasks"

		var req GetTasksRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			h.log.Debug("failed parse filters", logger.Err(err))
			responses.BadRequest(c, err.Error())
			return
		}

		page, err := h.uc.ListTasks(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetTasksResponse{Tasks: page.Items}, filters.Pagination, page.Total)
	}
}

// GetTaskById get task by id
//
//	@Summary		Get task by id
//	@Description	get task by id
//	@Tags			tasks
//	@Produce		json
//	@Param			id	path		int	true	"task id"
//	@Success		200	{object}	responses.Response[tasksrest.GetTaskByIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		404	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/tasks/{id} [get]
func (h *handler) GetTaskById() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "tasksrest.GetTaskById"

		id, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		task, err := h.uc.GetTaskById(c.Request.Context(), id)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetTaskByIdResponse{
			Task: task,
		})
	}
}

// GetTasksByUserId get tasks by user id
//
//	@Summary		Get tasks by user id
//	@Description	get tasks by user id
//	@Tags			tasks
//	@Produce		json
//	@Param			id			path		int		true	"user id"
//	@Param			statuses	query		string	false	"filter by statuses, comma-separated ids"
//	@Param			limit		query		int		false	"page size, 1..500"	default(100)
//	@Param			offset		query		int		false	"page offset"		default(0)
//	@Success		200			{object}	responses.Response[tasksrest.GetTasksByUserIdResponse]
//	@Failure		400			{object}	responses.Response[any]
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/tasks/user/{id} [get]
func (h *handler) GetTasksByUserId() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "tasksrest.GetTasksByUserId"

		userId, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		var req GetTasksRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			h.log.Debug("failed parse filters", logger.Err(err))
			responses.BadRequest(c, err.Error())
			return
		}

		page, err := h.uc.ListTasksByUserId(c.Request.Context(), userId, models.GetTasksByUserIdFilters(filters))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		listquery.OK(c, GetTasksByUserIdResponse{Tasks: page.Items}, filters.Pagination, page.Total)
	}
}

// AddTask add new task
//
//	@Summary		Add task
//	@Description	assign a new verification task to the user given by user_id (moderator or admin only)
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		tasksrest.AddTaskRequest	true	"query params"
//	@Success		201		{object}	responses.Response[tasksrest.AddTaskResponse]
//	@Failure		400		{object}	responses.Response[any]
//	@Failure		401		{object}	responses.Response[any]
//	@Failure		403		{object}	responses.Response[any]
//	@Failure		500		{object}	responses.Response[any]
//	@Router			/tasks [post]
func (h *handler) AddTask() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "tasksrest.AddTask"

		var req AddTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.log.Debug("failed binding request", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			return
		}

		task := models.Task{
			Name:   req.Name,
			UserID: req.UserID,
			MarkID: req.MarkID,
		}

		taskId, err := h.uc.AddTask(c.Request.Context(), task)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("add new task",
			slog.Int("user_id", req.UserID),
			slog.Int("mark_id", req.MarkID),
		)
		responses.Created(c, AddTaskResponse{
			TaskId: int(taskId),
		})
	}
}
