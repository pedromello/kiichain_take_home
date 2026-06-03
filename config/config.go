package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all the configuration parameters for the application.
type Config struct {
	DBHost           string
	DBPort           int
	DBUser           string
	DBPassword       string
	DBName           string
	DBSSLMode        string
	ServerPort       string
	HMACSecret       []byte
	ToleranceSeconds time.Duration
}

// LoadConfig reads the environment variables and returns a validated Config struct.
func LoadConfig() (*Config, error) {
	// Server configuration
	port := getEnv("PORT", "8080")

	// HMAC Signature security
	hmacSecretStr := os.Getenv("HMAC_SECRET")
	if hmacSecretStr == "" {
		return nil, errors.New("HMAC_SECRET environment variable is required")
	}

	// Replay tolerance in minutes (default to 5 minutes)
	toleranceMinStr := getEnv("TOLERANCE_MINUTES", "5")
	toleranceMin, err := strconv.Atoi(toleranceMinStr)
	if err != nil || toleranceMin <= 0 {
		return nil, fmt.Errorf("invalid TOLERANCE_MINUTES: %s, must be a positive integer", toleranceMinStr)
	}
	toleranceDuration := time.Duration(toleranceMin) * time.Minute

	// Database configurations
	dbHost := getEnv("DB_HOST", "localhost")
	dbPortStr := getEnv("DB_PORT", "5432")
	dbPort, err := strconv.Atoi(dbPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %s", dbPortStr)
	}

	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "ledger")
	dbSSLMode := getEnv("DB_SSLMODE", "disable")

	return &Config{
		DBHost:           dbHost,
		DBPort:           dbPort,
		DBUser:           dbUser,
		DBPassword:       dbPassword,
		DBName:           dbName,
		DBSSLMode:        dbSSLMode,
		ServerPort:       port,
		HMACSecret:       []byte(hmacSecretStr),
		ToleranceSeconds: toleranceDuration,
	}, nil
}

// getEnv retrieves an environment variable or returns a fallback default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
