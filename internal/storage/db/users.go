package db

import "github.com/jmoiron/sqlx"

type UsersRepository interface {
}

type UsersRepo struct {
	Conn *sqlx.DB
}
