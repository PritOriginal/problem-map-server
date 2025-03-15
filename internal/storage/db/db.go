package db

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/spf13/viper"
)

func Initialize() (*sqlx.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		viper.GetString("DB_HOST"), viper.GetInt("DB_PORT"), viper.GetString("DB_USERNAME"), viper.GetString("DB_PASSWORD"), viper.GetString("DB_NAME"))

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
