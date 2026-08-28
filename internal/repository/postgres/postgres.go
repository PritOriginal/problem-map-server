package postgres

import (
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const (
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 5 * time.Minute
)

type Postgres struct {
	DB *sqlx.DB
}

func orDefault[T int | time.Duration](v, def T) T {
	if v <= 0 {
		return def
	}
	return v
}

func New(cfg config.DatabaseConfig) (*Postgres, error) {
	const op = "storage.postgres.New"

	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Name, sslMode)

	db, err := sqlx.Open("postgres", psqlInfo)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	db.SetMaxOpenConns(orDefault(cfg.Pool.MaxOpenConns, defaultMaxOpenConns))
	db.SetMaxIdleConns(orDefault(cfg.Pool.MaxIdleConns, defaultMaxIdleConns))
	db.SetConnMaxLifetime(orDefault(cfg.Pool.ConnMaxLifetime, defaultConnMaxLifetime))

	if pingErr := db.Ping(); pingErr != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: %w", op, pingErr)
	}
	return &Postgres{DB: db}, nil
}

func (s *Postgres) Stop() error {
	return s.DB.Close()
}
