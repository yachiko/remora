// Package main is the entry point for the Remora GitHub reminder bot service.
package main

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yachiko/remora/internal/api"
	"github.com/yachiko/remora/internal/config"
	"github.com/yachiko/remora/internal/database"
	"github.com/yachiko/remora/internal/github"
	"github.com/yachiko/remora/internal/logger"
	"github.com/yachiko/remora/internal/scheduler"
	"github.com/yachiko/remora/internal/webhook"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Version is set during build time
var Version = "dev"

// githubClientWrapper wraps github.Client to match scheduler.GitHubClient interface
type githubClientWrapper struct {
	client *github.Client
}

func (w *githubClientWrapper) PostComment(ctx context.Context, installationID int64, owner, repo string, issueNumber int, body string) error {
	_, err := w.client.PostComment(ctx, installationID, owner, repo, issueNumber, body)
	return err
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Initialize("production", cfg.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Logger.Sync()
	}()

	logger.Info("starting Remora",
		zap.String("version", Version),
		zap.String("log_level", cfg.LogLevel))

	// Initialize database
	if err := database.Initialize(cfg); err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}

	logger.Info("database initialized",
		zap.String("type", cfg.DatabaseType))

	// Initialize repository
	repo := database.NewReminderRepository(database.DB)

	// Parse GitHub App private key
	privateKey, err := parsePrivateKey(cfg.GitHubAppPrivateKey)
	if err != nil {
		logger.Fatal("failed to parse GitHub App private key", zap.Error(err))
	}

	// Initialize GitHub client
	githubClient := github.NewClient(cfg.GitHubAppID, privateKey, logger.Logger)

	// Initialize webhook handler
	webhookHandler := webhook.NewHandler(
		cfg,
		repo,
		githubClient,
		logger.Logger,
	)

	// Initialize scheduler with wrapped GitHub client
	schedulerCfg := &scheduler.Config{
		Interval:   time.Duration(cfg.SchedulerInterval) * time.Minute,
		MaxRetries: 5,
	}
	githubWrapper := &githubClientWrapper{client: githubClient}
	sched := scheduler.New(repo, githubWrapper, logger.Logger, schedulerCfg)

	// Create HTTP server
	mux := http.NewServeMux()

	// Webhook endpoint
	mux.Handle("/webhook", webhookHandler)

	// Health check endpoint
	mux.HandleFunc("/health", healthHandler(database.DB, logger.Logger))

	// Readiness endpoint
	mux.HandleFunc("/ready", readinessHandler(database.DB, sched, logger.Logger))

	// Admin API endpoints (if enabled)
	if cfg.EnableAPI {
		if cfg.APISecret == "" {
			logger.Fatal("API is enabled but API secret is not configured")
		}

		apiHandler := api.NewHandler(repo, logger.Logger)

		// Register admin endpoints with authentication
		mux.HandleFunc("/api/v1/reminders", api.AuthMiddleware(cfg.APISecret, apiHandler.ListReminders))

		logger.Info("admin API enabled",
			zap.String("endpoint", "/api/v1/reminders"))
	}

	// Wrap mux with middlewares
	handler := api.RequestIDMiddleware(loggingMiddleware(mux, logger.Logger))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start scheduler
	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		logger.Fatal("failed to start scheduler", zap.Error(err))
	}

	logger.Info("scheduler started",
		zap.Duration("interval", schedulerCfg.Interval))

	// Start HTTP server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting",
			zap.Int("port", cfg.Port))
		serverErrors <- server.ListenAndServe()
	}()

	// Wait for interrupt signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal or server error
	select {
	case err := <-serverErrors:
		logger.Fatal("server error", zap.Error(err))
	case sig := <-shutdown:
		logger.Info("shutdown signal received",
			zap.String("signal", sig.String()))

		// Create shutdown context with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Stop scheduler gracefully
		logger.Info("stopping scheduler")
		if err := sched.Stop(); err != nil {
			logger.Error("error stopping scheduler", zap.Error(err))
		}

		// Shutdown HTTP server gracefully
		logger.Info("stopping HTTP server")
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("error during server shutdown", zap.Error(err))
			// Force close
			_ = server.Close()
		}

		logger.Info("shutdown complete")
	}
}

// healthHandler returns a health check handler
func healthHandler(db *gorm.DB, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check database connectivity
		sqlDB, err := db.DB()
		if err != nil {
			log.Error("health check: failed to get database instance", zap.Error(err))
			writeJSONError(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}

		if err := sqlDB.Ping(); err != nil {
			log.Error("health check: database ping failed", zap.Error(err))
			writeJSONError(w, "database unhealthy", http.StatusServiceUnavailable)
			return
		}

		// Return healthy response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "healthy",
			"version":   Version,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// readinessHandler returns a readiness check handler
func readinessHandler(db *gorm.DB, _ *scheduler.Scheduler, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check database connectivity
		sqlDB, err := db.DB()
		if err != nil {
			log.Error("readiness check: failed to get database instance", zap.Error(err))
			writeJSONError(w, "not ready: database error", http.StatusServiceUnavailable)
			return
		}

		if err := sqlDB.Ping(); err != nil {
			log.Error("readiness check: database ping failed", zap.Error(err))
			writeJSONError(w, "not ready: database unhealthy", http.StatusServiceUnavailable)
			return
		}

		// Return ready response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ready",
			"version":   Version,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// loggingMiddleware logs all HTTP requests
func loggingMiddleware(next http.Handler, log *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log request
		log.Info("HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
			zap.Int("status", wrapped.statusCode),
			zap.Duration("duration", time.Since(start)),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":     message,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// parsePrivateKey parses a PEM-encoded RSA private key
func parsePrivateKey(keyPEM string) (*rsa.PrivateKey, error) {
	return jwt.ParseRSAPrivateKeyFromPEM([]byte(keyPEM))
}
