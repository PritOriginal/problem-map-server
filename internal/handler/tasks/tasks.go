package tasksrest

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

type Tasks interface {
	GetTasks(ctx context.Context) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
}

type handler struct {
	log *slog.Logger
	uc  Tasks
}

func Register(r *gin.Engine, log *slog.Logger, uc Tasks) {
	handler := &handler{log: log, uc: uc}

	tasks := r.Group("/tasks")
	{
		tasks.GET("", handler.GetTasks())
		tasks.GET(":id", handler.GetTaskById())
		tasks.GET("user/:id", handler.GetTasksByUserId())
		tasks.POST("", handler.AddTask())
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
		tasks, err := h.uc.GetTasks(c.Request.Context())
		if err != nil {
			h.log.Error("error get tasks", logger.Err(err))
			responses.Internal(c, "error get tasks")
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
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		task, err := h.uc.GetTaskById(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
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
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.Response[tasksrest.GetTasksByUserIdResponse]
//	@Failure		400	{object}	responses.Response[any]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/tasks/user/{id} [get]
func (h *handler) GetTasksByUserId() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			h.log.Debug("failed parse id", logger.Err(err))
			responses.BadRequest(c, "failed parse id")
			return
		}

		tasks, err := h.uc.GetTasksByUserId(c.Request.Context(), userId)
		if err != nil {
			h.log.Error("error get tasks by user id", slog.Int("user_id", userId), logger.Err(err))
			responses.Internal(c, "error get tasks by user id")
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
//	@Description	add new task
//	@Tags			tasks
//	@Produce		json
//	@Param			request	body		tasksrest.AddTaskRequest	true	"query params"
//	@Success		201		{object}	responses.Response[tasksrest.AddTaskResponse]
//	@Failure		400		{object}	responses.Response[any]
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

		task := models.Task{
			Name:   req.Name,
			UserID: req.UserID,
			MarkID: req.MarkID,
		}

		taskId, err := h.uc.AddTask(c.Request.Context(), task)
		if err != nil {
			h.log.Error("failed add task", logger.Err(err))
			responses.Internal(c, "failed add task")
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
