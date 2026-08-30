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

// ErrorInfo describes a failure. Message is human-readable; Code is the
// machine-readable discriminator of the cases a client must tell apart
// (see the Code* constants), omitted where there is nothing to
// disambiguate.
type ErrorInfo struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// Machine-readable error codes carried by ErrorInfo.Code. They are the
// single source shared by the handlers and the middlewares, so that
// clients never have to tell responses apart by their message or by a
// status code reused for unrelated cases.
const (
	// CodeSimilarMarks marks the 409 of POST /marks raised because active
	// marks of the same type already exist nearby; the payload lists them
	// and the client may repeat the request with ?force=true.
	CodeSimilarMarks = "similar_marks"
	// CodeIdempotencyInFlight marks the 425 returned while the first
	// request with the same Idempotency-Key is still being handled; the
	// client should retry after Retry-After seconds.
	CodeIdempotencyInFlight = "idempotency_in_flight"
	// CodeIdempotencyKeyReused marks the 422 returned when an
	// Idempotency-Key is reused with a different payload.
	CodeIdempotencyKeyReused = "idempotency_key_reused"
)

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

// FailWithCode writes an error response carrying a machine-readable code
// next to the message.
func FailWithCode(c *gin.Context, status int, code, message string) {
	c.JSON(status, Response[any]{
		Success: false,
		Error:   &ErrorInfo{Message: message, Code: code},
	})
}

// FailWithCodePayload writes an error response that carries both a
// machine-readable code and a payload describing the failure.
func FailWithCodePayload[T any](c *gin.Context, status int, code, message string, payload T) {
	c.JSON(status, Response[T]{
		Success: false,
		Payload: payload,
		Error:   &ErrorInfo{Message: message, Code: code},
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

func TooManyRequests(c *gin.Context, message string) {
	Fail(c, http.StatusTooManyRequests, message)
}

func Internal(c *gin.Context, message string) {
	Fail(c, http.StatusInternalServerError, message)
}
