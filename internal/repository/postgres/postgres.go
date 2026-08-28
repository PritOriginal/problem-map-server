package postgres

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type Postgres struct {
	DB *sqlx.DB
}

func New(cfg config.DatabaseConfig) (*Postgres, error) {
	const op = "storage.postgres.New"

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Name)

	db, err := sqlx.Open("postgres", psqlInfo)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &Postgres{DB: db}, nil
}

// Ping verifies the database connection is alive.
func (s *Postgres) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}

// Close closes the underlying connection pool.
func (s *Postgres) Close() error {
	return s.DB.Close()
}

// Stop is an alias of Close kept for backward compatibility.
func (s *Postgres) Stop() error {
	return s.Close()
}
