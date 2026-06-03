# Webhook Ledger Service (Go + PostgreSQL + MVC)

A secure, high-performance HTTP service implemented in Go to process signed ledger webhooks and query consolidated user balances. It strictly adheres to the Model-View-Controller (MVC) pattern and leverages PostgreSQL for transaction storage and balance consistency.

---

## Architecture & Tech Stack

* **Framework/Router**: [Chi (`github.com/go-chi/chi/v5`)](https://github.com/go-chi/chi) - A lightweight, net/http-compatible router for structured logging, panics recovery, and request ID tracking.
* **Database**: **PostgreSQL 16** - Selected for strong transactional safety (`ACID`) and precise storage.
* **Monetary Precision**: [Shopspring Decimal (`github.com/shopspring/decimal`)](https://github.com/shopspring/decimal) - Ensures absolute decimal precision on all mathematical operations (handling 18 decimal places or more).
* **Database Driver**: [lib/pq (`github.com/lib/pq`)](https://github.com/lib/pq) - Standard pure-Go driver for PostgreSQL.
* **Structured Logging**: Native `log/slog` (introduced in Go 1.21) - Production-grade structured logging format generating JSON outputs.
* **Containerization**: Multi-stage `Dockerfile` and `docker-compose.yml` for unified orchestration.

---

## Folder Structure

```
kiichain-assessment/
├── cmd/
│   └── server/
│       └── main.go             # Entrypoint (initializes DB, router, and starts server)
├── config/
│   └── config.go           # Loads environment variables (DB credentials, secret key, port)
├── db/
│   └── migrations/
│       └── 000001_init.up.sql  # SQL schema definition (ledger_entries, balances)
├── pkg/
│   ├── models/                 # M: Models & Data Layer (encapsulates DB operations)
│   │   ├── db.go               # DB connection pool setup & migrations runner
│   │   ├── ledger_entry.go     # Transactions/Ledger DB logic & struct
│   │   └── balance.go          # User Balances DB logic & struct
│   ├── views/                  # V: Views & Serialization (JSON formatting)
│   │   └── response.go         # API JSON structures & response writers
│   ├── controllers/            # C: Controllers (receives requests, calls models)
│   │   ├── webhook_controller.go
│   │   └── balance_controller.go
│   └── middleware/             # HTTP Middlewares (logging, headers, authentication)
│       ├── auth.go             # HMAC signature & replay attack validation
│       ├── logger.go           # Structured logging mapping request ID
│       └── headers.go          # Custom headers middleware (X-Empty-Header)
├── tests/                      # Integration testing suite
│   └── integration/            # Route-based integration test files
│       ├── webhook/
│       │   └── post_test.go    # Integration tests for POST /webhook
│       └── balance/
│           └── get_test.go     # Integration tests for GET /balance/{user}
├── Dockerfile                  # Production multi-stage Docker build
├── docker-compose.yml          # Local container environment (App + Postgres)
├── README.md                   # This documentation guide
└── run.sh                      # E2E integration test script using curl & openssl
```

---

## Getting Started

### 1. Run the Entire System (App + Database)
You can build and start the application containerized alongside PostgreSQL using:
```bash
docker compose up --build
```
This command:
1. Spins up PostgreSQL and runs a healthcheck.
2. Builds the Go application statically.
3. Automatically runs SQL migrations on application startup.
4. Exposes the API server on `http://localhost:8080`.

---

## Configuration

The application is configured using environment variables (with sensible defaults):

| Env Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | HTTP Port the server listens on | `8080` |
| `HMAC_SECRET` | Secret key used to verify HMAC-SHA256 signatures | *(Required)* |
| `TOLERANCE_MINUTES` | Time window tolerance in minutes to prevent replay attacks | `5` |
| `DB_HOST` | Hostname of the Postgres DB | `localhost` |
| `DB_PORT` | Port of the Postgres DB | `5432` |
| `DB_USER` | Username for the Postgres DB | `postgres` |
| `DB_PASSWORD` | Password for the Postgres DB | `postgres` |
| `DB_NAME` | Name of the database to use | `ledger` |
| `DB_SSLMODE` | SSL Mode for Postgres connection | `disable` |

---

## API Documentation

### 1. Update Ledger Webhook
* **Endpoint**: `POST /webhook`
* **Security Headers**:
  * `X-Timestamp`: The UNIX timestamp (in seconds) of when the request was created.
  * `X-Nonce`: A unique nonce string for each request.
  * `X-Signature`: Hex-encoded HMAC-SHA256 signature.
* **Signature Payload Format**:
  `payload = X-Timestamp + "\n" + X-Nonce + "\n" + <raw_request_body_bytes>`
* **Request Body**:
  ```json
  {
    "user": "user_alice",
    "asset": "ETH",
    "amount": "1.250000000000000000"
  }
  ```
* **Success Response (200 OK)**:
  ```json
  {
    "status": "success"
  }
  ```
* **Failure Response Statuses**:
  * `400 Bad Request`: Missing headers, invalid JSON format, or expired timestamp (replay check).
  * `401 Unauthorized`: Invalid request signature.
  * `409 Conflict`: Replay attack detected (reused nonce).
  * `500 Internal Server Error`: Database transactional failures.

---

### 2. Retrieve User Balance
* **Endpoint**: `GET /balance/{user}`
* **Success Response (200 OK)**:
  ```json
  {
    "user": "user_alice",
    "balances": {
      "ETH": "1.250000000000000000"
    }
  }
  ```
  *Note: Users with no recorded balances return a formatted empty object: `{"user":"unknown_user","balances":{}}`.*

---

## Verification & Testing

### 1. Run Automated Integration Tests
Integration tests are located under `/tests/integration/...` and verify actual HTTP calls against a real PostgreSQL instance.

To run the integration tests locally:
1. Start only the PostgreSQL container:
   ```bash
   docker compose up -d db
   ```
2. Run the Go test command:
   ```bash
   go test -v ./tests/integration/...
   ```
   *(Note: This uses standard Go testing package. If Go is not on your PATH, use the absolute path: `& "C:\Program Files\Go\bin\go.exe" test -v ./tests/integration/...`)*

### 2. Run E2E Flow Script
We provide a bash script `run.sh` that simulates a full transaction lifecycle:
1. Verifies the initial empty balance.
2. Deposits `1500.50 USDT` via a signed webhook (calculating HMAC).
3. Asserts the balance changed.
4. Simulates a **Replay Attack** by sending the same nonce (asserts `409 Conflict`).
5. Simulates an **Invalid Signature** webhook (asserts `401 Unauthorized`).
6. Simulates an **Expired Timestamp** (asserts `400 Bad Request`).
7. Deducts `500.25 USDT` via a signed webhook.
8. Asserts the final balance matches exactly `1000.25 USDT`.

To run the verification script:
```bash
chmod +x run.sh
./run.sh
```
