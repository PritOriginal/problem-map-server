package postgres

import (
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type Postgres struct {
	DB *sqlx.DB
}

// New opens a connection pool and verifies it with a ping.
func New(cfg config.DatabaseConfig) (*Postgres, error) {
	const op = "storage.postgres.New"

	db, err := sqlx.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	db.SetMaxOpenConns(cfg.Pool.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Pool.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Pool.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &Postgres{DB: db}, nil
}

func (s *Postgres) Stop() error {
	return s.DB.Close()
}
