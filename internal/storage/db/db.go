package db

import (
	"fmt"

	"github.com/PritOriginal/problem-map-server/configs"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func Initialize(cfg configs.DatabaseConfig) (*sqlx.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Name)

	db, err := sqlx.Open("postgres", psqlInfo)
	if err != nil {
		return db, err
	}

	pingErr := db.Ping()
	if pingErr != nil {
		return db, err
	}
	return db, nil
}
