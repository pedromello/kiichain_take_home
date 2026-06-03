package models

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"
)

// Balance represents the consolidated balance of a single asset for a user.
type Balance struct {
	UserID  string          `json:"user_id"`
	Asset   string          `json:"asset"`
	Balance decimal.Decimal `json:"balance"`
}

// BalanceRepository handles database operations for Balances.
type BalanceRepository struct {
	db *sql.DB
}

// NewBalanceRepository creates a new instance of BalanceRepository.
func NewBalanceRepository(db *sql.DB) *BalanceRepository {
	return &BalanceRepository{db: db}
}

// GetUserBalances queries all asset balances for a given user.
// Returns a map of asset name to decimal balance. If no balances are found,
// returns an empty map (which maps to `{}` in JSON) rather than an error.
func (r *BalanceRepository) GetUserBalances(ctx context.Context, userID string) (map[string]decimal.Decimal, error) {
	rows, err := r.queryBalances(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBalances(rows)
}

// queryBalances executes the SELECT query to fetch asset balances from the database.
func (r *BalanceRepository) queryBalances(ctx context.Context, userID string) (*sql.Rows, error) {
	query := `
		SELECT asset, balance
		FROM balances
		WHERE user_id = $1;
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user balances: %w", err)
	}
	return rows, nil
}

// scanBalances iterates over database rows and aggregates them into the balance map.
func scanBalances(rows *sql.Rows) (map[string]decimal.Decimal, error) {
	balances := make(map[string]decimal.Decimal)
	for rows.Next() {
		var asset string
		var balance decimal.Decimal
		if err := rows.Scan(&asset, &balance); err != nil {
			return nil, fmt.Errorf("failed to scan balance row: %w", err)
		}
		balances[asset] = balance
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading balance rows: %w", err)
	}

	return balances, nil
}
