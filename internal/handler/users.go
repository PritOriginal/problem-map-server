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

type UsersHandler struct {
	handlers.BaseHandler
	uc usecase.Users
}

func NewUsers(log *slog.Logger, uc usecase.Users) *UsersHandler {
	return &UsersHandler{handlers.BaseHandler{Log: log}, uc}
}

func (h *UsersHandler) GetUserById() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed parse id", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		user, err := h.uc.GetUserById(context.Background(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				h.Render(w, r, responses.ErrNotFound)
			} else {
				h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed get user by id", Err: err})
			}
			return
		}

		h.Render(w, r, responses.SucceededRenderer(user))
	}
}

func (h *UsersHandler) GetUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := h.uc.GetUsers(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get users", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(users))
	}
}

func (h *UsersHandler) AddUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user models.User
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			h.RenderError(w, r,
				handlers.HandlerError{Msg: "failed decode request body", Err: err},
				responses.ErrBadRequest,
			)
			return
		}

		_, err := h.uc.AddUser(context.Background(), user)
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "failed add user", Err: err})
			return
		}

		h.Render(w, r, responses.SucceededResponseOK)
	}
}
