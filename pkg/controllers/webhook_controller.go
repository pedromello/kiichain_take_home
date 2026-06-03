package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"kiichain-assessment/pkg/models"
	"kiichain-assessment/pkg/views"

	"github.com/shopspring/decimal"
)

// WebhookRequest represents the expected JSON payload for the POST /webhook endpoint.
type WebhookRequest struct {
	User   string `json:"user"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

// WebhookController handles requests to update the ledger via signed webhooks.
type WebhookController struct {
	repo *models.LedgerRepository
}

// NewWebhookController creates a new instance of WebhookController.
func NewWebhookController(repo *models.LedgerRepository) *WebhookController {
	return &WebhookController{repo: repo}
}

// HandleWebhook processes incoming signed webhooks to adjust user balances.
func (c *WebhookController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	var req WebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		views.SendError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Validate and parse payload
	amount, err := c.filterInput(req)
	if err != nil {
		views.SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate and parse security headers
	xNonce, timestamp, err := validateHeaders(r)
	if err != nil {
		views.SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Create and persist the ledger entry
	entry := &models.LedgerEntry{
		UserID:    req.User,
		Asset:     req.Asset,
		Amount:    amount,
		Nonce:     xNonce,
		Timestamp: timestamp,
	}

	err = c.repo.CreateLedgerEntry(r.Context(), entry)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateNonce) {
			views.SendError(w, http.StatusConflict, err.Error())
			return
		}
		views.SendError(w, http.StatusInternalServerError, "failed to record transaction in the ledger")
		return
	}

	// Return success response
	views.SendJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// filterInput validates the request fields and parses the amount into a high-precision decimal.
// Returns the parsed amount and an error indicating the validation failure if any check fails.
func (c *WebhookController) filterInput(req WebhookRequest) (decimal.Decimal, error) {
	if req.User == "" {
		return decimal.Zero, errors.New("field 'user' is required and cannot be empty")
	}
	if req.Asset == "" {
		return decimal.Zero, errors.New("field 'asset' is required and cannot be empty")
	}
	if req.Amount == "" {
		return decimal.Zero, errors.New("field 'amount' is required and cannot be empty")
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return decimal.Zero, errors.New("invalid decimal format for field 'amount'")
	}

	return amount, nil
}

// validateHeaders extracts and validates the security headers X-Nonce and X-Timestamp from the request.
// Returns the nonce, the parsed timestamp, and any error encountered during validation or parsing.
func validateHeaders(r *http.Request) (string, time.Time, error) {
	xNonce := r.Header.Get("X-Nonce")
	xTimestamp := r.Header.Get("X-Timestamp")

	if xNonce == "" {
		return "", time.Time{}, errors.New("missing X-Nonce header")
	}
	if xTimestamp == "" {
		return "", time.Time{}, errors.New("missing X-Timestamp header")
	}

	unixSec, err := strconv.ParseInt(xTimestamp, 10, 64)
	if err != nil {
		return "", time.Time{}, errors.New("invalid X-Timestamp header format")
	}

	return xNonce, time.Unix(unixSec, 0), nil
}
