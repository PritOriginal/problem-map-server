package handlers

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
)

type BaseHandler struct {
	Log      *slog.Logger
	Validate *validator.Validate
}

func (h *BaseHandler) ValidateStruct(req interface{}) error {
	return h.Validate.Struct(req)
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

func (h *BaseHandler) ParsePhotos(w http.ResponseWriter, r *http.Request) ([]io.Reader, error) {
	var photos []io.Reader
	for _, fheaders := range r.MultipartForm.File {
		for _, header := range fheaders {
			file, err := header.Open()
			if err != nil {
				return photos, err
			}

			img, format, err := image.Decode(file)
			if err != nil {
				return photos, err
			}

			if format == "png" {
				buf := new(bytes.Buffer)
				if err := jpeg.Encode(buf, img, nil); err != nil {
					return photos, err
				}
				photos = append(photos, buf)
			} else {
				photos = append(photos, file)
			}
		}
	}
	return photos, nil
}
