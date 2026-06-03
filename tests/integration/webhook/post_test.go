package webhook_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"kiichain-assessment/pkg/controllers"
	"kiichain-assessment/pkg/middleware"
	"kiichain-assessment/pkg/models"
	"kiichain-assessment/pkg/views"
	"kiichain-assessment/tests"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)

// computeHMAC computes the hex encoded HMAC-SHA256 signature for the request.
func computeHMAC(secret []byte, timestamp, nonce string, body []byte) string {
	payload := timestamp + "\n" + nonce + "\n" + string(body)
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func TestWebhookIntegration(t *testing.T) {
	orch := tests.NewOrchestrator(t)
	defer orch.Close()

	// Clear database to ensure test isolation
	orch.CleanDatabase(t)

	ledgerRepo := models.NewLedgerRepository(orch.DB)

	// Setup controllers and router
	webhookController := controllers.NewWebhookController(ledgerRepo)
	balanceRepo := models.NewBalanceRepository(orch.DB)
	balanceController := controllers.NewBalanceController(balanceRepo)

	r := chi.NewRouter()
	r.Use(middleware.HeadersMiddleware)
	r.With(middleware.AuthMiddleware(ledgerRepo, orch.Config.HMACSecret, orch.Config.ToleranceSeconds)).
		Post("/webhook", webhookController.HandleWebhook)
	r.Get("/balance/{user}", balanceController.HandleGetBalance)

	server := httptest.NewServer(r)
	defer server.Close()

	t.Run("Success - Create Ledger Entry and Upsert Balance", func(t *testing.T) {
		reqBody := controllers.WebhookRequest{
			User:   "user_alice",
			Asset:  "ETH",
			Amount: "1.500000000000000000",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		nonce := "nonce_1"
		timestampStr := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeHMAC(orch.Config.HMACSecret, timestampStr, nonce, bodyBytes)

		req, err := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", timestampStr)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", signature)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Assertions
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200 OK, got %d. Body: %s", resp.StatusCode, body)
		}

		// Verify custom X-Empty-Header response header
		if _, exists := resp.Header["X-Empty-Header"]; !exists {
			t.Errorf("Expected header 'X-Empty-Header' to be present in response")
		}

		// Verify balance was updated in database using the Orchestrator
		ethBalance, ok := orch.GetBalance(t, "user_alice", "ETH")
		if !ok {
			t.Fatalf("Expected ETH balance record to be present in database")
		}

		expectedBalance := decimal.RequireFromString("1.500000000000000000")
		if !ethBalance.Equal(expectedBalance) {
			t.Errorf("Expected ETH balance to be %s, got %s", expectedBalance, ethBalance)
		}
	})

	t.Run("Failure - Replay Attack (Duplicate Nonce)", func(t *testing.T) {
		reqBody := controllers.WebhookRequest{
			User:   "user_alice",
			Asset:  "ETH",
			Amount: "0.5",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		nonce := "nonce_reused"
		timestampStr := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeHMAC(orch.Config.HMACSecret, timestampStr, nonce, bodyBytes)

		// First request (should succeed)
		req1, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("X-Timestamp", timestampStr)
		req1.Header.Set("X-Nonce", nonce)
		req1.Header.Set("X-Signature", signature)

		resp1, err := http.DefaultClient.Do(req1)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		_ = resp1.Body.Close()

		if resp1.StatusCode != http.StatusOK {
			t.Fatalf("Expected first request to succeed, got %d", resp1.StatusCode)
		}

		// Second request with same nonce (should be rejected with 409 Conflict)
		req2, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Timestamp", timestampStr)
		req2.Header.Set("X-Nonce", nonce)
		req2.Header.Set("X-Signature", signature)

		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		defer func() { _ = resp2.Body.Close() }()

		if resp2.StatusCode != http.StatusConflict {
			t.Errorf("Expected 409 Conflict for duplicate nonce, got %d", resp2.StatusCode)
		}
	})

	t.Run("Failure - Invalid Signature", func(t *testing.T) {
		reqBody := controllers.WebhookRequest{
			User:   "user_alice",
			Asset:  "ETH",
			Amount: "1.0",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		nonce := "nonce_bad_sig"
		timestampStr := strconv.FormatInt(time.Now().Unix(), 10)
		badSignature := "invalid_signature_hex_code"

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", timestampStr)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", badSignature)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized for bad signature, got %d", resp.StatusCode)
		}
	})

	t.Run("Failure - Expired Timestamp", func(t *testing.T) {
		reqBody := controllers.WebhookRequest{
			User:   "user_alice",
			Asset:  "ETH",
			Amount: "1.0",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		nonce := "nonce_expired_time"
		// 10 minutes in the past (beyond 5 minutes default tolerance)
		expiredTimestampStr := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		signature := computeHMAC(orch.Config.HMACSecret, expiredTimestampStr, nonce, bodyBytes)

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", expiredTimestampStr)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", signature)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for expired timestamp, got %d", resp.StatusCode)
		}
	})

	t.Run("Success - Deduct Balance (Negative Amount)", func(t *testing.T) {
		reqBody := controllers.WebhookRequest{
			User:   "user_alice",
			Asset:  "ETH",
			Amount: "-0.500000000000000000",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		nonce := "nonce_deduct"
		timestampStr := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeHMAC(orch.Config.HMACSecret, timestampStr, nonce, bodyBytes)

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", timestampStr)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", signature)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
		}

		// Query updated balance using the Orchestrator
		ethBalance, ok := orch.GetBalance(t, "user_alice", "ETH")
		if !ok {
			t.Fatalf("Expected ETH balance record to be present in database")
		}

		expectedBalance := decimal.RequireFromString("1.500000000000000000")
		if !ethBalance.Equal(expectedBalance) {
			t.Errorf("Expected ETH balance to be updated to %s, got %s", expectedBalance, ethBalance)
		}
	})

	t.Run("Concurrency - Replay Protection (Same Nonce)", func(t *testing.T) {
		// Clean database for a fresh state
		orch.CleanDatabase(t)

		reqBody := controllers.WebhookRequest{
			User:   "user_concy_replay",
			Asset:  "USD",
			Amount: "100.00",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		nonce := "nonce_concurrent_replay"
		timestampStr := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeHMAC(orch.Config.HMACSecret, timestampStr, nonce, bodyBytes)

		const concurrentCount = 10
		var wg sync.WaitGroup
		statusCodes := make(chan int, concurrentCount)

		for i := 0; i < concurrentCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, err := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
				if err != nil {
					t.Errorf("Failed to create request: %v", err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", timestampStr)
				req.Header.Set("X-Nonce", nonce)
				req.Header.Set("X-Signature", signature)

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Errorf("Request failed: %v", err)
					return
				}
				_ = resp.Body.Close()
				statusCodes <- resp.StatusCode
			}()
		}

		wg.Wait()
		close(statusCodes)

		successCount := 0
		conflictCount := 0
		otherCount := 0

		for code := range statusCodes {
			switch code {
			case http.StatusOK:
				successCount++
			case http.StatusConflict:
				conflictCount++
			default:
				otherCount++
			}
		}

		// Exactly one request must succeed, and all others must fail with 409 Conflict
		if successCount != 1 {
			t.Errorf("Expected exactly 1 successful request, got %d", successCount)
		}
		if conflictCount != concurrentCount-1 {
			t.Errorf("Expected exactly %d duplicate nonce conflict requests, got %d", concurrentCount-1, conflictCount)
		}
		if otherCount > 0 {
			t.Errorf("Got unexpected HTTP status codes: %d responses", otherCount)
		}

		// Ensure the balance was only updated once (value should be exactly 100.00)
		bal, ok := orch.GetBalance(t, "user_concy_replay", "USD")
		if !ok {
			t.Fatalf("Expected balance to exist in database")
		}
		expectedBalance := decimal.RequireFromString("100.00")
		if !bal.Equal(expectedBalance) {
			t.Errorf("Expected balance to be %s, got %s", expectedBalance, bal)
		}
	})

	t.Run("Concurrency - Balance Accumulation (Different Nonces)", func(t *testing.T) {
		// Clean database
		orch.CleanDatabase(t)

		const concurrentCount = 50
		const amountPerReq = "0.1"
		userID := "user_concy_accum"
		asset := "BTC"

		var wg sync.WaitGroup
		statusCodes := make(chan int, concurrentCount)

		for i := 0; i < concurrentCount; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				reqBody := controllers.WebhookRequest{
					User:   userID,
					Asset:  asset,
					Amount: amountPerReq,
				}
				bodyBytes, _ := json.Marshal(reqBody)

				nonce := fmt.Sprintf("nonce_accum_%d", index)
				timestampStr := strconv.FormatInt(time.Now().Unix(), 10)
				signature := computeHMAC(orch.Config.HMACSecret, timestampStr, nonce, bodyBytes)

				req, err := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes))
				if err != nil {
					t.Errorf("Failed to create request: %v", err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", timestampStr)
				req.Header.Set("X-Nonce", nonce)
				req.Header.Set("X-Signature", signature)

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Errorf("Request failed: %v", err)
					return
				}
				_ = resp.Body.Close()
				statusCodes <- resp.StatusCode
			}(i)
		}

		wg.Wait()
		close(statusCodes)

		// Assert all requests return 200 OK
		successCount := 0
		for code := range statusCodes {
			if code == http.StatusOK {
				successCount++
			}
		}

		if successCount != concurrentCount {
			t.Errorf("Expected all %d requests to succeed, got %d successes", concurrentCount, successCount)
		}

		// Assert that the final balance is exactly equivalent to concurrentCount * amountPerReq
		bal, ok := orch.GetBalance(t, userID, asset)
		if !ok {
			t.Fatalf("Expected balance record to exist in database")
		}

		// Expected sum is 50 * 0.1 = 5.0
		expectedSum := decimal.NewFromFloat(float64(concurrentCount)).Mul(decimal.RequireFromString(amountPerReq))
		if !bal.Equal(expectedSum) {
			t.Errorf("Lost update or incorrect balance accumulation: expected %s, got %s", expectedSum, bal)
		}
	})

	t.Run("Precision - High-Precision Arithmetic and Value Preservation", func(t *testing.T) {
		// Clean database
		orch.CleanDatabase(t)

		userID := "user_precision_test"
		asset := "BTC"

		// 1. Send first deposit with 18 decimal places (very small amount)
		amount1 := "0.000000000000000001" // 1 wei-equivalent
		nonce1 := "nonce_precision_1"
		reqBody1 := controllers.WebhookRequest{
			User:   userID,
			Asset:  asset,
			Amount: amount1,
		}
		bodyBytes1, _ := json.Marshal(reqBody1)
		timestampStr1 := strconv.FormatInt(time.Now().Unix(), 10)
		sig1 := computeHMAC(orch.Config.HMACSecret, timestampStr1, nonce1, bodyBytes1)

		req1, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes1))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("X-Timestamp", timestampStr1)
		req1.Header.Set("X-Nonce", nonce1)
		req1.Header.Set("X-Signature", sig1)

		resp1, err := http.DefaultClient.Do(req1)
		if err != nil {
			t.Fatalf("Request 1 failed: %v", err)
		}
		_ = resp1.Body.Close()
		if resp1.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp1.StatusCode)
		}

		// 2. Send second deposit (another small amount)
		amount2 := "0.000000000000000009"
		nonce2 := "nonce_precision_2"
		reqBody2 := controllers.WebhookRequest{
			User:   userID,
			Asset:  asset,
			Amount: amount2,
		}
		bodyBytes2, _ := json.Marshal(reqBody2)
		timestampStr2 := strconv.FormatInt(time.Now().Unix(), 10)
		sig2 := computeHMAC(orch.Config.HMACSecret, timestampStr2, nonce2, bodyBytes2)

		req2, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes2))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Timestamp", timestampStr2)
		req2.Header.Set("X-Nonce", nonce2)
		req2.Header.Set("X-Signature", sig2)

		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Fatalf("Request 2 failed: %v", err)
		}
		_ = resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp2.StatusCode)
		}

		// 3. Send third deposit (a very large number to test combination of huge + tiny values)
		amount3 := "123456789012345678.000000000000000005"
		nonce3 := "nonce_precision_3"
		reqBody3 := controllers.WebhookRequest{
			User:   userID,
			Asset:  asset,
			Amount: amount3,
		}
		bodyBytes3, _ := json.Marshal(reqBody3)
		timestampStr3 := strconv.FormatInt(time.Now().Unix(), 10)
		sig3 := computeHMAC(orch.Config.HMACSecret, timestampStr3, nonce3, bodyBytes3)

		req3, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", bytes.NewReader(bodyBytes3))
		req3.Header.Set("Content-Type", "application/json")
		req3.Header.Set("X-Timestamp", timestampStr3)
		req3.Header.Set("X-Nonce", nonce3)
		req3.Header.Set("X-Signature", sig3)

		resp3, err := http.DefaultClient.Do(req3)
		if err != nil {
			t.Fatalf("Request 3 failed: %v", err)
		}
		_ = resp3.Body.Close()
		if resp3.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", resp3.StatusCode)
		}

		// Query the final balance using the GET /balance endpoint to verify view representation
		reqGet, _ := http.NewRequest(http.MethodGet, server.URL+"/balance/"+userID, nil)
		respGet, err := http.DefaultClient.Do(reqGet)
		if err != nil {
			t.Fatalf("Get request failed: %v", err)
		}
		defer func() { _ = respGet.Body.Close() }()

		if respGet.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK from GET balance, got %d", respGet.StatusCode)
		}

		bodyBytesGet, _ := io.ReadAll(respGet.Body)
		var balanceRes views.BalanceResponse
		if err := json.Unmarshal(bodyBytesGet, &balanceRes); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		btcVal, ok := balanceRes.Balances[asset]
		if !ok {
			t.Fatalf("Expected BTC balance to be present")
		}

		// Expected sum is:
		//   0.000000000000000001
		// + 0.000000000000000009
		// + 123456789012345678.000000000000000005
		// = 123456789012345678.000000000000000015
		// (Standard float64 would round/truncate this due to lack of precision, typically only has 15-17 significant digits)
		expectedSum := "123456789012345678.000000000000000015"
		if btcVal != expectedSum {
			t.Errorf("Precision lost during arithmetic aggregation: expected '%s', got '%s'", expectedSum, btcVal)
		}
	})
}
