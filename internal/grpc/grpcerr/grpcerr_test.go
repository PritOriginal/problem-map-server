package grpcerr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/grpc/grpcerr"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMap(t *testing.T) {
	log := slogdiscard.NewDiscardLogger()

	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{name: "NotFound", err: usecase.ErrNotFound, wantCode: codes.NotFound},
		{name: "WrappedNotFound", err: fmt.Errorf("op: %w", usecase.ErrNotFound), wantCode: codes.NotFound},
		{name: "Conflict", err: usecase.ErrConflict, wantCode: codes.AlreadyExists},
		{name: "Unauthorized", err: usecase.ErrUnauthorized, wantCode: codes.Unauthenticated},
		{name: "Unavailable", err: usecase.ErrUnavailable, wantCode: codes.Unavailable},
		{name: "Forbidden", err: usecase.ErrForbidden, wantCode: codes.PermissionDenied},
		{name: "InvalidArgument", err: usecase.ErrInvalidArgument, wantCode: codes.InvalidArgument},
		{name: "TooManyRequests", err: usecase.ErrTooManyRequests, wantCode: codes.ResourceExhausted},
		{name: "InvalidReferenceViaUsecase", err: fmt.Errorf("op: %w: %w", usecase.ErrInvalidArgument, repository.ErrInvalidReference), wantCode: codes.InvalidArgument},
		{name: "Unknown", err: errors.New("boom"), wantCode: codes.Internal},
		{name: "StatusPassthrough", err: status.Error(codes.PermissionDenied, "x"), wantCode: codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := grpcerr.Map(log, tt.err, "msg")
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, status.Code(err))
			if tt.wantCode != codes.PermissionDenied {
				// Internal details never leak into the status message.
				assert.Equal(t, "msg", status.Convert(err).Message())
			}
		})
	}

	assert.NoError(t, grpcerr.Map(log, nil, "msg"))
}

func TestInvalidArgument(t *testing.T) {
	err := grpcerr.InvalidArgument("bad")
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, "bad", status.Convert(err).Message())
}
