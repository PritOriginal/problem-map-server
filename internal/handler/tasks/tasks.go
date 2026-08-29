package tasksrest

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
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
		var req GetTasksRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			h.log.Debug("failed parse query params", logger.Err(err))
			responses.BadRequest(c, "invalid query params")
			return
		}
		statuses, err := handlers.ParseIntArray(req.Statuses)
		if err != nil {
			h.log.Debug("failed parse statuses", logger.Err(err))
			responses.BadRequest(c, "failed parse statuses")
			return
		}
		p := req.Model()

		page, err := h.uc.ListTasks(c.Request.Context(), models.GetTasksFilters{
			Statuses:   statuses,
			Pagination: p,
		})
		if err != nil {
			if errors.Is(err, usecase.ErrInvalidArgument) {
				h.log.Debug("invalid pagination", logger.Err(err))
				responses.BadRequest(c, "invalid query params")
				return
			}
			h.log.Error("error get tasks", logger.Err(err))
			responses.Internal(c, "error get tasks")
			return
		}

		responses.OKList(c, GetTasksResponse{
			Tasks: page.Items,
		}, listquery.Meta(p, page.Total))
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
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		task, err := h.uc.GetTaskById(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				h.log.Debug("task not found", slog.Int("id", id))
				responses.NotFound(c, "task not found")
			} else {
				h.log.Error("error get task by id", slog.Int("id", id), logger.Err(err))
				responses.Internal(c, "error get task by id")
			}
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
		userId, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		var req GetTasksRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			h.log.Debug("failed parse query params", logger.Err(err))
			responses.BadRequest(c, "invalid query params")
			return
		}
		statuses, err := handlers.ParseIntArray(req.Statuses)
		if err != nil {
			h.log.Debug("failed parse statuses", logger.Err(err))
			responses.BadRequest(c, "failed parse statuses")
			return
		}
		p := req.Model()

		page, err := h.uc.ListTasksByUserId(c.Request.Context(), userId, models.GetTasksByUserIdFilters{
			Statuses:   statuses,
			Pagination: p,
		})
		if err != nil {
			if errors.Is(err, usecase.ErrInvalidArgument) {
				h.log.Debug("invalid pagination", logger.Err(err))
				responses.BadRequest(c, "invalid query params")
				return
			}
			h.log.Error("error get tasks by user id", slog.Int("user_id", userId), logger.Err(err))
			responses.Internal(c, "error get tasks by user id")
			return
		}

		responses.OKList(c, GetTasksByUserIdResponse{
			Tasks: page.Items,
		}, listquery.Meta(p, page.Total))
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
			h.log.Error("failed add task", logger.Err(err))
			responses.Internal(c, "failed add task")
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
