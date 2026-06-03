package views

import (
	"encoding/json"
	"net/http"

	"github.com/shopspring/decimal"
)

// ErrorResponse represents a standardized JSON error message payload.
type ErrorResponse struct {
	Error string `json:"error"`
}

// BalanceResponse represents the JSON output for the GET /balance/{user} endpoint.
type BalanceResponse struct {
	User     string            `json:"user"`
	Balances map[string]string `json:"balances"`
}

// NewBalanceResponse maps the internal models representation of balances (decimal.Decimal)
// to the external view representation where monetary values are represented as strings.
func NewBalanceResponse(userID string, balances map[string]decimal.Decimal) BalanceResponse {
	res := BalanceResponse{
		User:     userID,
		Balances: make(map[string]string),
	}
	for asset, val := range balances {
		res.Balances[asset] = val.String()
	}
	return res
}

// SendJSON encodes the data struct to JSON and writes it to the ResponseWriter.
func SendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Log the error or handle it (in a helper we write standard response)
		// Since we cannot write headers again, we just encode what we can.
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Internal Server Error"})
	}
}

// SendError sends a formatted JSON error response with the given HTTP status code.
func SendError(w http.ResponseWriter, status int, message string) {
	SendJSON(w, status, ErrorResponse{Error: message})
}
