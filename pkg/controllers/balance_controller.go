// Package controllers implements the request handlers for the application endpoints.
package controllers

import (
	"errors"
	"net/http"

	"kiichain-assessment/pkg/models"
	"kiichain-assessment/pkg/views"

	"github.com/go-chi/chi/v5"
)

// BalanceController handles requests to retrieve user consolidated balances.
type BalanceController struct {
	repo *models.BalanceRepository
}

// NewBalanceController creates a new instance of BalanceController.
func NewBalanceController(repo *models.BalanceRepository) *BalanceController {
	return &BalanceController{repo: repo}
}

// HandleGetBalance retrieves the consolidated balances of a specific user.
func (c *BalanceController) HandleGetBalance(w http.ResponseWriter, r *http.Request) {
	userID, err := c.filterInput(r)
	if err != nil {
		views.SendError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve consolidated balances from the database
	balances, err := c.repo.GetUserBalances(r.Context(), userID)
	if err != nil {
		views.SendError(w, http.StatusInternalServerError, "failed to query user balances")
		return
	}

	// Format response to conform to the specified View format (balances represented as strings)
	response := views.NewBalanceResponse(userID, balances)
	views.SendJSON(w, http.StatusOK, response)
}

// filterInput extracts and validates the request parameters for querying balances.
// Returns the validated userID, or an error if validation fails.
func (c *BalanceController) filterInput(r *http.Request) (string, error) {
	userID := chi.URLParam(r, "user")
	if userID == "" {
		return "", errors.New("parameter 'user' is required and cannot be empty")
	}
	return userID, nil
}
