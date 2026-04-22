package logger

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/SladkyCitron/slogcolor"
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

func SetupLogger(env Environment) (*slog.Logger, error) {
	return SetupLoggerWithWriter(env, os.Stdout)
}

func SetupLoggerWithWriter(env Environment, out io.Writer) (*slog.Logger, error) {
	var logger *slog.Logger
	switch env {
	case Local:
		logger = slog.New(slogcolor.NewHandler(os.Stderr, &slogcolor.Options{Level: slog.LevelDebug, TimeFormat: time.RFC3339}))
	case Dev:
		logger = slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case Prod:
		logger = slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo, AddSource: true}))
	default:
		return logger, errors.New("invalid name environment")
	}

	return logger, nil
}
