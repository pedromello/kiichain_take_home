package tests

import (
	"database/sql"
	"os"
	"testing"

	"kiichain-assessment/config"
	"kiichain-assessment/pkg/models"

	"github.com/shopspring/decimal"
)

// Orchestrator coordinates database operations, seeding, and assertions for integration tests.
type Orchestrator struct {
	DB     *sql.DB
	Config *config.Config
}

// NewOrchestrator initializes configurations, database connection pool, runs startup migrations.
// If the database is not available, the test is skipped.
func NewOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()

	// Ensure temporary environment variables for tests are set
	if os.Getenv("HMAC_SECRET") == "" {
		os.Setenv("HMAC_SECRET", "test-secret-key-very-long-and-secure")
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Orchestrator failed to load config: %v", err)
	}

	db, err := models.InitDB(cfg)
	if err != nil {
		t.Skipf("Orchestrator: skipping integration test, Postgres not reachable: %v", err)
	}

	return &Orchestrator{
		DB:     db,
		Config: cfg,
	}
}

// Close closes the underlying database connection pool.
func (o *Orchestrator) Close() {
	if o.DB != nil {
		o.DB.Close()
	}
}

// CleanDatabase truncates both ledger_entries and balances tables to ensure a clean state.
func (o *Orchestrator) CleanDatabase(t *testing.T) {
	t.Helper()
	query := "TRUNCATE TABLE ledger_entries, balances RESTART IDENTITY CASCADE;"
	if _, err := o.DB.Exec(query); err != nil {
		t.Fatalf("Orchestrator failed to clean database: %v", err)
	}
}

// SeedBalance inserts a consolidated balance record directly into the database.
func (o *Orchestrator) SeedBalance(t *testing.T, userID, asset string, balanceStr string) {
	t.Helper()
	val, err := decimal.NewFromString(balanceStr)
	if err != nil {
		t.Fatalf("Orchestrator failed to parse seed balance string '%s': %v", balanceStr, err)
	}

	query := `
		INSERT INTO balances (user_id, asset, balance)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, asset)
		DO UPDATE SET balance = EXCLUDED.balance;
	`
	if _, err := o.DB.Exec(query, userID, asset, val); err != nil {
		t.Fatalf("Orchestrator failed to seed balance record for user '%s', asset '%s': %v", userID, asset, err)
	}
}

// GetBalance queries the database directly for a user's asset balance.
// Returns the decimal amount and a boolean indicating if a balance record was found.
func (o *Orchestrator) GetBalance(t *testing.T, userID, asset string) (decimal.Decimal, bool) {
	t.Helper()
	var val decimal.Decimal
	query := "SELECT balance FROM balances WHERE user_id = $1 AND asset = $2"
	err := o.DB.QueryRow(query, userID, asset).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return decimal.Zero, false
		}
		t.Fatalf("Orchestrator failed to query balance for user '%s', asset '%s': %v", userID, asset, err)
	}
	return val, true
}
