#!/usr/bin/env bash

# Exit immediately if a command exits with a non-zero status
set -e

# Load environment variables from .env.development if it exists
if [ -f .env.development ]; then
    export $(grep -v '^#' .env.development | xargs)
fi

# Configuration
SERVER_URL=${SERVER_URL:-"http://localhost:8080"}
SECRET=${HMAC_SECRET:-"test-secret-key-should-be-long-and-secure"}

# Generate dynamic user ID and run ID to ensure idempotency across multiple script runs
RUN_ID=$(date +%s)
USER_ID="user_test_$RUN_ID"
ASSET="USDT"

echo "====================================================================="
echo "Starting E2E Webhook flow testing on $SERVER_URL"
echo "Targeting User: $USER_ID"
echo "====================================================================="

# Helper to calculate signature and send webhook
# Args: $1=Amount, $2=Nonce, $3=TimestampOffset(seconds), $4=BadSignatureOverride, $5=ExpectedStatus
send_webhook() {
    local amount="$1"
    local nonce="$2"
    local offset="$3"
    local bad_sig="$4"
    local expected_status="$5"

    local timestamp
    timestamp=$(date +%s)
    if [ -n "$offset" ]; then
        timestamp=$((timestamp + offset))
    fi

    local body
    body="{\"user\":\"$USER_ID\",\"asset\":\"$ASSET\",\"amount\":\"$amount\"}"

    # Calculate standard HMAC-SHA256 signature
    # Payload format: X-Timestamp + "\n" + X-Nonce + "\n" + request_body
    local payload
    payload=$(printf "%s\n%s\n%s" "$timestamp" "$nonce" "$body")
    
    local signature
    signature=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $NF}')

    if [ -n "$bad_sig" ]; then
        signature="invalid_signature_override_value"
    fi

    echo "--> Sending POST /webhook (Amount: $amount, Nonce: $nonce, Timestamp: $timestamp)"
    
    # Send HTTP Request and capture both Response Body and HTTP Status Code
    local response
    local status_code
    response=$(curl -s -w "\n%{http_code}" -X POST "$SERVER_URL/webhook" \
        -H "Content-Type: application/json" \
        -H "X-Timestamp: $timestamp" \
        -H "X-Nonce: $nonce" \
        -H "X-Signature: $signature" \
        -H "X-Empty-Header: " \
        -d "$body")

    status_code=$(echo "$response" | tail -n1)
    local body_content
    body_content=$(echo "$response" | head -n -1)

    echo "    Status Received: $status_code"
    echo "    Body: $body_content"

    if [ "$status_code" -ne "$expected_status" ]; then
        echo "[ERROR] Expected HTTP status $expected_status, got $status_code!"
        exit 1
    fi
    echo "    [OK] Match expected status $expected_status"
    echo "---------------------------------------------------------------------"
}

# 1. Clear database status check (Verify initial balance query)
echo "1. Checking initial balance of user $USER_ID (should be empty/0)"
initial_balance=$(curl -s "$SERVER_URL/balance/$USER_ID")
echo "   Response: $initial_balance"
echo "---------------------------------------------------------------------"

# 2. Success Webhook deposit: +1500.50 USDT
echo "2. Sending valid webhook: Deposit 1500.50 USDT"
send_webhook "1500.500000000000000000" "${RUN_ID}_nonce_1" "0" "" "200"

# 3. Verify balance has been updated
echo "3. Querying balance after deposit"
balance_after_deposit=$(curl -s -i "$SERVER_URL/balance/$USER_ID")
echo "$balance_after_deposit"
echo "---------------------------------------------------------------------"

# 4. Failure: Replay Attack (Resend same nonce)
echo "4. Replay attack: Resending webhook with reused ${RUN_ID}_nonce_1"
# Should return 409 Conflict
send_webhook "100.00" "${RUN_ID}_nonce_1" "0" "" "409"

# 5. Failure: Invalid Signature
echo "5. Sending webhook with invalid signature"
# Should return 401 Unauthorized
send_webhook "100.00" "${RUN_ID}_nonce_2" "0" "true" "401"

# 6. Failure: Expired Timestamp (Replay Prevention check 2)
echo "6. Sending webhook with expired timestamp (10 minutes ago)"
# Should return 400 Bad Request
send_webhook "100.00" "${RUN_ID}_nonce_3" "-600" "" "400"

# 7. Success Webhook deduction: -500.25 USDT
echo "7. Sending valid webhook: Deduct 500.25 USDT"
send_webhook "-500.250000000000000000" "${RUN_ID}_nonce_4" "0" "" "200"

# 8. Final balance verification (1500.50 - 500.25 = 1000.25 USDT)
echo "8. Checking final balance of user $USER_ID"
final_balance=$(curl -s "$SERVER_URL/balance/$USER_ID")
echo "   Response: $final_balance"

# Check math correctness
if [[ "$final_balance" == *'"USDT":"1000.25"'* ]]; then
    echo "====================================================================="
    echo " SUCCESS: All webhook and ledger calculations executed correctly!"
    echo "====================================================================="
else
    echo "====================================================================="
    echo " FAILURE: Balance did not match expected value of 1000.25 USDT!"
    echo "====================================================================="
    exit 1
fi
