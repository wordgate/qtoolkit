package pg

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
)

// dbtx is the interface sqlc generates for `sql_package: "pgx/v5"`.
type dbtx interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

var (
	_ dbtx = (*pgxpool.Pool)(nil)
	_ dbtx = (pgx.Tx)(nil)
)

func reset(t *testing.T) {
	t.Helper()
	clear := func() {
		Close()
		globalPool, initErr, initOnce = nil, nil, sync.Once{}
		viper.Reset()
	}
	clear()
	t.Cleanup(clear)
}

// testDSN returns the DSN of a real PostgreSQL used by integration tests, e.g.
// PG_TEST_DSN=postgres://postgres:test@127.0.0.1:55432/qtk?sslmode=disable
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	return dsn
}

func TestMissingDSN(t *testing.T) {
	reset(t)
	if _, err := Get(); err == nil || !strings.Contains(err.Error(), "pg.dsn") {
		t.Fatalf("want pg.dsn error, got %v", err)
	}
}

func TestPoolConfigDefaultsAndOverrides(t *testing.T) {
	dsn := "postgres://u:secret@localhost:5432/db"

	pc, err := poolConfig(&Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	if pc.MaxConnLifetime != time.Hour || pc.MaxConnIdleTime != 30*time.Minute || pc.MinConns != 0 || pc.MaxConns < 4 {
		t.Fatalf("unexpected pgxpool defaults: %+v", pc)
	}

	pc, err = poolConfig(&Config{DSN: dsn, MaxConns: 20, MinConns: 2, MaxConnLifetime: 10 * time.Minute, MaxConnIdleTime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if pc.MaxConns != 20 || pc.MinConns != 2 || pc.MaxConnLifetime != 10*time.Minute || pc.MaxConnIdleTime != time.Minute {
		t.Fatalf("overrides not applied: %+v", pc)
	}

	// DSN pool params are kept when config leaves the field unset.
	pc, err = poolConfig(&Config{DSN: dsn + "?pool_max_conns=7"})
	if err != nil {
		t.Fatal(err)
	}
	if pc.MaxConns != 7 {
		t.Fatalf("want DSN max conns 7, got %d", pc.MaxConns)
	}

	if _, err := poolConfig(&Config{DSN: dsn, MaxConns: 2, MinConns: 3}); err == nil {
		t.Fatal("want error for min_conns > max_conns")
	}
}

func TestViperDurations(t *testing.T) {
	reset(t)
	viper.Set("pg.dsn", "postgres://u:p@localhost/db")
	viper.Set("pg.max_conn_lifetime", "15m")
	cfg, err := loadConfigFromViper()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConnLifetime != 15*time.Minute {
		t.Fatalf("want 15m, got %v", cfg.MaxConnLifetime)
	}
}

func TestInvalidDSNDoesNotLeakPassword(t *testing.T) {
	_, err := poolConfig(&Config{DSN: "postgres://u:supersecret@localhost:notaport/db"})
	if err == nil {
		t.Fatal("want parse error")
	}
	if strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("error leaks password: %v", err)
	}
}

func TestUnreachableServerDoesNotFailStartup(t *testing.T) {
	reset(t)
	viper.Set("pg.dsn", "postgres://u:p@127.0.0.1:1/db?connect_timeout=1")
	start := time.Now()
	pool, err := Get()
	if err != nil || pool == nil {
		t.Fatalf("Get must not fail for an unreachable server: %v", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Get blocked for %v", d)
	}
	if err := pool.Ping(context.Background()); err == nil {
		t.Fatal("want ping error from unreachable server")
	}
}

func TestInvalidConfigErrorIsSticky(t *testing.T) {
	reset(t)
	viper.Set("pg.dsn", "postgres://u:p@localhost:notaport/db")
	if _, err := Get(); err == nil {
		t.Fatal("want config error")
	}
	if _, err := Get(); err == nil {
		t.Fatal("want the init error on every call")
	}
}

func TestPoolAndTransaction(t *testing.T) {
	dsn := testDSN(t)
	reset(t)
	viper.Set("pg.dsn", dsn)
	viper.Set("pg.max_conns", 3)

	pool := MustGet()
	if pool != MustGet() {
		t.Fatal("Get must return the same pool")
	}
	if got := pool.Config().MaxConns; got != 3 {
		t.Fatalf("max_conns from viper not applied: %d", got)
	}

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS pg_test_items; CREATE TABLE pg_test_items (v int)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), `DROP TABLE IF EXISTS pg_test_items`) })

	// Commit path.
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO pg_test_items VALUES (1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Rollback path.
	boom := errors.New("boom")
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO pg_test_items VALUES (2)`); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_test_items`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 committed row, got %d", n)
	}
}
