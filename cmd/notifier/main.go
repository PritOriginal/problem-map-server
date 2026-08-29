package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/PritOriginal/problem-map-server/internal/app"
	"github.com/PritOriginal/problem-map-server/internal/app/notifier"
	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/spf13/cobra"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "notifier",
	Short: "Turns domain events into user notifications and webhook deliveries",
	Long: `Subscribes to mark.status_changed, task.assigned and check.added on NATS,
stores a notification for every addressee and hands it to the push sender.
A second consumer (mark.>, task.>, check.>) delivers every event to the
subscribed webhooks and retries failed deliveries on a schedule.
Runs until SIGINT/SIGTERM.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run()
	},
}

func init() {
	rootCmd.Flags().StringVar(&configPath, "config", "", "path to config file (or CONFIG_PATH env)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Printf("notifier: %v", err)
		os.Exit(1)
	}
}

func run() error {
	configPath = config.ResolveConfigPath(configPath)
	if configPath == "" {
		return errors.New("config path is empty: pass --config or set CONFIG_PATH")
	}

	// Like the tasker, the notifier validates only the sections it uses
	// (database and broker; no JWT keys).
	cfg, err := config.ReadPath(configPath)
	if err != nil {
		return err
	}
	if err := errors.Join(cfg.DB.Validate(), cfg.Nats.Validate()); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	logger, err := slogger.SetupLogger(cfg.Env)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	if code := app.Run(logger, notifier.New(logger, cfg)); code != 0 {
		return fmt.Errorf("exit code %d", code)
	}
	return nil
}
