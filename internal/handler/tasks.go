package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
)

type TasksHandler struct {
	handlers.BaseHandler
	uc usecase.Tasks
}

func NewTasks(log *slog.Logger, uc usecase.Tasks) *TasksHandler {
	return &TasksHandler{handlers.BaseHandler{Log: log}, uc}
}

func (h *TasksHandler) GetTasks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := h.uc.GetTasks(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get tasks", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(tasks))
	}
}

func (h *TasksHandler) GetTaskById() http.HandlerFunc {
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

		h.Render(w, r, responses.SucceededRenderer(task))
	}
}

func (h *TasksHandler) GetTasksByUserId() http.HandlerFunc {
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
			if errors.Is(err, storage.ErrNotFound) {
				h.Render(w, r, responses.ErrNotFound)
			} else {
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get tasks by user id", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededRenderer(tasks))
	}
}

func (h *TasksHandler) AddTask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var task models.Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		_, err := h.uc.AddTask(context.Background(), task)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed add task", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededResponseOK)
	}
}
