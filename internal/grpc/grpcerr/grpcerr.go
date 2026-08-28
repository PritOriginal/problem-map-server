// Package grpcerr maps usecase errors to gRPC status errors so that every
// gRPC handler reports the same codes for the same domain failures.
package grpcerr

import (
	"errors"
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

	switch {
	case errors.Is(err, usecase.ErrNotFound):
		log.Debug(msg, append(attrs, logger.Err(err))...)
		return status.Error(codes.NotFound, msg)
	case errors.Is(err, usecase.ErrConflict):
		log.Debug(msg, append(attrs, logger.Err(err))...)
		return status.Error(codes.AlreadyExists, msg)
	case errors.Is(err, usecase.ErrUnauthorized):
		log.Debug(msg, append(attrs, logger.Err(err))...)
		return status.Error(codes.Unauthenticated, msg)
	case errors.Is(err, usecase.ErrUnavailable):
		log.Error(msg, append(attrs, logger.Err(err))...)
		return status.Error(codes.Unavailable, msg)
	default:
		log.Error(msg, append(attrs, logger.Err(err))...)
		return status.Error(codes.Internal, msg)
	}
}

// InvalidArgument returns a codes.InvalidArgument status with the given message.
func InvalidArgument(msg string) error {
	return status.Error(codes.InvalidArgument, msg)
}
