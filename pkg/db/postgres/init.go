// Package postgres provides a sqlx-based connection pool with OTel instrumentation.
package postgres

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"github.com/uptrace/opentelemetry-go-extra/otelsqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// PostgresDsn carries the discrete fields needed to compose a libpq DSN.
type PostgresDsn struct {
	Host, User, Password, Port, Db string
	MaxOpen, MaxIdle               int
}

// NewPostgresDB returns a *sqlx.DB pool wrapped with OTel tracing/metrics.
// Always pings the DB at boot; returns error so callers can fail-fast.
func NewPostgresDB(dsn PostgresDsn) (*sqlx.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dsn.Host, dsn.Port, dsn.User, dsn.Password, dsn.Db,
	)

	db, err := otelsqlx.Open("postgres", connStr,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	if dsn.MaxOpen > 0 {
		db.SetMaxOpenConns(dsn.MaxOpen)
	}
	if dsn.MaxIdle > 0 {
		db.SetMaxIdleConns(dsn.MaxIdle)
	}
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return db, nil
}
