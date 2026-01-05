package tasksrest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Tasks interface {
	GetTasks(ctx context.Context) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
}

type handler struct {
	*handlers.BaseHandler
	uc Tasks
}

func Register(r *chi.Mux, uc Tasks, bh *handlers.BaseHandler) {
	handler := &handler{BaseHandler: bh, uc: uc}

	r.Route("/tasks", func(r chi.Router) {
		r.Get("/", handler.GetTasks())
		r.Get("/{id}", handler.GetTaskById())
		r.Get("/user/{id}", handler.GetTasksByUserId())
		r.Post("/", handler.AddTask())
	})
}

// GetTasks lists all existing tasks
//
//	@Summary		List tasks
//	@Description	get tasks
//	@Tags			tasks
//	@Produce		json
//	@Success		200	{object}	responses.SucceededResponse[tasksrest.GetTasksResponse]
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/tasks [get]
func (h *handler) GetTasks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := h.uc.GetTasks(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get tasks", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(GetTasksResponse{
			Tasks: tasks,
		}))
	}
}

// GetTaskById get task by id
//
//	@Summary		Get task by id
//	@Description	get task by id
//	@Tags			tasks
//	@Produce		json
//	@Param			id	path		int	true	"task id"
//	@Success		200	{object}	responses.SucceededResponse[tasksrest.GetTaskByIdResponse]
//	@Failure		400	{object}	responses.ErrorResponse
//	@Failure		404	{object}	responses.ErrorResponse
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/tasks/{id} [get]
func (h *handler) GetTaskById() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		task, err := h.uc.GetTaskById(context.Background(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.Render(w, r, responses.ErrNotFound)
			} else {
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get task by id", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetTaskByIdResponse{
			Task: task,
		}))
	}
}

// GetTasksByUserId get tasks by user id
//
//	@Summary		Get tasks by user id
//	@Description	get tasks by user id
//	@Tags			tasks
//	@Produce		json
//	@Param			id	path		int	true	"user id"
//	@Success		200	{object}	responses.SucceededResponse[tasksrest.GetTasksByUserIdResponse]
//	@Failure		400	{object}	responses.ErrorResponse
//	@Failure		500	{object}	responses.ErrorResponse
//	@Router			/tasks/user/{id} [get]
func (h *handler) GetTasksByUserId() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userId, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		tasks, err := h.uc.GetTasksByUserId(context.Background(), userId)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get tasks by user id", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededRenderer(GetTasksByUserIdResponse{
			Tasks: tasks,
		}))
	}
}

// AddTask add new task
//
//	@Summary		Add task
//	@Description	add new task
//	@Tags			tasks
//	@Produce		json
//	@Param			request	body		tasksrest.AddTaskRequest	true	"query params"
//	@Success		201		{object}	responses.SucceededResponse[any]
//	@Failure		400		{object}	responses.ErrorResponse
//	@Failure		500		{object}	responses.ErrorResponse
//	@Router			/tasks [post]
func (h *handler) AddTask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AddTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		if err := h.ValidateStruct(req); err != nil {
			validateErr := err.(validator.ValidationErrors)
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "invalid request", Err: validateErr},
				responses.ErrBadRequest,
			)
			return
		}

		task := models.Task{
			Name:   req.Name,
			UserID: req.UserID,
			MarkID: req.MarkID,
		}

		_, err := h.uc.AddTask(context.Background(), task)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed add task", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededCreatedRenderer())
	}
}
