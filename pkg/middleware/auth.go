// Package middleware provides HTTP middlewares for routing, logging, authorization, and custom header settings.
package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"kiichain-assessment/pkg/models"
	"kiichain-assessment/pkg/views"
)

// AuthMiddleware creates a middleware that validates incoming HTTP requests
// using HMAC-SHA256 signature verification and replay attack prevention (timestamp & nonce).
func AuthMiddleware(repo *models.LedgerRepository, secretKey []byte, tolerance time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			xTimestamp := r.Header.Get("X-Timestamp")
			xNonce := r.Header.Get("X-Nonce")
			xSignature := r.Header.Get("X-Signature")

			// Ensure all security headers are present
			if xTimestamp == "" || xNonce == "" || xSignature == "" {
				views.SendError(w, http.StatusBadRequest, "missing required security headers (X-Timestamp, X-Nonce, X-Signature)")
				return
			}

			// 1. Verify Timestamp (Replay Attack Prevention)
			unixSec, err := strconv.ParseInt(xTimestamp, 10, 64)
			if err != nil {
				views.SendError(w, http.StatusBadRequest, "invalid X-Timestamp format; must be a unix timestamp in seconds")
				return
			}

			timestamp := time.Unix(unixSec, 0)
			diff := time.Since(timestamp)
			if diff < 0 {
				diff = -diff
			}

			if diff > tolerance {
				views.SendError(w, http.StatusBadRequest, "request timestamp is outside the acceptable tolerance window")
				return
			}

			// 2. Verify Nonce (Replay Attack Prevention)
			exists, err := repo.NonceExists(r.Context(), xNonce)
			if err != nil {
				views.SendError(w, http.StatusInternalServerError, "internal server error: failed to verify request nonce")
				return
			}
			if exists {
				views.SendError(w, http.StatusConflict, "replay attack detected: nonce has already been used")
				return
			}

			// 3. Verify Signature
			// Read body to compute signature
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				views.SendError(w, http.StatusInternalServerError, "internal server error: failed to read request body")
				return
			}
			// Restore request body so controllers can decode the JSON payload
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// Format: request_body = X-Timestamp + "\n" + X-Nonce + "\n" + <raw_request_body_bytes_as_string>
			payload := xTimestamp + "\n" + xNonce + "\n" + string(bodyBytes)

			// Compute expected HMAC SHA256 hex signature
			h := hmac.New(sha256.New, secretKey)
			h.Write([]byte(payload))
			expectedSig := hex.EncodeToString(h.Sum(nil))

			// Secure constant-time comparison to prevent timing attacks
			if !hmac.Equal([]byte(expectedSig), []byte(xSignature)) {
				views.SendError(w, http.StatusUnauthorized, "invalid request signature")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
