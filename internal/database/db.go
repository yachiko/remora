package database

import (
	"fmt"
	"time"

	"github.com/yachiko/remora/internal/config"
	"github.com/yachiko/remora/internal/logger"
	"github.com/yachiko/remora/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DB is the global database instance
var DB *gorm.DB

// Initialize sets up the database connection and runs migrations
func Initialize(cfg *config.Config) error {
	var dialector gorm.Dialector
	var err error

	// Configure GORM logger
	gormLog := gormlogger.Default.LogMode(gormlogger.Silent)
	if cfg.LogLevel == "debug" {
		gormLog = gormlogger.Default.LogMode(gormlogger.Info)
	}

	// Select appropriate database driver
	switch cfg.DatabaseType {
	case "postgresql", "postgres":
		dsn := cfg.DatabaseURL()
		dialector = postgres.Open(dsn)
		logger.Info("connecting to postgresql", zap.String("host", cfg.DatabaseHost))

	case "mysql":
		dsn := cfg.DatabaseURL()
		dialector = mysql.Open(dsn)
		logger.Info("connecting to mysql", zap.String("host", cfg.DatabaseHost))

	case "sqlite":
		dsn := cfg.DatabaseName
		dialector = sqlite.Open(dsn)
		logger.Info("connecting to sqlite", zap.String("file", cfg.DatabaseName))

	default:
		return fmt.Errorf("unsupported database type: %s", cfg.DatabaseType)
	}

	// Open database connection
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: gormLog,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	logger.Info("database connection pool configured",
		zap.Int("max_idle_conns", 10),
		zap.Int("max_open_conns", 100))

	// Run auto-migration
	if err := AutoMigrate(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.Info("database initialized successfully")
	return nil
}

// AutoMigrate runs database migrations
func AutoMigrate() error {
	logger.Info("running database migrations")

	if err := DB.AutoMigrate(&models.Reminder{}); err != nil {
		return fmt.Errorf("failed to migrate Reminder model: %w", err)
	}

	logger.Info("database migrations completed")
	return nil
}

// HealthCheck verifies database connectivity
func HealthCheck() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	logger.Info("closing database connection")
	return sqlDB.Close()
}
