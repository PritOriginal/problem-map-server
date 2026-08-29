package responses

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the common envelope of every API answer.
// Meta is present only on list endpoints and carries pagination info;
// it sits next to payload so that payload keeps its domain shape.
type Response[T any] struct {
	Success bool       `json:"success"`
	Payload T          `json:"payload,omitempty"`
	Meta    *ListMeta  `json:"meta,omitempty"`
	Error   *ErrorInfo `json:"error,omitempty"`
}

// ListMeta describes the window returned by a list endpoint.
type ListMeta struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type ErrorInfo struct {
	Message string `json:"message"`
}

func Success[T any](c *gin.Context, status int, data T) {
	c.JSON(status, Response[T]{
		Success: true,
		Payload: data,
	})
}

func OK[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, Response[T]{
		Success: true,
		Payload: data,
	})
}

// OKList writes a 200 response with pagination meta next to the payload.
func OKList[T any](c *gin.Context, data T, meta ListMeta) {
	c.JSON(http.StatusOK, Response[T]{
		Success: true,
		Payload: data,
		Meta:    &meta,
	})
}

func Created[T any](c *gin.Context, data T) {
	c.JSON(http.StatusCreated, Response[T]{
		Success: true,
		Payload: data,
	})
}

func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, Response[any]{
		Success: false,
		Error:   &ErrorInfo{Message: message},
	})
}

// FailWithPayload writes an error response that still carries a payload,
// e.g. a readiness report describing which dependency is down.
func FailWithPayload[T any](c *gin.Context, status int, message string, payload T) {
	c.JSON(status, Response[T]{
		Success: false,
		Payload: payload,
		Error:   &ErrorInfo{Message: message},
	})
}

func BadRequest(c *gin.Context, message string) {
	Fail(c, http.StatusBadRequest, message)
}

func NotFound(c *gin.Context, message string) {
	Fail(c, http.StatusNotFound, message)
}

func Unauthorized(c *gin.Context, message string) {
	Fail(c, http.StatusUnauthorized, message)
}

func Forbidden(c *gin.Context, message string) {
	Fail(c, http.StatusForbidden, message)
}

func Conflict(c *gin.Context, message string) {
	Fail(c, http.StatusConflict, message)
}

func Internal(c *gin.Context, message string) {
	Fail(c, http.StatusInternalServerError, message)
}
