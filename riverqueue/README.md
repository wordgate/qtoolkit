# riverqueue

Builds a native `*river.Client[pgx.Tx]` ([riverqueue/river](https://riverqueue.com)
with the `riverpgxv5` driver) on a `*pgxpool.Pool` you pass in, usually the one
from [`pg`](../pg). There is no wrapper API: use river directly.

The package is not named `river` so it can be imported next to
`github.com/riverqueue/river` without an alias.

```bash
go get github.com/wordgate/qtoolkit/riverqueue
```

## Configuration

See [`riverqueue_config.yml`](riverqueue_config.yml).

| Key | Default | Used when |
|---|---|---|
| `river.queues` | `{default: 100}` | the client has `Workers` and the code sets no `Queues` |
| `river.soft_stop_timeout` | `30s` | the code sets no `SoftStopTimeout` |

Size the pool for the workers: jobs that use the database wait for a pool
connection, and pgxpool's default is only max(4, NumCPU) connections shared
with HTTP traffic. Set `postgres.max_conns` to cover HTTP concurrency plus the
workers that hit the database. River's LISTEN/NOTIFY listener takes one more
connection, hijacked out of the pool, so the server sees `max_conns + 1`.

Everything else (Workers, Logger, PeriodicJobs, ErrorHandler, ...) is set in
code on the `river.Config` you pass; it is used as given.

## Usage

```go
pool := pg.MustGet()

workers := river.NewWorkers()
river.AddWorker(workers, &SendEmailWorker{})

client, err := riverqueue.New(pool, &river.Config{
	Workers: workers,
	Logger:  slogger.Get(),
})
if err != nil {
	return err
}
if err := client.Start(ctx); err != nil {
	return err
}

// Insert the job in the same transaction as the domain change.
err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
	if err := store.New(tx).CreateOrder(ctx, arg); err != nil {
		return err
	}
	_, err := client.InsertTx(ctx, tx, SendEmailArgs{OrderID: id}, nil)
	return err
})
```

Pass `nil` as the config for an insert-only client (no queues are worked).

### Graceful stop

On deploy, call river's own `Stop`: it stops fetching new jobs and waits for
running ones. After `river.soft_stop_timeout` the running jobs' contexts are
cancelled; `Stop` still returns only once they have returned.

```go
<-shutdownSignal
httpServer.Shutdown(ctx)
client.Stop(ctx) // ctx bounds the total wait
pg.Close()
```

## Migrations

`riverqueue` never migrates River's schema. Run River's migrations as an
explicit step, in one of two ways.

**Plain SQL files** (for a migration directory applied by your own tooling).
Print them with the River CLI, pinned to the same version as the `river`
module in your go.mod:

```bash
go install github.com/riverqueue/river/cmd/river@v0.51.2
# All migrations; version 1 only creates River's own migration-tracking table.
river migrate-get --all --exclude-version 1 --up   > migrations/0002_river.up.sql
river migrate-get --all --exclude-version 1 --down > migrations/0002_river.down.sql
# After upgrading river later: only the new versions, e.g.
river migrate-get --version 7 --up > migrations/0010_river_7.up.sql
```

**In Go** (e.g. a `migrate` subcommand of your binary):

```go
migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
if err != nil {
	return err
}
res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
```

## Tests

Integration tests need a real PostgreSQL (`PG_TEST_DSN`, see [`pg`](../pg/README.md#tests))
and are skipped without one. They run River's migrations in that database.
