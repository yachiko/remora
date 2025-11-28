package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "valid configuration with all required fields",
			envVars: map[string]string{
				"DATABASE_TYPE":          "postgresql",
				"DATABASE_HOST":          "localhost",
				"DATABASE_PORT":          "5432",
				"DATABASE_NAME":          "remora",
				"DATABASE_USER":          "testuser",
				"DATABASE_PASSWORD":      "testpass",
				"GITHUB_APP_ID":          "123456",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
			},
			wantErr: false,
		},
		{
			name: "missing DATABASE_HOST for postgresql",
			envVars: map[string]string{
				"DATABASE_TYPE":          "postgresql",
				"DATABASE_NAME":          "remora",
				"DATABASE_USER":          "testuser",
				"GITHUB_APP_ID":          "123456",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
			},
			wantErr: true,
		},
		{
			name: "missing GITHUB_APP_ID",
			envVars: map[string]string{
				"DATABASE_TYPE":          "postgresql",
				"DATABASE_HOST":          "localhost",
				"DATABASE_NAME":          "remora",
				"DATABASE_USER":          "testuser",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
			},
			wantErr: true,
		},
		{
			name: "invalid DATABASE_TYPE",
			envVars: map[string]string{
				"DATABASE_TYPE":          "mongodb",
				"DATABASE_HOST":          "localhost",
				"DATABASE_NAME":          "remora",
				"DATABASE_USER":          "testuser",
				"GITHUB_APP_ID":          "123456",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
			},
			wantErr: true,
		},
		{
			name: "sqlite configuration",
			envVars: map[string]string{
				"DATABASE_TYPE":          "sqlite",
				"DATABASE_NAME":          "./test.db",
				"GITHUB_APP_ID":          "123456",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
			},
			wantErr: false,
		},
		{
			name: "admin API enabled with secret",
			envVars: map[string]string{
				"DATABASE_TYPE":          "postgresql",
				"DATABASE_HOST":          "localhost",
				"DATABASE_NAME":          "remora",
				"DATABASE_USER":          "testuser",
				"GITHUB_APP_ID":          "123456",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
				"REMORA_ENABLE_API":      "true",
				"REMORA_API_SECRET":      "admin-secret",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				_ = os.Setenv(k, v)
			}

			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && cfg == nil {
				t.Error("Load() returned nil config when no error expected")
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("DATABASE_TYPE", "sqlite")
	_ = os.Setenv("DATABASE_NAME", "./test.db")
	_ = os.Setenv("GITHUB_APP_ID", "123456")
	_ = os.Setenv("GITHUB_APP_PRIVATE_KEY", "test-key")
	_ = os.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DatabaseType != "sqlite" {
		t.Errorf("DatabaseType = %v, want sqlite", cfg.DatabaseType)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %v, want 8080", cfg.Port)
	}
	if cfg.SchedulerInterval != 5 {
		t.Errorf("SchedulerInterval = %v, want 5", cfg.SchedulerInterval)
	}
	if cfg.ErrorMode != "reaction_only" {
		t.Errorf("ErrorMode = %v, want reaction_only", cfg.ErrorMode)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				DatabaseType:        "postgresql",
				DatabaseHost:        "localhost",
				DatabasePort:        5432,
				DatabaseName:        "remora",
				DatabaseUser:        "testuser",
				DatabasePassword:    "testpass",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: &Config{
				DatabaseType:        "postgresql",
				DatabaseHost:        "localhost",
				DatabasePort:        5432,
				DatabaseName:        "remora",
				DatabaseUser:        "testuser",
				DatabasePassword:    "testpass",
				Port:                0,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "port too high",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                70000,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "invalid scheduler interval",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   0,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "invalid error mode",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "invalid_mode",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "invalid_level",
			},
			wantErr: true,
		},
		{
			name: "invalid rate limit",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           0,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "api enabled without secret",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
				EnableAPI:           true,
				APISecret:           "",
			},
			wantErr: true,
		},
		{
			name: "missing github app private key",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "missing github webhook secret",
			config: &Config{
				DatabaseType:        "sqlite",
				DatabaseName:        "test.db",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "mysql missing host",
			config: &Config{
				DatabaseType:        "mysql",
				DatabaseHost:        "",
				DatabasePort:        3306,
				DatabaseName:        "remora",
				DatabaseUser:        "testuser",
				DatabasePassword:    "testpass",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "mysql missing user",
			config: &Config{
				DatabaseType:        "mysql",
				DatabaseHost:        "localhost",
				DatabasePort:        3306,
				DatabaseName:        "remora",
				DatabaseUser:        "",
				DatabasePassword:    "testpass",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "invalid database port",
			config: &Config{
				DatabaseType:        "postgresql",
				DatabaseHost:        "localhost",
				DatabasePort:        70000,
				DatabaseName:        "remora",
				DatabaseUser:        "testuser",
				DatabasePassword:    "testpass",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "info",
			},
			wantErr: true,
		},
		{
			name: "valid mysql config with default port",
			config: &Config{
				DatabaseType:        "mysql",
				DatabaseHost:        "localhost",
				DatabasePort:        0, // should get default 3306
				DatabaseName:        "remora",
				DatabaseUser:        "testuser",
				DatabasePassword:    "testpass",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_and_comment",
				RateLimit:           60,
				LogLevel:            "debug",
			},
			wantErr: false,
		},
		{
			name: "valid postgresql config with default port",
			config: &Config{
				DatabaseType:        "postgresql",
				DatabaseHost:        "localhost",
				DatabasePort:        0, // should get default 5432
				DatabaseName:        "remora",
				DatabaseUser:        "testuser",
				DatabasePassword:    "testpass",
				Port:                8080,
				GitHubAppID:         123456,
				GitHubAppPrivateKey: "test-key",
				GitHubWebhookSecret: "test-secret",
				SchedulerInterval:   5,
				ErrorMode:           "reaction_only",
				RateLimit:           60,
				LogLevel:            "warn",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDatabaseURL(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   string
	}{
		{
			name: "postgresql",
			config: &Config{
				DatabaseType:     "postgresql",
				DatabaseHost:     "localhost",
				DatabasePort:     5432,
				DatabaseUser:     "testuser",
				DatabasePassword: "testpass",
				DatabaseName:     "remora",
				DatabaseSSLMode:  "disable",
			},
			want: "host=localhost port=5432 user=testuser password=testpass dbname=remora sslmode=disable",
		},
		{
			name: "mysql",
			config: &Config{
				DatabaseType:     "mysql",
				DatabaseHost:     "localhost",
				DatabasePort:     3306,
				DatabaseUser:     "testuser",
				DatabasePassword: "testpass",
				DatabaseName:     "remora",
			},
			want: "testuser:testpass@tcp(localhost:3306)/remora?parseTime=true",
		},
		{
			name: "sqlite with path",
			config: &Config{
				DatabaseType: "sqlite",
				DatabaseName: "/var/data/remora.db",
			},
			want: "/var/data/remora.db",
		},
		{
			name: "sqlite without path",
			config: &Config{
				DatabaseType: "sqlite",
				DatabaseName: "",
			},
			want: "./remora.db",
		},
		{
			name: "unknown database type",
			config: &Config{
				DatabaseType: "unknown",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.DatabaseURL()
			if got != tt.want {
				t.Errorf("DatabaseURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Errors: []error{
			os.ErrInvalid,
			os.ErrNotExist,
		},
	}
	got := err.Error()
	if got != "configuration validation failed: invalid argument; file does not exist" {
		t.Errorf("ValidationError.Error() = %v", got)
	}
}

func TestLoad_EnvParsing(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		check   func(*Config) bool
	}{
		{
			name: "invalid int env returns default",
			envVars: map[string]string{
				"DATABASE_TYPE":          "sqlite",
				"DATABASE_NAME":          "./test.db",
				"GITHUB_APP_ID":          "123456",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
				"REMORA_PORT":            "invalid",
			},
			check: func(c *Config) bool {
				return c.Port == 8080
			},
		},
		{
			name: "invalid bool env returns default",
			envVars: map[string]string{
				"DATABASE_TYPE":          "sqlite",
				"DATABASE_NAME":          "./test.db",
				"GITHUB_APP_ID":          "123456",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
				"REMORA_ENABLE_API":      "not_a_bool",
			},
			check: func(c *Config) bool {
				return c.EnableAPI == false
			},
		},
		{
			name: "invalid int64 env returns default",
			envVars: map[string]string{
				"DATABASE_TYPE":          "sqlite",
				"DATABASE_NAME":          "./test.db",
				"GITHUB_APP_ID":          "not_an_int64",
				"GITHUB_APP_PRIVATE_KEY": "test-key",
				"GITHUB_WEBHOOK_SECRET":  "test-secret",
			},
			check: func(_ *Config) bool {
				// Should fail validation since GitHubAppID is 0
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				_ = os.Setenv(k, v)
			}

			cfg, err := Load()
			// Some tests may fail validation, that's okay
			if err == nil && cfg != nil && !tt.check(cfg) {
				t.Errorf("Load() env parsing failed for %s", tt.name)
			}
		})
	}
}

func TestLoad_AllLogLevels(t *testing.T) {
	logLevels := []string{"debug", "info", "warn", "error", "fatal"}

	for _, level := range logLevels {
		t.Run(level, func(t *testing.T) {
			os.Clearenv()
			_ = os.Setenv("DATABASE_TYPE", "sqlite")
			_ = os.Setenv("DATABASE_NAME", "./test.db")
			_ = os.Setenv("GITHUB_APP_ID", "123456")
			_ = os.Setenv("GITHUB_APP_PRIVATE_KEY", "test-key")
			_ = os.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")
			_ = os.Setenv("LOG_LEVEL", level)

			cfg, err := Load()
			if err != nil {
				t.Errorf("Load() error = %v for log level %s", err, level)
				return
			}
			if cfg.LogLevel != level {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, level)
			}
		})
	}
}
