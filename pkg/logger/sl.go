package logger

import (
	"errors"
	"log/slog"
	"os"

	"github.com/PritOriginal/problem-map-server/pkg/logger/prettylog"
)

type Environment string

const (
	Local Environment = "local"
	Dev   Environment = "dev"
	Prod  Environment = "prod"
)

func Err(err error) slog.Attr {
	return slog.Attr{
		Key:   "error",
		Value: slog.StringValue(err.Error()),
	}
}

func SetupLogger(env Environment, logFIle *os.File) (*slog.Logger, error) {
	var logger *slog.Logger
	switch env {
	case Local:
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true}))
	case Dev:
		logger = slog.New(prettylog.NewPrettyHandler(os.Stdout, prettylog.PrettyHandlerOptions{SlogOpts: slog.HandlerOptions{Level: slog.LevelDebug}}))
	case Prod:
		logger = slog.New(slog.NewJSONHandler(logFIle, &slog.HandlerOptions{Level: slog.LevelInfo, AddSource: true}))
	default:
		return logger, errors.New("invalid name environment")
	}

	return logger, nil
}
