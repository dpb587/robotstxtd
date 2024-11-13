package storage

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

func Open(dsn string) (*sql.DB, error) {
	dh, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	_, err = dh.Exec(schema)
	if err != nil {
		return nil, fmt.Errorf("migrate: %v", err)
	}

	return dh, nil
}
