-- Create ledger_entries table to record each webhook request event
CREATE TABLE IF NOT EXISTS ledger_entries (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    asset VARCHAR(50) NOT NULL,
    amount NUMERIC(36, 18) NOT NULL,
    nonce VARCHAR(255) NOT NULL UNIQUE,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create balances table to store consolidated user asset balances
CREATE TABLE IF NOT EXISTS balances (
    user_id VARCHAR(255) NOT NULL,
    asset VARCHAR(50) NOT NULL,
    balance NUMERIC(36, 18) NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, asset)
);

-- Index to optimize querying balances for a specific user
CREATE INDEX IF NOT EXISTS idx_balances_user_id ON balances(user_id);
