package handlers

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ErrBadRequest is returned (wrapped) by the request parsing helpers when a
// path or query parameter is malformed. Handlers should map it to a 400.
var ErrBadRequest = errors.New("bad request")

// ParamInt parses the integer path parameter with the given name.
// A malformed value is reported as an error wrapping ErrBadRequest.
func ParamInt(c *gin.Context, name string) (int, error) {
	value, err := strconv.Atoi(c.Param(name))
	if err != nil {
		return 0, fmt.Errorf("%w: invalid path param %q: %w", ErrBadRequest, name, err)
	}
	return value, nil
}

// QueryIntArray parses the comma-separated integer query parameter with the
// given name. A missing parameter yields an empty slice; a malformed value is
// reported as an error wrapping ErrBadRequest.
func QueryIntArray(c *gin.Context, name string) ([]int, error) {
	values, err := ParseIntArray(c.Query(name))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid query param %q: %w", ErrBadRequest, name, err)
	}
	return values, nil
}
