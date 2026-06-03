package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// ErrDuplicateNonce is returned when a request attempts to reuse an existing nonce.
var ErrDuplicateNonce = errors.New("duplicate nonce: request has already been processed")

// LedgerEntry represents a single transaction or balance adjustment event.
type LedgerEntry struct {
	ID        int64           `json:"id"`
	UserID    string          `json:"user"`
	Asset     string          `json:"asset"`
	Amount    decimal.Decimal `json:"amount"`
	Nonce     string          `json:"nonce"`
	Timestamp time.Time       `json:"timestamp"`
	CreatedAt time.Time       `json:"created_at"`
}

// LedgerRepository handles database operations for LedgerEntries.
type LedgerRepository struct {
	db *sql.DB
}

// NewLedgerRepository creates a new instance of LedgerRepository.
func NewLedgerRepository(db *sql.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

// CreateLedgerEntry inserts a ledger entry and updates the consolidated balance in a transaction.
func (r *LedgerRepository) CreateLedgerEntry(ctx context.Context, entry *LedgerEntry) error {
	// Start a database transaction to ensure atomicity
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	// Will rollback automatically if function returns with error before committing
	defer func() { _ = tx.Rollback() }()

	// 1. Insert the ledger entry
	if err := r.insertLedgerEntry(ctx, tx, entry); err != nil {
		return err
	}

	// 2. Upsert the consolidated user balance
	if err := r.upsertBalance(ctx, tx, entry.UserID, entry.Asset, entry.Amount); err != nil {
		return err
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// insertLedgerEntry executes the SQL query to insert a transaction log inside a transaction.
func (r *LedgerRepository) insertLedgerEntry(ctx context.Context, tx *sql.Tx, entry *LedgerEntry) error {
	query := `
		INSERT INTO ledger_entries (user_id, asset, amount, nonce, timestamp)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at;
	`
	err := tx.QueryRowContext(ctx, query, entry.UserID, entry.Asset, entry.Amount, entry.Nonce, entry.Timestamp).
		Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		// Check for PostgreSQL unique constraint violation (code 23505) on the nonce
		var pgErr *pq.Error
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return ErrDuplicateNonce
			}
		}
		return fmt.Errorf("failed to insert ledger entry: %w", err)
	}
	return nil
}

// upsertBalance executes the SQL query to add the transaction amount to the user's consolidated balance.
func (r *LedgerRepository) upsertBalance(ctx context.Context, tx *sql.Tx, userID string, asset string, amount decimal.Decimal) error {
	query := `
		INSERT INTO balances (user_id, asset, balance)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, asset)
		DO UPDATE SET balance = balances.balance + EXCLUDED.balance;
	`
	_, err := tx.ExecContext(ctx, query, userID, asset, amount)
	if err != nil {
		return fmt.Errorf("failed to update consolidated balance: %w", err)
	}
	return nil
}

// NonceExists checks if a nonce has already been used in the ledger_entries table.
// This allows fast checking before starting a full write transaction.
func (r *LedgerRepository) NonceExists(ctx context.Context, nonce string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM ledger_entries WHERE nonce = $1)"
	err := r.db.QueryRowContext(ctx, query, nonce).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check nonce existence: %w", err)
	}
	return exists, nil
}
