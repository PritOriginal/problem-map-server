package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/cobra"
)

var (
	migrationsPath string
	cfg            *config.Config
	configPath     string
)

var rootCmd = &cobra.Command{
	Use:   "migrator",
	Short: "Database migration tool",
	Long:  "A CLI tool for managing database migrations",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if migrationsPath == "" {
			return errors.New("migrations-path is required")
		}
		cfg = config.MustLoadPath(configPath)
		return nil
	},
}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	Long:  "Apply all pending migrations or a specific number of steps",
	RunE: func(cmd *cobra.Command, args []string) error {
		steps, _ := cmd.Flags().GetInt("steps")

		m, err := createMigrate()
		if err != nil {
			return err
		}
		defer m.Close()

		if steps > 0 {
			fmt.Printf("Applying next %d migrations...\n", steps)
			err = m.Steps(steps)
		} else {
			fmt.Println("Applying all pending migrations...")
			err = m.Up()
		}

		if err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				fmt.Println("No migrations to apply")
				return nil
			}
			return err
		}

		fmt.Println("Migrations applied successfully")
		return nil
	},
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback migrations",
	Long:  "Rollback last migration or a specific number of steps",
	RunE: func(cmd *cobra.Command, args []string) error {
		steps, _ := cmd.Flags().GetInt("steps")

		m, err := createMigrate()
		if err != nil {
			return err
		}
		defer m.Close()

		if steps > 0 {
			fmt.Printf("Rolling back last %d migrations...\n", steps)
			err = m.Steps(-steps)
		} else {
			fmt.Println("Rolling back last migration...")
			err = m.Steps(-1)
		}

		if err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				fmt.Println("No migrations to rollback")
				return nil
			}
			return err
		}

		fmt.Println("Migrations rolled back successfully")
		return nil
	},
}

var forceCmd = &cobra.Command{
	Use:   "force [version]",
	Short: "Force a specific migration version",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var version int
		fmt.Sscanf(args[0], "%d", &version)

		m, err := createMigrate()
		if err != nil {
			return err
		}
		defer m.Close()

		if err := m.Force(version); err != nil {
			return err
		}

		fmt.Printf("Forced version to %d\n", version)
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show current migration version",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := createMigrate()
		if err != nil {
			return err
		}
		defer m.Close()

		version, dirty, err := m.Version()
		if err != nil {
			return err
		}

		fmt.Printf("Current version: %d\n", version)
		if dirty {
			fmt.Println("Status: dirty (incomplete migration)")
		} else {
			fmt.Println("Status: clean")
		}
		return nil
	},
}

func createMigrate() (*migrate.Migrate, error) {
	databaseURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.DB.Username, cfg.DB.Password, cfg.DB.Host, cfg.DB.Port, cfg.DB.Name)

	return migrate.New(
		"file://"+migrationsPath,
		databaseURL,
	)
}

var dropCmd = &cobra.Command{
	Use:   "drop",
	Short: "Drop everything in the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := createMigrate()
		if err != nil {
			return err
		}
		defer m.Close()

		if err := m.Drop(); err != nil {
			return err
		}

		fmt.Println("Database dropped successfully")
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&migrationsPath, "migrations-path", "", "Path to migrations directory")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")

	rootCmd.MarkPersistentFlagRequired("migrations-path")

	upCmd.Flags().IntP("steps", "s", 0, "Number of migrations to apply (0 = all)")
	downCmd.Flags().IntP("steps", "s", 0, "Number of migrations to rollback (0 = last one)")

	rootCmd.AddCommand(upCmd, downCmd, forceCmd, versionCmd, dropCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
