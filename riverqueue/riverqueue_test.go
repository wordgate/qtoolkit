package riverqueue

import (
	"context"
	"errors"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/spf13/viper"
)

type pingArgs struct {
	N int `json:"n"`
}

func (pingArgs) Kind() string { return "ping" }

type pingWorker struct {
	river.WorkerDefaults[pingArgs]
	done chan int
}

func (w *pingWorker) Work(ctx context.Context, job *river.Job[pingArgs]) error {
	w.done <- job.Args.N
	return nil
}

func resetViper(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
}

// testPool connects to the PostgreSQL named by PG_TEST_DSN and applies River's
// migrations, the way an application's explicit migration step would.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestQueuesDefault(t *testing.T) {
	resetViper(t)
	q, err := queuesFromViper()
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 || q[river.QueueDefault].MaxWorkers != defaultMaxWorkers {
		t.Fatalf("unexpected default queues: %+v", q)
	}
}

func TestQueuesFromViper(t *testing.T) {
	resetViper(t)
	viper.Set("river.queues", map[string]any{"default": 20, "webhooks": 5})
	q, err := queuesFromViper()
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 2 || q["default"].MaxWorkers != 20 || q["webhooks"].MaxWorkers != 5 {
		t.Fatalf("unexpected queues: %+v", q)
	}

	viper.Set("river.queues", map[string]any{"default": 0})
	if _, err := queuesFromViper(); err == nil {
		t.Fatal("want error for zero workers")
	}
}

func TestNilPool(t *testing.T) {
	if _, err := New(nil, nil); err == nil {
		t.Fatal("want error for nil pool")
	}
}

func TestCallerConfigIsKept(t *testing.T) {
	resetViper(t)
	viper.Set("river.queues", map[string]any{"default": 20})
	viper.Set("river.soft_stop_timeout", "5s")

	workers := river.NewWorkers()
	river.AddWorker(workers, &pingWorker{})
	cfg := &river.Config{
		Workers:         workers,
		Queues:          map[string]river.QueueConfig{"own": {MaxWorkers: 3}},
		SoftStopTimeout: time.Minute,
	}
	got, err := buildConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Queues["own"]; !ok || len(got.Queues) != 1 {
		t.Fatalf("caller queues replaced: %+v", got.Queues)
	}
	if got.SoftStopTimeout != time.Minute {
		t.Fatalf("caller soft stop timeout replaced: %v", got.SoftStopTimeout)
	}

	// Viper fills what the caller left unset.
	got, err = buildConfig(&river.Config{Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	if got.Queues["default"].MaxWorkers != 20 || got.SoftStopTimeout != 5*time.Second {
		t.Fatalf("viper settings not applied: %+v %v", got.Queues, got.SoftStopTimeout)
	}
	if cfg.SoftStopTimeout != time.Minute || len(cfg.Queues) != 1 {
		t.Fatal("caller config was modified")
	}
}

func TestInsertOnlyClient(t *testing.T) {
	resetViper(t)
	got, err := buildConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Queues != nil {
		t.Fatal("insert-only client must not get queues")
	}
	if got.SoftStopTimeout != defaultSoftStopTimeout {
		t.Fatalf("want default soft stop timeout, got %v", got.SoftStopTimeout)
	}

	// An insert-only client builds without touching the database.
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/db") // lazy; never dialed
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := New(pool, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWorkJobInsertedInTx(t *testing.T) {
	resetViper(t)
	pool := testPool(t)
	viper.Set("river.queues", map[string]any{"default": 2})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	worker := &pingWorker{done: make(chan int, 1)}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)

	client, err := New(pool, &river.Config{Workers: workers, FetchPollInterval: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// A job inserted in a rolled-back transaction is never worked.
	_ = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := client.InsertTx(ctx, tx, pingArgs{N: 1}, nil); err != nil {
			t.Fatal(err)
		}
		return context.Canceled // force rollback
	})
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := client.InsertTx(ctx, tx, pingArgs{N: 2}, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case n := <-worker.done:
		if n != 2 {
			t.Fatalf("worked job %d; want only the committed job 2", n)
		}
	case <-ctx.Done():
		t.Fatal("job was not worked")
	}

	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

// outagePool returns a pool on PG_TEST_DSN whose new connections fail while
// *down is true, simulating a database outage without stopping the server.
func outagePool(t *testing.T, down *atomic.Bool) *pgxpool.Pool {
	t.Helper()
	testPool(t) // skip without PG_TEST_DSN; apply migrations
	pc, err := pgxpool.ParseConfig(os.Getenv("PG_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	var d net.Dialer
	pc.ConnConfig.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if down.Load() {
			return nil, errors.New("simulated outage")
		}
		return d.DialContext(ctx, network, addr)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// River's Start pings the database and fails while it is unreachable. The
// documented pattern (README "Starting while the database is down") retries
// Start on the same client until it succeeds; this proves that works.
func TestStartRetriesUntilDatabaseIsBack(t *testing.T) {
	resetViper(t)
	var down atomic.Bool
	down.Store(true)
	pool := outagePool(t, &down)

	worker := &pingWorker{done: make(chan int, 1)}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)

	// New must not touch the database.
	client, err := New(pool, &river.Config{Workers: workers, FetchPollInterval: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("New with database down: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Start(ctx); err == nil {
		t.Fatal("want Start to fail while the database is down")
	}

	go func() {
		time.Sleep(300 * time.Millisecond)
		down.Store(false)
	}()
	for {
		if err := client.Start(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Start never succeeded after the database came back")
		case <-time.After(100 * time.Millisecond):
		}
	}

	if _, err := client.Insert(ctx, pingArgs{N: 7}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-worker.done:
		if n != 7 {
			t.Fatalf("worked job %d, want 7", n)
		}
	case <-ctx.Done():
		t.Fatal("job was not worked after a retried Start")
	}
	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

// A started client survives an outage: after its pool connections are dropped
// and the database is unreachable for a while, it works new jobs once the
// database is back, without a restart.
func TestClientRecoversFromOutage(t *testing.T) {
	resetViper(t)
	var down atomic.Bool
	pool := outagePool(t, &down)

	worker := &pingWorker{done: make(chan int, 2)}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	client, err := New(pool, &river.Config{Workers: workers, FetchPollInterval: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())

	down.Store(true)
	pool.Reset() // drop idle connections, as a server restart would
	if _, err := client.Insert(ctx, pingArgs{N: 1}, nil); err == nil {
		t.Fatal("want insert to fail during the outage")
	}
	time.Sleep(time.Second)
	down.Store(false)

	// Inserts may fail briefly while connections are re-established.
	for {
		if _, err := client.Insert(ctx, pingArgs{N: 2}, nil); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("insert never succeeded after the outage")
		case <-time.After(100 * time.Millisecond):
		}
	}
	select {
	case n := <-worker.done:
		if n != 2 {
			t.Fatalf("worked job %d, want 2", n)
		}
	case <-ctx.Done():
		t.Fatal("job inserted after the outage was not worked")
	}
}
