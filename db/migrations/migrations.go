package migrations

import _ "embed"

// InitSQL holds the embedded SQL migration queries.
//
//go:embed 000001_init.up.sql
var InitSQL string
