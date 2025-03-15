package responses

import (
	"net/http"

	"github.com/go-chi/render"
)

type SucceededResponse struct {
	HTTPStatusCode int `json:"-"` // http response status code

	Status  string       `json:"status"`
	Message string       `json:"message"`
	Payload *interface{} `json:"payload"`
}

// Render implements render.Renderer.
func (s *SucceededResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, s.HTTPStatusCode)
	return nil
}

type ErrorResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code

	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *ErrorResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, s.HTTPStatusCode)
	return nil
}

var (
	SucceededResponseOK = &SucceededResponse{HTTPStatusCode: http.StatusOK, Status: "succeeded", Message: ""}
	ErrConflict         = &ErrorResponse{HTTPStatusCode: http.StatusConflict, Status: "failed", Message: "Already exists"}
	ErrMethodNotAllowed = &ErrorResponse{HTTPStatusCode: http.StatusMethodNotAllowed, Status: "failed", Message: "Method not allowed"}
	ErrNotFound         = &ErrorResponse{HTTPStatusCode: http.StatusNotFound, Status: "failed", Message: "Resource not found"}
	ErrBadRequest       = &ErrorResponse{HTTPStatusCode: http.StatusBadRequest, Status: "failed", Message: "Bad request"}
	ErrUnauthorized     = &ErrorResponse{HTTPStatusCode: http.StatusUnauthorized, Status: "failed", Message: "Unauthorized"}
	ErrInternalServer   = &ErrorResponse{HTTPStatusCode: http.StatusInternalServerError, Status: "failed", Message: ""}
)

func SucceededRenderer(data interface{}) render.Renderer {
	return &SucceededResponse{
		HTTPStatusCode: http.StatusOK,
		Status:         "succeeded",
		Message:        "",
		Payload:        &data,
	}
}

func SucceededCreatedRenderer() render.Renderer {
	return &SucceededResponse{
		HTTPStatusCode: http.StatusCreated,
		Status:         "succeeded",
		Message:        "",
	}
}

func ErrorRenderer(err error) render.Renderer {
	return &ErrorResponse{
		Err:            err,
		HTTPStatusCode: http.StatusBadRequest,
		Status:         "failed",
		Message:        err.Error(),
	}
}

func ServerErrorRenderer(err error) *ErrorResponse {
	return &ErrorResponse{
		Err:            err,
		HTTPStatusCode: http.StatusInternalServerError,
		Status:         "failed",
		Message:        err.Error(),
	}
}
