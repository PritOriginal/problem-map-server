// Package app holds the lifecycle pieces shared by every server binary:
// a runner that ties an application to OS signals and a closer group that
// shuts infrastructure clients down in a deterministic order.
package app

import (
	"context"
	"io"
	"log/slog"
	"os/signal"
	"syscall"

	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

// App is a long-running server: Run blocks until the server stops, Stop
// shuts it down gracefully.
type App interface {
	Run() error
	Stop()
}

// Run starts app in the background, waits for SIGINT/SIGTERM or for Run to
// return, then stops the app. It returns the process exit code.
func Run(log *slog.Logger, app App) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run()
	}()

	exitCode := 0
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			log.Error("server failed", slogger.Err(err))
			exitCode = 1
		}
	}

	// Restore default signal handling before the graceful stop so that a
	// second SIGINT/SIGTERM terminates the process immediately.
	stop()
	app.Stop()

	log.Info("server stopped")
	return exitCode
}

// Closers is an ordered group of named io.Closer values. Close releases them
// in reverse registration order, so dependants registered after their
// dependencies are closed first.
type Closers []namedCloser

type namedCloser struct {
	name string
	c    io.Closer
}

// Add registers c under name. A nil c is ignored.
func (cs *Closers) Add(name string, c io.Closer) {
	if c == nil {
		return
	}
	*cs = append(*cs, namedCloser{name: name, c: c})
}

// Close closes every registered closer in reverse order, logging failures.
func (cs Closers) Close(log *slog.Logger) {
	for i := len(cs) - 1; i >= 0; i-- {
		nc := cs[i]
		if err := nc.c.Close(); err != nil {
			log.Error("an error occurred while closing the client", slog.String("client", nc.name), slogger.Err(err))
		}
	}
}
