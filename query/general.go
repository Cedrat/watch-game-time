package query

import "github.com/jmoiron/sqlx"

type Database struct {
	*sqlx.DB
}

func NewDatabase(db *sqlx.DB) *Database {
	return &Database{
		db,
	}
}
