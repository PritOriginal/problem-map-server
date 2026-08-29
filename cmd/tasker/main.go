package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/app"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/spf13/cobra"
)

var (
	configPath string
	once       bool
	interval   time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "tasker",
	Short: "Assigns mark verification tasks to users",
	Long: `Expires overdue tasks and assigns new verification tasks.

By default the job runs on a schedule (tasker.interval / TASKER_INTERVAL,
overridable with --interval) until SIGINT/SIGTERM. Use --once to run a single
pass and exit.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(cmd.Context())
	},
}

func init() {
	rootCmd.Flags().StringVar(&configPath, "config", "", "path to config file (or CONFIG_PATH env)")
	rootCmd.Flags().BoolVar(&once, "once", false, "run a single pass and exit")
	rootCmd.Flags().DurationVar(&interval, "interval", 0, "override tasker.interval for scheduled mode (e.g. 15m)")
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Printf("tasker: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	configPath = config.ResolveConfigPath(configPath)
	if configPath == "" {
		return errors.New("config path is empty: pass --config or set CONFIG_PATH")
	}

	// Like the migrator, the tasker validates only the sections it uses
	// (no JWT keys), so it starts with a minimal config.
	cfg, err := config.ReadPath(configPath)
	if err != nil {
		return err
	}
	if interval > 0 {
		cfg.Tasker.Interval = interval
	}
	if err := errors.Join(cfg.DB.Validate(), cfg.Tasker.Validate()); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	logger, err := slogger.SetupLogger(cfg.Env)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		logger.Error("failed connection to database", slogger.Err(err))
		return err
	}
	logger.Info("PostgreSQL connected!")
	defer func() {
		if err := postgresDB.Close(); err != nil {
			logger.Error("an error occurred while closing the connection to the database", slogger.Err(err))
		}
	}()

	publisher, publisherCloser := app.NewPublisher(logger, cfg.Nats, nil)
	if publisherCloser != nil {
		defer func() {
			if err := publisherCloser.Close(); err != nil {
				logger.Error("an error occurred while closing the nats connection", slogger.Err(err))
			}
		}()
	}

	trManager := manager.Must(trmsqlx.NewDefaultFactory(postgresDB.DB))
	tasker := usecase.NewTasker(logger, cfg.Tasker, trManager, usecase.TaskerRepositories{
		Tasks: postgres.NewTasks(postgresDB.DB, trmsqlx.DefaultCtxGetter),
		Marks: postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter),
		Users: postgres.NewUsers(postgresDB.DB, trmsqlx.DefaultCtxGetter),
	}).WithEvents(publisher)

	if once {
		return finish(ctx, logger, runPass(ctx, logger, tasker))
	}

	logger.Info("tasker scheduled", slog.Duration("interval", cfg.Tasker.Interval))
	return runScheduled(ctx, logger, tasker, cfg.Tasker.Interval)
}

// runScheduled runs a pass immediately and then every interval until ctx is
// cancelled. A failed pass is logged and the schedule continues; only a
// shutdown signal stops the loop.
func runScheduled(ctx context.Context, logger *slog.Logger, tasker *usecase.Tasker, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := runPass(ctx, logger, tasker); err != nil {
			if ctx.Err() != nil {
				return finish(ctx, logger, err)
			}
			logger.Error("tasker pass failed", slogger.Err(err))
		}

		select {
		case <-ctx.Done():
			return finish(ctx, logger, nil)
		case <-ticker.C:
		}
	}
}

// runPass expires overdue tasks and assigns new ones. Every repository call
// observes ctx: on SIGINT/SIGTERM the in-flight query is cancelled and the
// pass returns, so the deferred DB close never races with a running query.
func runPass(ctx context.Context, logger *slog.Logger, tasker *usecase.Tasker) error {
	started := time.Now()

	expired, err := tasker.ExpireOverdue(ctx)
	if err != nil {
		return fmt.Errorf("expire overdue: %w", err)
	}

	stats, err := tasker.Update(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	logger.Info("tasker pass finished",
		slog.Int64("expired", expired),
		slog.Int("assigned", stats.Assigned),
		slog.Int("marks", stats.Marks),
		slog.Int("covered", stats.Covered),
		slog.Int("users", stats.Users),
		slog.Int("candidates", stats.Candidates),
		slog.Int("iterations", stats.Iterations),
		slog.Duration("duration", time.Since(started)),
	)

	return nil
}

// finish maps a shutdown signal to a clean exit (nil) and passes any other
// error through.
func finish(ctx context.Context, logger *slog.Logger, err error) error {
	if ctx.Err() != nil {
		if err != nil {
			logger.Warn("shutdown signal received, pass aborted", slogger.Err(err))
		} else {
			logger.Info("shutdown signal received, tasker stopped")
		}
		return nil
	}
	return err
}
