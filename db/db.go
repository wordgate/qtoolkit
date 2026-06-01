package db

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Connection-pool defaults, applied via the standard database/sql knobs.
//
// These are the effective values when a field is left unset in config. They are
// tuned for low-traffic services, where the dominant risk is not connection
// exhaustion but a connection sitting idle long enough to be killed server-side
// (MySQL wait_timeout / a proxy idle-cutoff) and then stalling for seconds — or
// failing with "invalid connection" — when reused.
//
//	knob              database/sql   qtoolkit   why
//	────────────────  ─────────────  ─────────  ───────────────────────────────
//	MaxOpenConns      0 (unlimited)  0          unchanged: low traffic won't
//	                                            spike, so a cap stays opt-in.
//	MaxIdleConns      2              5          a small warm pool; low concurrency
//	                                            does not need many idle conns, and
//	                                            fewer idle conns waste fewer of
//	                                            MySQL's connection slots.
//	ConnMaxLifetime   0 (forever)    1h         backstop: recycle a connection by
//	                                            age regardless of use.
//	ConnMaxIdleTime   0 (no timeout) 5m         the key fix: reap idle connections
//	                                            during the long quiet gaps of a
//	                                            low-traffic service, so we never
//	                                            reuse one the server already
//	                                            dropped.
//
// A default of 0 means "do not call the setter" — database/sql keeps its own
// behavior for that knob.
const (
	defaultMaxOpenConns    = 0
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = time.Hour
	defaultConnMaxIdleTime = 5 * time.Minute
)

// Config represents database configuration
type Config struct {
	DSN   string `yaml:"dsn" json:"dsn"`     // MySQL DSN connection string
	Debug bool   `yaml:"debug" json:"debug"` // Enable debug mode

	// Connection pool tuning (all optional; <= 0 means "use default").
	MaxOpenConns    int `yaml:"max_open_conns" json:"max_open_conns"`                         // 0 = unlimited (default)
	MaxIdleConns    int `yaml:"max_idle_conns" json:"max_idle_conns"`                         // defaults to defaultMaxIdleConns
	ConnMaxLifetime int `yaml:"conn_max_lifetime_seconds" json:"conn_max_lifetime_seconds"`   // seconds; defaults to defaultConnMaxLifetime
	ConnMaxIdleTime int `yaml:"conn_max_idle_time_seconds" json:"conn_max_idle_time_seconds"` // seconds; 0 = no idle timeout (default)
}

// poolSettings holds the effective pool parameters after defaults are applied.
type poolSettings struct {
	maxOpen     int
	maxIdle     int
	maxLifetime time.Duration
	maxIdleTime time.Duration
}

// resolvePool computes the effective connection-pool settings from cfg,
// applying the defaults above where a field is unset (<= 0). Kept pure
// (no *sql.DB) so the defaults are unit-testable without a live database.
func resolvePool(cfg *Config) poolSettings {
	p := poolSettings{
		maxOpen:     defaultMaxOpenConns,
		maxIdle:     defaultMaxIdleConns,
		maxLifetime: defaultConnMaxLifetime,
		maxIdleTime: defaultConnMaxIdleTime,
	}
	if cfg.MaxOpenConns > 0 {
		p.maxOpen = cfg.MaxOpenConns
	}
	if cfg.MaxIdleConns > 0 {
		p.maxIdle = cfg.MaxIdleConns
	}
	if cfg.ConnMaxLifetime > 0 {
		p.maxLifetime = time.Duration(cfg.ConnMaxLifetime) * time.Second
	}
	if cfg.ConnMaxIdleTime > 0 {
		p.maxIdleTime = time.Duration(cfg.ConnMaxIdleTime) * time.Second
	}
	return p
}

var (
	globalDB *gorm.DB
	initOnce sync.Once
	initErr  error
)

// loadConfigFromViper loads database configuration from viper
// Configuration path: database.dsn and database.debug
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	// Load from viper
	cfg.DSN = viper.GetString("database.dsn")
	cfg.Debug = viper.GetBool("database.debug")
	cfg.MaxOpenConns = viper.GetInt("database.max_open_conns")
	cfg.MaxIdleConns = viper.GetInt("database.max_idle_conns")
	cfg.ConnMaxLifetime = viper.GetInt("database.conn_max_lifetime_seconds")
	cfg.ConnMaxIdleTime = viper.GetInt("database.conn_max_idle_time_seconds")

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

	// Apply connection-pool settings via the standard database/sql knobs
	// (the GORM-recommended approach). Failure to reach the underlying *sql.DB
	// is non-fatal: the pool simply keeps stdlib defaults.
	if sqlDB, sqlErr := globalDB.DB(); sqlErr == nil {
		p := resolvePool(cfg)
		if p.maxOpen > 0 {
			sqlDB.SetMaxOpenConns(p.maxOpen)
		}
		sqlDB.SetMaxIdleConns(p.maxIdle)
		sqlDB.SetConnMaxLifetime(p.maxLifetime)
		if p.maxIdleTime > 0 {
			sqlDB.SetConnMaxIdleTime(p.maxIdleTime)
		}
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
