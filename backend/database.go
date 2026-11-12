package main

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/lib/pq"
	"time"
)

func openStore(dsn string) (PostgresStore, error) {
	if dsn == "" {
		return PostgresStore{}, errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return PostgresStore{}, errors.New("invalid database configuration")
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return PostgresStore{}, errors.New("Postgres is unavailable")
	}
	store := PostgresStore{DB: db}
	if err = store.Initialize(ctx); err != nil {
		_ = db.Close()
		return PostgresStore{}, errors.New("failed to initialize mock storage")
	}
	return store, nil
}
