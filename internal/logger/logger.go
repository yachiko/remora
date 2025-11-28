// Package logger provides structured logging functionality for the Remora
// reminder service using the zap logging library.
package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the global logger instance
var Logger *zap.Logger

// Initialize creates and configures the global logger based on environment
func Initialize(environment string, logLevel string) error {
	var config zap.Config

	// Configure based on environment
	if strings.ToLower(environment) == "production" {
		// Production: JSON format for machine parsing
		config = zap.NewProductionConfig()
	} else {
		// Development: Console format for human readability
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Set log level
	level, err := parseLogLevel(logLevel)
	if err != nil {
		return err
	}
	config.Level = zap.NewAtomicLevelAt(level)

	// Build logger
	logger, err := config.Build(
		zap.AddCallerSkip(1), // Skip one level to show actual caller
	)
	if err != nil {
		return err
	}

	Logger = logger
	return nil
}

// parseLogLevel converts string log level to zap.Level
func parseLogLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zap.DebugLevel, nil
	case "info":
		return zap.InfoLevel, nil
	case "warn", "warning":
		return zap.WarnLevel, nil
	case "error":
		return zap.ErrorLevel, nil
	case "fatal":
		return zap.FatalLevel, nil
	default:
		return zap.InfoLevel, nil // Default to INFO
	}
}

// Sync flushes any buffered log entries
func Sync() error {
	if Logger != nil {
		return Logger.Sync()
	}
	return nil
}

// Helper functions for structured logging

// Debug logs a debug message with structured fields
func Debug(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Debug(msg, fields...)
	}
}

// Info logs an info message with structured fields
func Info(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Info(msg, fields...)
	}
}

// Warn logs a warning message with structured fields
func Warn(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Warn(msg, fields...)
	}
}

// Error logs an error message with structured fields
func Error(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Error(msg, fields...)
	}
}

// Fatal logs a fatal message with structured fields and exits
func Fatal(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Fatal(msg, fields...)
	} else {
		// Fallback if logger not initialized
		os.Exit(1)
	}
}

// With creates a child logger with additional fields
func With(fields ...zap.Field) *zap.Logger {
	if Logger != nil {
		return Logger.With(fields...)
	}
	return zap.NewNop()
}

// Component returns a logger with component field set
func Component(name string) *zap.Logger {
	return With(zap.String("component", name))
}
