package handlers

import (
	"log/slog"
	"net/http"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/render"
)

type BaseHandler struct {
	Log *slog.Logger
}

func (h *BaseHandler) Render(w http.ResponseWriter, r *http.Request, v render.Renderer) {
	if err := render.Render(w, r, v); err != nil {
		h.Log.Error("failed succeeded render", logger.Err(err))
		render.Render(w, r, responses.ErrInternalServer)
		return
	}
}

type HandlerError struct {
	Msg string
	Err error
}

func (h *BaseHandler) RenderError(w http.ResponseWriter, r *http.Request, handlerErr HandlerError, v render.Renderer) {
	h.Log.Error(handlerErr.Msg, logger.Err(handlerErr.Err))
	render.Render(w, r, v)
}

func (h *BaseHandler) RenderInternalError(w http.ResponseWriter, r *http.Request, handlerErr HandlerError) {
	h.RenderError(w, r, handlerErr, responses.ErrInternalServer)
}
