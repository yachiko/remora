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
				os.Setenv(k, v)
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
	os.Setenv("DATABASE_TYPE", "sqlite")
	os.Setenv("DATABASE_NAME", "./test.db")
	os.Setenv("GITHUB_APP_ID", "123456")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "test-key")
	os.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")

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
