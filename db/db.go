package db

import (
	"fmt"
	"sync"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Config represents database configuration
type Config struct {
	DSN   string `yaml:"dsn" json:"dsn"`     // MySQL DSN connection string
	Debug bool   `yaml:"debug" json:"debug"` // Enable debug mode
}

var (
	globalDB  *gorm.DB
	initOnce  sync.Once
	initErr   error
)

// loadConfigFromViper loads database configuration from viper
// Configuration path: database.dsn and database.debug
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	// Load from viper
	cfg.DSN = viper.GetString("database.dsn")
	cfg.Debug = viper.GetBool("database.debug")

	// Validate required fields
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database DSN not configured (check database.dsn)")
	}

	return cfg, nil
}

// initialize performs the actual database initialization
// This is called once via sync.Once
func initialize() {
	cfg, err := loadConfigFromViper()
	if err != nil {
		initErr = fmt.Errorf("failed to load database config: %v", err)
		return
	}

	var dbErr error
	globalDB, dbErr = gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if dbErr != nil {
		initErr = fmt.Errorf("failed to connect to database: %v", dbErr)
		return
	}

	if cfg.Debug {
		globalDB = globalDB.Debug()
	}

	initErr = nil
}

// Get returns the global database instance with lazy loading
// The database is initialized on the first call to Get()
// Returns nil if initialization failed
func Get() *gorm.DB {
	initOnce.Do(initialize)
	return globalDB
}

// MustGet returns the global database instance or panics if not initialized
// The database is initialized on the first call
func MustGet() *gorm.DB {
	initOnce.Do(initialize)

	if initErr != nil {
		panic(fmt.Sprintf("database initialization failed: %v", initErr))
	}

	if globalDB == nil {
		panic("database is nil after initialization")
	}

	return globalDB
}

// GetError returns the initialization error if any
func GetError() error {
	return initErr
}

// Close closes the database connection
func Close() error {
	if globalDB == nil {
		return nil
	}

	sqlDB, err := globalDB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// Reset resets the database instance and initialization state
// This is mainly useful for testing
func Reset() {
	if globalDB != nil {
		sqlDB, _ := globalDB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	globalDB = nil
	initErr = nil
	initOnce = sync.Once{}
}
