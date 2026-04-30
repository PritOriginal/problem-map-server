package responses

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response[T any] struct {
	Success bool       `json:"success"`
	Payload T          `json:"payload,omitempty"`
	Error   *ErrorInfo `json:"error,omitempty"`
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

func BadRequest(c *gin.Context, message string) {
	Fail(c, http.StatusBadRequest, message)
}

func NotFound(c *gin.Context, message string) {
	Fail(c, http.StatusNotFound, message)
}

func Unauthorized(c *gin.Context, message string) {
	Fail(c, http.StatusUnauthorized, message)
}

func Conflict(c *gin.Context, message string) {
	Fail(c, http.StatusConflict, message)
}

func Internal(c *gin.Context, message string) {
	Fail(c, http.StatusInternalServerError, message)
}
