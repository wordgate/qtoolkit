// Package pg loads PostgreSQL settings from viper and returns a native
// *pgxpool.Pool (jackc/pgx v5). It adds no query API of its own: business code
// uses pgx directly, or sqlc-generated code with `sql_package: "pgx/v5"`.
//
// Usage:
//
//	viper.SetConfigFile("config.yml")
//	viper.ReadInConfig()
//
//	pool := pg.MustGet() // fails only on bad config; connects lazily
//	defer pg.Close()
//
//	q := store.New(pool) // sqlc: *pgxpool.Pool satisfies DBTX
//
//	// Transactions: use pgx's own helper (commit on nil, rollback on error).
//	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
//		return store.New(tx).CreateUser(ctx, arg) // pgx.Tx satisfies DBTX
//	})
package pg

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
)

// Config holds the pool settings read from viper (pg.*).
//
// Every pool field is optional. An unset (zero) field keeps the value from the
// DSN, and if the DSN does not set it either, pgxpool's default applies:
//
//	key                 pgxpool default         notes
//	──────────────────  ──────────────────────  ────────────────────────────────
//	max_conns           max(4, runtime.NumCPU)  size it for everything the process
//	                                            runs (HTTP + river workers) and
//	                                            keep hosts × max_conns below the
//	                                            server's max_connections.
//	min_conns           0                       connections kept open when idle.
//	max_conn_lifetime   1h                      recycle a connection by age.
//	max_conn_idle_time  30m                     close a connection idle this long.
//
// pgxpool also runs a health check every minute that closes broken or expired
// connections, so a connection dropped server-side is not handed out again.
type Config struct {
	DSN             string        `yaml:"dsn"`
	MaxConns        int32         `yaml:"max_conns"`
	MinConns        int32         `yaml:"min_conns"`
	MaxConnLifetime time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `yaml:"max_conn_idle_time"`
}

var (
	globalPool *pgxpool.Pool
	initErr    error
	initOnce   sync.Once
)

func loadConfigFromViper() (*Config, error) {
	cfg := &Config{
		DSN:             viper.GetString("pg.dsn"),
		MaxConns:        viper.GetInt32("pg.max_conns"),
		MinConns:        viper.GetInt32("pg.min_conns"),
		MaxConnLifetime: viper.GetDuration("pg.max_conn_lifetime"),
		MaxConnIdleTime: viper.GetDuration("pg.max_conn_idle_time"),
	}
	if cfg.DSN == "" {
		return nil, fmt.Errorf("pg.dsn is required")
	}
	return cfg, nil
}

// poolConfig parses the DSN and applies the configured overrides.
func poolConfig(cfg *Config) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		// pgx redacts the password in parse errors.
		return nil, fmt.Errorf("invalid pg.dsn: %w", err)
	}
	if cfg.MaxConns > 0 {
		pc.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		pc.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		pc.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		pc.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	if pc.MinConns > pc.MaxConns {
		return nil, fmt.Errorf("pg.min_conns (%d) exceeds max_conns (%d)", pc.MinConns, pc.MaxConns)
	}
	return pc, nil
}

func initialize() {
	cfg, err := loadConfigFromViper()
	if err != nil {
		initErr = err
		return
	}
	pc, err := poolConfig(cfg)
	if err != nil {
		initErr = err
		return
	}
	// NewWithConfig does not connect: connections are opened on first use, so
	// an unreachable server never fails or blocks startup. Queries return the
	// connection error until the server is back.
	pool, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		initErr = fmt.Errorf("create postgres pool: %w", err)
		return
	}
	globalPool = pool
}

// Get returns the shared pool, creating it from viper config on the first call.
// It never connects or waits for the server, so it fails only on invalid
// config; that error is returned on every call. Use pool.Ping(ctx) for a
// readiness check.
func Get() (*pgxpool.Pool, error) {
	initOnce.Do(initialize)
	return globalPool, initErr
}

// MustGet is Get that panics on error, for use at process startup.
func MustGet() *pgxpool.Pool {
	pool, err := Get()
	if err != nil {
		panic("pg: " + err.Error())
	}
	return pool
}

// Close closes the shared pool if it was created. Call it once at shutdown,
// after everything that uses the pool (HTTP server, river client) has stopped.
func Close() {
	if globalPool != nil {
		globalPool.Close()
	}
}
