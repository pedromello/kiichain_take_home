package models

import (
	"database/sql"
	"fmt"
	"kiichain-assessment/config"
	"kiichain-assessment/db/migrations"

	// Register the PostgreSQL driver for database/sql
	_ "github.com/lib/pq"
)

// InitDB initializes a PostgreSQL connection pool and runs startup migrations.
func InitDB(cfg *config.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening database connection: %w", err)
	}

	// Ping database to verify connection is established
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error pinging database: %w", err)
	}

	// Run initial schema migrations using a PostgreSQL transaction and advisory lock
	// to prevent DDL race conditions when multiple test suites or services start concurrently.
	tx, err := db.Begin()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error beginning migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Acquire a transaction-level advisory lock using a constant key (e.g., 42069)
	if _, err = tx.Exec("SELECT pg_advisory_xact_lock(42069);"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error acquiring migration advisory lock: %w", err)
	}

	if _, err = tx.Exec(migrations.InitSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error running database schema migrations: %w", err)
	}

	if err = tx.Commit(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error committing migration transaction: %w", err)
	}

	return db, nil
}
