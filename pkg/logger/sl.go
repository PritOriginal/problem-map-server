package logger

import (
	"log/slog"
	"os"

	"github.com/PritOriginal/problem-map-server/pkg/logger/prettylog"
)

func Err(err error) slog.Attr {
	return slog.Attr{
		Key:   "error",
		Value: slog.StringValue(err.Error()),
	}
}

func SetupLogger(env string, f *os.File) *slog.Logger {
	var logger *slog.Logger
	switch env {
	case "local":
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true}))
	case "dev":
		logger = slog.New(prettylog.NewPrettyHandler(os.Stdout, prettylog.PrettyHandlerOptions{SlogOpts: slog.HandlerOptions{Level: slog.LevelDebug}}))
	case "prod":
		logger = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo, AddSource: true}))
	}

	return logger
}
