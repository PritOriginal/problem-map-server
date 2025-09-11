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

func New(cfg config.DatabaseConfig) (*Postgres, error) {
	const op = "storage.postgres.New"

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Name)

	db, err := sqlx.Open("postgres", psqlInfo)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	pingErr := db.Ping()
	if pingErr != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &Postgres{DB: db}, nil
}

func (s *Postgres) Stop() error {
	return s.DB.Close()
}
