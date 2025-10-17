package usersrest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/chi/v5"
)

type Users interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUsers(ctx context.Context) ([]models.User, error)
}

type handler struct {
	handlers.BaseHandler
	uc Users
}

func Register(r *chi.Mux, log *slog.Logger, uc Users) {
	handler := &handler{handlers.BaseHandler{Log: log}, uc}

	r.Route("/users", func(r chi.Router) {
		r.Get("/", handler.GetUsers())
		r.Get("/{id}", handler.GetUserById())
	})
}

func (h *handler) GetUserById() http.HandlerFunc {
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

func (h *handler) GetUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := h.uc.GetUsers(context.Background())
		if err != nil {
			h.RenderInternalError(w, r, handlers.HandlerError{Msg: "error get users", Err: err})
			return
		}
		h.Render(w, r, responses.SucceededRenderer(users))
	}
}
