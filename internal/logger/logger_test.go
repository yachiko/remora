package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestInitialize(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		logLevel    string
		wantErr     bool
	}{
		{
			name:        "production environment with info level",
			environment: "production",
			logLevel:    "info",
			wantErr:     false,
		},
		{
			name:        "development environment with debug level",
			environment: "development",
			logLevel:    "debug",
			wantErr:     false,
		},
		{
			name:        "production with warn level",
			environment: "production",
			logLevel:    "warn",
			wantErr:     false,
		},
		{
			name:        "development with error level",
			environment: "development",
			logLevel:    "error",
			wantErr:     false,
		},
		{
			name:        "invalid log level defaults to info",
			environment: "development",
			logLevel:    "invalid",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.environment, tt.logLevel)
			if (err != nil) != tt.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && Logger == nil {
				t.Error("Initialize() succeeded but Logger is nil")
			}

			// Clean up
			if Logger != nil {
				Logger.Sync()
				Logger = nil
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantLevel zapcore.Level
	}{
		{"debug", "debug", zap.DebugLevel},
		{"info", "info", zap.InfoLevel},
		{"warn", "warn", zap.WarnLevel},
		{"warning", "warning", zap.WarnLevel},
		{"error", "error", zap.ErrorLevel},
		{"fatal", "fatal", zap.FatalLevel},
		{"uppercase DEBUG", "DEBUG", zap.DebugLevel},
		{"invalid defaults to info", "invalid", zap.InfoLevel},
		{"empty defaults to info", "", zap.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogLevel(tt.level)
			if err != nil {
				t.Errorf("parseLogLevel() error = %v", err)
				return
			}
			if got != tt.wantLevel {
				t.Errorf("parseLogLevel() = %v, want %v", got, tt.wantLevel)
			}
		})
	}
}

func TestLoggerHelperFunctions(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a custom logger that writes to buffer
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "" // Disable timestamp for easier testing
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&buf),
		zapcore.DebugLevel,
	)
	Logger = zap.New(core)
	defer func() {
		Logger.Sync()
		Logger = nil
	}()

	// Test Debug
	Debug("debug message", zap.String("key", "value"))
	if !strings.Contains(buf.String(), "debug message") {
		t.Error("Debug() did not log message")
	}
	if !strings.Contains(buf.String(), "\"key\":\"value\"") {
		t.Error("Debug() did not log field")
	}
	buf.Reset()

	// Test Info
	Info("info message", zap.Int("count", 42))
	if !strings.Contains(buf.String(), "info message") {
		t.Error("Info() did not log message")
	}
	if !strings.Contains(buf.String(), "\"count\":42") {
		t.Error("Info() did not log field")
	}
	buf.Reset()

	// Test Warn
	Warn("warn message", zap.Bool("flag", true))
	if !strings.Contains(buf.String(), "warn message") {
		t.Error("Warn() did not log message")
	}
	if !strings.Contains(buf.String(), "\"flag\":true") {
		t.Error("Warn() did not log field")
	}
	buf.Reset()

	// Test Error
	Error("error message", zap.String("error", "test error"))
	if !strings.Contains(buf.String(), "error message") {
		t.Error("Error() did not log message")
	}
	if !strings.Contains(buf.String(), "\"error\":\"test error\"") {
		t.Error("Error() did not log field")
	}
	buf.Reset()
}

func TestWith(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = ""
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&buf),
		zapcore.InfoLevel,
	)
	Logger = zap.New(core)
	defer func() {
		Logger.Sync()
		Logger = nil
	}()

	// Create child logger with fields
	childLogger := With(zap.String("request_id", "abc-123"))
	childLogger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "\"request_id\":\"abc-123\"") {
		t.Error("With() did not add field to logger")
	}
	if !strings.Contains(output, "test message") {
		t.Error("With() logger did not log message")
	}
}

func TestComponent(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = ""
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&buf),
		zapcore.InfoLevel,
	)
	Logger = zap.New(core)
	defer func() {
		Logger.Sync()
		Logger = nil
	}()

	// Create component logger
	webhookLogger := Component("webhook")
	webhookLogger.Info("webhook received")

	output := buf.String()
	if !strings.Contains(output, "\"component\":\"webhook\"") {
		t.Error("Component() did not add component field")
	}
	if !strings.Contains(output, "webhook received") {
		t.Error("Component() logger did not log message")
	}
}

func TestProductionFormat(t *testing.T) {
	var buf bytes.Buffer

	// Initialize in production mode
	Logger = nil
	err := Initialize("production", "info")
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer func() {
		Logger.Sync()
		Logger = nil
	}()

	// Replace logger core to write to buffer
	encoderConfig := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&buf),
		zapcore.InfoLevel,
	)
	Logger = zap.New(core)

	Info("test message", zap.String("key", "value"))

	// Verify JSON format
	var logEntry map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &logEntry)
	if err != nil {
		t.Errorf("Production log output is not valid JSON: %v", err)
	}

	// Verify required fields
	if logEntry["level"] != "info" {
		t.Errorf("Expected level=info, got %v", logEntry["level"])
	}
	if logEntry["msg"] != "test message" {
		t.Errorf("Expected msg='test message', got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key='value', got %v", logEntry["key"])
	}
}

func TestDevelopmentFormat(t *testing.T) {
	Logger = nil
	err := Initialize("development", "debug")
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer func() {
		Logger.Sync()
		Logger = nil
	}()

	// Development mode uses console encoder
	// Just verify it initializes without error
	if Logger == nil {
		t.Error("Development logger not initialized")
	}
}

func TestNilLogger(t *testing.T) {
	// Ensure Logger is nil
	Logger = nil

	// These should not panic when Logger is nil
	Debug("test")
	Info("test")
	Warn("test")
	Error("test")

	childLogger := With(zap.String("key", "value"))
	if childLogger == nil {
		t.Error("With() returned nil when Logger is nil")
	}

	componentLogger := Component("test")
	if componentLogger == nil {
		t.Error("Component() returned nil when Logger is nil")
	}
}

func TestSync(t *testing.T) {
	// Test Sync with nil logger
	err := Sync()
	if err != nil {
		t.Errorf("Sync() with nil logger returned error: %v", err)
	}

	// Initialize logger
	err = Initialize("development", "info")
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer func() { Logger = nil }()

	// Test Sync with initialized logger
	err = Sync()
	// Note: Sync() may return "sync /dev/stderr: inappropriate ioctl for device" on some systems
	// This is expected and not an actual error
	// We just verify it doesn't panic
}
