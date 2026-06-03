package balance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kiichain-assessment/pkg/controllers"
	"kiichain-assessment/pkg/middleware"
	"kiichain-assessment/pkg/models"
	"kiichain-assessment/pkg/views"
	"kiichain-assessment/tests"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)

func TestGetBalanceIntegration(t *testing.T) {
	orch := tests.NewOrchestrator(t)
	defer orch.Close()

	// Clear database to ensure test isolation
	orch.CleanDatabase(t)

	balanceRepo := models.NewBalanceRepository(orch.DB)

	// Setup controller and router
	balanceController := controllers.NewBalanceController(balanceRepo)

	r := chi.NewRouter()
	r.Use(middleware.HeadersMiddleware)
	r.Get("/balance/{user}", balanceController.HandleGetBalance)

	server := httptest.NewServer(r)
	defer server.Close()

	t.Run("Success - Retrieve Seeded Balances", func(t *testing.T) {
		// Seed balances using the Orchestrator
		orch.SeedBalance(t, "user_bob", "USD", "250.75")
		orch.SeedBalance(t, "user_bob", "BTC", "0.004812000000000000")

		req, err := http.NewRequest(http.MethodGet, server.URL+"/balance/user_bob", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}

		// Verify custom X-Empty-Header response header is present
		if _, exists := resp.Header["X-Empty-Header"]; !exists {
			t.Errorf("Expected header 'X-Empty-Header' to be present in response")
		}

		// Read and parse response body
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		var balanceRes views.BalanceResponse
		if err := json.Unmarshal(bodyBytes, &balanceRes); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Assertions
		if balanceRes.User != "user_bob" {
			t.Errorf("Expected user 'user_bob', got '%s'", balanceRes.User)
		}

		usdVal, ok := balanceRes.Balances["USD"]
		if !ok {
			t.Errorf("Expected USD balance to be present")
		}
		usdDec, _ := decimal.NewFromString(usdVal)
		expectedUSD := decimal.NewFromFloat(250.75)
		if !usdDec.Equal(expectedUSD) {
			t.Errorf("Expected USD balance to be %s, got %s", expectedUSD, usdVal)
		}

		btcVal, ok := balanceRes.Balances["BTC"]
		if !ok {
			t.Errorf("Expected BTC balance to be present")
		}
		btcDec, _ := decimal.NewFromString(btcVal)
		expectedBTC := decimal.RequireFromString("0.004812000000000000")
		if !btcDec.Equal(expectedBTC) {
			t.Errorf("Expected BTC balance to be %s, got %s", expectedBTC, btcVal)
		}
	})

	t.Run("Success - Non-existent User Returns Empty Balances Object", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/balance/user_charlie", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		// Verify empty map formats to JSON {}
		expectedJSON := `{"user":"user_charlie","balances":{}}`
		trimmedBody := string(bytesTrimRight(bodyBytes))
		if trimmedBody != expectedJSON {
			t.Errorf("Expected body to be '%s', got '%s'", expectedJSON, trimmedBody)
		}
	})
}

// bytesTrimRight removes trailing newlines or whitespace from json bytes
func bytesTrimRight(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}
