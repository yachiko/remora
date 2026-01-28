// Package config provides configuration loading and validation for the Remora
// reminder service from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Database type constants for supported database drivers.
const (
	DatabaseTypePostgres = "postgresql"
	DatabaseTypeMySQL    = "mysql"
	DatabaseTypeSQLite   = "sqlite"
)

// Config holds all application configuration
type Config struct {
	// Database configuration
	DatabaseType       string
	DatabaseHost       string
	DatabasePort       int
	DatabaseName       string
	DatabaseUser       string
	DatabasePassword   string
	DatabaseSSLMode    string // For PostgreSQL: disable, require, verify-ca, verify-full
	DatabaseSQLitePath string // File path for SQLite database

	// Database connection pool
	DatabaseMaxOpenConns    int
	DatabaseMaxIdleConns    int
	DatabaseConnMaxLifetime int // seconds

	// HTTP server configuration
	Port        int
	WebhookPath string
	HealthPath  string
	ReadyPath   string

	// GitHub App configuration
	GitHubAppID         int64
	GitHubAppPrivateKey string
	GitHubWebhookSecret string

	// Scheduler configuration
	SchedulerInterval int // Minutes

	// Error handling
	ErrorMode string // "reaction_only" or "reaction_and_comment"

	// Reminder behavior
	PostToClosed bool

	// Admin API (optional)
	EnableAPI bool
	APISecret string

	// Rate limiting
	RateLimit int // Requests per minute

	// Logging
	LogLevel string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseType:       getEnv("DATABASE_TYPE", "sqlite"),
		DatabaseHost:       getEnv("DATABASE_HOST", ""),
		DatabasePort:       getEnvAsInt("DATABASE_PORT", 0),
		DatabaseName:       getEnv("DATABASE_NAME", ""),
		DatabaseUser:       getEnv("DATABASE_USER", ""),
		DatabasePassword:   getEnv("DATABASE_PASSWORD", ""),
		DatabaseSSLMode:    getEnv("DATABASE_SSLMODE", "disable"),
		DatabaseSQLitePath: getEnv("DATABASE_SQLITE_PATH", "./data/remora.db"),

		DatabaseMaxOpenConns:    getEnvAsInt("DATABASE_MAX_OPEN_CONNS", 25),
		DatabaseMaxIdleConns:    getEnvAsInt("DATABASE_MAX_IDLE_CONNS", 5),
		DatabaseConnMaxLifetime: getEnvAsInt("DATABASE_CONN_MAX_LIFETIME", 300),

		Port:        getEnvAsInt("REMORA_PORT", 8080),
		WebhookPath: getEnv("REMORA_WEBHOOK_PATH", "/webhook"),
		HealthPath:  getEnv("REMORA_HEALTH_PATH", "/health"),
		ReadyPath:   getEnv("REMORA_READY_PATH", "/ready"),

		GitHubAppID:         getEnvAsInt64("GITHUB_APP_ID", 0),
		GitHubAppPrivateKey: getEnv("GITHUB_APP_PRIVATE_KEY", ""),
		GitHubWebhookSecret: getEnv("GITHUB_WEBHOOK_SECRET", ""),

		SchedulerInterval: getEnvAsInt("REMORA_SCHEDULER_INTERVAL", 5),

		ErrorMode: getEnv("REMORA_ERROR_MODE", "reaction_only"),

		PostToClosed: getEnvAsBool("REMORA_POST_TO_CLOSED", true),

		EnableAPI: getEnvAsBool("REMORA_ENABLE_API", false),
		APISecret: getEnv("REMORA_API_SECRET", ""),

		RateLimit: getEnvAsInt("REMORA_RATE_LIMIT", 60),

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	var errs []error

	// Database validation
	if c.DatabaseType == "" {
		errs = append(errs, errors.New("DATABASE_TYPE is required"))
	}
	if !isValidDatabaseType(c.DatabaseType) {
		errs = append(errs, fmt.Errorf("DATABASE_TYPE must be one of: postgresql, mysql, sqlite (got: %s)", c.DatabaseType))
	}

	// For PostgreSQL and MySQL, validate required fields
	if c.DatabaseType == DatabaseTypePostgres || c.DatabaseType == DatabaseTypeMySQL {
		if c.DatabaseHost == "" {
			errs = append(errs, errors.New("DATABASE_HOST is required for postgresql/mysql"))
		}
		if c.DatabaseName == "" {
			errs = append(errs, errors.New("DATABASE_NAME is required for postgresql/mysql"))
		}
		if c.DatabaseUser == "" {
			errs = append(errs, errors.New("DATABASE_USER is required for postgresql/mysql"))
		}
		if c.DatabasePort == 0 {
			// Set default port if not specified
			if c.DatabaseType == DatabaseTypePostgres {
				c.DatabasePort = 5432
			} else {
				c.DatabasePort = 3306
			}
		}
		if c.DatabasePort < 1 || c.DatabasePort > 65535 {
			errs = append(errs, fmt.Errorf("DATABASE_PORT must be between 1 and 65535 (got: %d)", c.DatabasePort))
		}
	}

	// GitHub App validation
	if c.GitHubAppID == 0 {
		errs = append(errs, errors.New("GITHUB_APP_ID is required"))
	}
	if c.GitHubAppPrivateKey == "" {
		errs = append(errs, errors.New("GITHUB_APP_PRIVATE_KEY is required"))
	}
	if c.GitHubWebhookSecret == "" {
		errs = append(errs, errors.New("GITHUB_WEBHOOK_SECRET is required"))
	}

	// Port validation
	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, fmt.Errorf("REMORA_PORT must be between 1 and 65535 (got: %d)", c.Port))
	}

	// Scheduler validation
	if c.SchedulerInterval < 1 {
		errs = append(errs, fmt.Errorf("REMORA_SCHEDULER_INTERVAL must be at least 1 minute (got: %d)", c.SchedulerInterval))
	}

	// Error mode validation
	if !isValidErrorMode(c.ErrorMode) {
		errs = append(errs, fmt.Errorf("REMORA_ERROR_MODE must be 'reaction_only' or 'reaction_and_comment' (got: %s)", c.ErrorMode))
	}

	// Admin API validation
	if c.EnableAPI && c.APISecret == "" {
		errs = append(errs, errors.New("REMORA_API_SECRET is required when REMORA_ENABLE_API is true"))
	}

	// Rate limit validation
	if c.RateLimit < 1 {
		errs = append(errs, fmt.Errorf("REMORA_RATE_LIMIT must be at least 1 (got: %d)", c.RateLimit))
	}

	// Log level validation
	if !isValidLogLevel(c.LogLevel) {
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error, fatal (got: %s)", c.LogLevel))
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// ValidationError represents multiple validation errors
type ValidationError struct {
	Errors []error
}

func (e *ValidationError) Error() string {
	var msgs []string
	for _, err := range e.Errors {
		msgs = append(msgs, err.Error())
	}
	return "configuration validation failed: " + strings.Join(msgs, "; ")
}

// DatabaseURL constructs the database connection URL from individual components
func (c *Config) DatabaseURL() string {
	switch c.DatabaseType {
	case DatabaseTypePostgres:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			c.DatabaseHost, c.DatabasePort, c.DatabaseUser, c.DatabasePassword, c.DatabaseName, c.DatabaseSSLMode)
	case DatabaseTypeMySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			c.DatabaseUser, c.DatabasePassword, c.DatabaseHost, c.DatabasePort, c.DatabaseName)
	case DatabaseTypeSQLite:
		// For SQLite, use DatabaseSQLitePath
		if c.DatabaseSQLitePath == "" {
			return "./data/remora.db"
		}
		return c.DatabaseSQLitePath
	default:
		return ""
	}
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func isValidDatabaseType(dbType string) bool {
	validTypes := []string{DatabaseTypePostgres, DatabaseTypeMySQL, DatabaseTypeSQLite}
	for _, t := range validTypes {
		if dbType == t {
			return true
		}
	}
	return false
}

func isValidErrorMode(mode string) bool {
	return mode == "reaction_only" || mode == "reaction_and_comment"
}

func isValidLogLevel(level string) bool {
	validLevels := []string{"debug", "info", "warn", "error", "fatal"}
	for _, l := range validLevels {
		if level == l {
			return true
		}
	}
	return false
}
