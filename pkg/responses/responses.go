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

func Success[T any](ctx *gin.Context, status int, data T) {
	ctx.JSON(status, Response[T]{
		Success: true,
		Payload: data,
	})
}

func OK[T any](ctx *gin.Context, data T) {
	ctx.JSON(http.StatusOK, Response[T]{
		Success: true,
		Payload: data,
	})
}

func Created[T any](ctx *gin.Context, data T) {
	ctx.JSON(http.StatusCreated, Response[T]{
		Success: true,
		Payload: data,
	})
}

func Fail(ctx *gin.Context, status int, message string) {
	ctx.JSON(status, Response[any]{
		Success: false,
		Error:   &ErrorInfo{Message: message},
	})
}

func BadRequest(ctx *gin.Context, message string) {
	Fail(ctx, http.StatusBadRequest, message)
}

func NotFound(ctx *gin.Context, message string) {
	Fail(ctx, http.StatusNotFound, message)
}

func Unauthorized(ctx *gin.Context, message string) {
	Fail(ctx, http.StatusUnauthorized, message)
}

func Conflict(ctx *gin.Context, message string) {
	Fail(ctx, http.StatusConflict, message)
}

func Internal(ctx *gin.Context, message string) {
	Fail(ctx, http.StatusInternalServerError, message)
}
