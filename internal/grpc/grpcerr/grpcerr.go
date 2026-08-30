// Package grpcerr maps usecase errors to gRPC status errors so that every
// gRPC handler reports the same codes for the same domain failures.
package grpcerr

import (
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Map converts a usecase error to a gRPC status error. msg is used as the
// status message; internal errors are logged with the given attributes and
// returned as codes.Internal without details.
func Map(log *slog.Logger, err error, msg string, attrs ...any) error {
	if err == nil {
		return nil
	}

	// Errors that already are gRPC statuses are passed through unchanged.
	if _, ok := status.FromError(err); ok {
		return err
	}

	attrs = append(attrs, logger.Err(err))

	code, ok := codeOf[usecase.Kind(err)]
	if !ok {
		log.Error(msg, attrs...)
		return status.Error(codes.Internal, msg)
	}
	if code == codes.Unavailable {
		log.Error(msg, attrs...)
	} else {
		log.Debug(msg, attrs...)
	}
	return status.Error(code, msg)
}

// codeOf is the gRPC code for each classified usecase error; kinds absent
// from the table (KindInternal) are reported as codes.Internal.
var codeOf = map[usecase.ErrorKind]codes.Code{
	usecase.KindNotFound:        codes.NotFound,
	usecase.KindConflict:        codes.AlreadyExists,
	usecase.KindUnauthorized:    codes.Unauthenticated,
	usecase.KindForbidden:       codes.PermissionDenied,
	usecase.KindUnavailable:     codes.Unavailable,
	usecase.KindInvalidArgument: codes.InvalidArgument,
	usecase.KindTooManyRequests: codes.ResourceExhausted,
}

// InvalidArgument returns a codes.InvalidArgument status with the given message.
func InvalidArgument(msg string) error {
	return status.Error(codes.InvalidArgument, msg)
}
