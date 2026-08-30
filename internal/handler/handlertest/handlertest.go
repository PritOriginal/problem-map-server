// Package handlertest contains helpers shared by REST handler unit tests.
package handlertest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/stretchr/testify/require"
)

// messages maps an HTTP error status to the message responses.FromError
// returns for it. Statuses missing here (e.g. 400 from request binding) are
// checked only for a non-empty message.
var messages = map[int]string{
	http.StatusUnauthorized:        responses.MsgUnauthorized,
	http.StatusForbidden:           responses.MsgForbidden,
	http.StatusNotFound:            responses.MsgNotFound,
	http.StatusConflict:            responses.MsgConflict,
	http.StatusInternalServerError: responses.MsgInternal,
}

// AssertResponse checks the recorded status code and the response envelope:
// a successful response must carry success=true and no error, an error
// response must carry success=false and the message FromError produces for
// that status.
func AssertResponse(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()

	require.Equal(t, wantStatus, w.Code, "body: %s", w.Body.String())

	if wantStatus == http.StatusNoContent {
		require.Empty(t, w.Body.String())
		return
	}

	var body responses.Response[json.RawMessage]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())

	if wantStatus < http.StatusBadRequest {
		require.True(t, body.Success)
		require.Nil(t, body.Error)
		return
	}

	require.False(t, body.Success)

	// A 401 produced by the gin-jwt middleware itself uses its own envelope
	// ({"code":401,"message":"..."}), not responses.Response.
	if body.Error == nil && wantStatus == http.StatusUnauthorized {
		var jwtBody struct {
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jwtBody))
		require.NotEmpty(t, jwtBody.Message, "body: %s", w.Body.String())
		return
	}

	require.NotNil(t, body.Error, "body: %s", w.Body.String())
	if msg, ok := messages[wantStatus]; ok {
		require.Equal(t, msg, body.Error.Message)
	} else {
		require.NotEmpty(t, body.Error.Message)
	}
}
