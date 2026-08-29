package tasksrest

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

type Tasks interface {
	GetTasks(ctx context.Context, filters models.GetTasksFilters) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) ([]models.Task, error)
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

// GetTasks lists all existing tasks
//
//	@Summary		List tasks
//	@Description	get tasks
//	@Tags			tasks
//	@Produce		json
//	@Success		200	{object}	responses.Response[tasksrest.GetTasksResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/tasks [get]
func (h *handler) GetTasks() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "tasksrest.GetTasks"

		statuses, err := handlers.QueryIntArray(c, "statuses")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		tasks, err := h.uc.GetTasks(c.Request.Context(), models.GetTasksFilters{
			Statuses: statuses,
		})
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetTasksResponse{
			Tasks: tasks,
		})
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
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.Response[tasksrest.GetTasksByUserIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/tasks/user/{id} [get]
func (h *handler) GetTasksByUserId() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "tasksrest.GetTasksByUserId"

		userId, err := handlers.ParamInt(c, "id")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		statuses, err := handlers.QueryIntArray(c, "statuses")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		tasks, err := h.uc.GetTasksByUserId(c.Request.Context(), userId, models.GetTasksByUserIdFilters{
			Statuses: statuses,
		})
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetTasksByUserIdResponse{
			Tasks: tasks,
		})
	}
}

// AddTask add new task
//
//	@Summary		Add task
//	@Description	add new task on behalf of the authenticated user (moderator or admin)
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

		userId, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		task := models.Task{
			Name:   req.Name,
			UserID: userId,
			MarkID: req.MarkID,
		}

		taskId, err := h.uc.AddTask(c.Request.Context(), task)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		h.log.Info("add new task",
			slog.Int("user_id", userId),
			slog.Int("mark_id", req.MarkID),
		)
		responses.Created(c, AddTaskResponse{
			TaskId: int(taskId),
		})
	}
}
