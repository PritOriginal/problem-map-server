package handler

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/go-chi/render"
)

type MapHandler struct {
	uc usecase.Map
}

func NewMap(uc usecase.Map) *MapHandler {
	return &MapHandler{uc: uc}
}

func (h *MapHandler) GetDistricts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		districts, err := h.uc.GetDistricts(context.Background())
		if err != nil {
			render.Render(w, r, responses.ErrInternalServer)
			return
		}
		if err := render.Render(w, r, responses.SucceededRenderer(districts)); err != nil {
			render.Render(w, r, responses.ErrInternalServer)
			return
		}
	}
}

func (h *MapHandler) GetMarks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		marks, err := h.uc.GetMarks(context.Background())
		if err != nil {
			log.Print(err)
			render.Render(w, r, responses.ErrInternalServer)
			return
		}
		if err := render.Render(w, r, responses.SucceededRenderer(marks)); err != nil {
			render.Render(w, r, responses.ErrInternalServer)
			return
		}
	}
}

func (h *MapHandler) AddMark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var newMark models.Mark
		if err := json.NewDecoder(r.Body).Decode(&newMark); err != nil {
			render.Render(w, r, responses.ErrBadRequest)
			return
		}

		if err := h.uc.AddMark(context.Background(), newMark); err != nil {
			render.Render(w, r, responses.ErrInternalServer)
			return
		}

		if err := render.Render(w, r, responses.SucceededCreatedRenderer()); err != nil {
			render.Render(w, r, responses.ErrInternalServer)
			return
		}
	}
}

func (h *MapHandler) AddPhotos() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			render.Render(w, r, responses.ErrBadRequest)
			return
		}

		// image, format, err := image.Decode(r.Body)

		if err := os.WriteFile("test.jpg", data, 0666); err != nil {
			render.Render(w, r, responses.ErrInternalServer)
			return
		}

		if err := render.Render(w, r, responses.SucceededCreatedRenderer()); err != nil {
			render.Render(w, r, responses.ErrInternalServer)
			return
		}
	}
}
