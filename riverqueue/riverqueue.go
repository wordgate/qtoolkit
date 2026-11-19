// Package riverqueue builds a native *river.Client[pgx.Tx] (riverqueue/river
// with the riverpgxv5 driver) on an existing *pgxpool.Pool, filling queue
// settings from viper (river.*). It adds no job API of its own: business code
// uses river directly.
//
// Usage:
//
//	pool := pg.MustGet()
//
//	workers := river.NewWorkers()
//	river.AddWorker(workers, &SendEmailWorker{})
//
//	client, err := riverqueue.New(pool, &river.Config{
//		Workers: workers,
//		Logger:  slogger.Get(),
//	})
//	if err != nil { ... }
//	if err := client.Start(ctx); err != nil { ... }
//
//	// Insert a job in the same transaction as the domain change.
//	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
//		if err := store.New(tx).CreateOrder(ctx, arg); err != nil {
//			return err
//		}
//		_, err := client.InsertTx(ctx, tx, SendEmailArgs{...}, nil)
//		return err
//	})
//
//	// Graceful stop on deploy: stop fetching, let running jobs finish. Running
//	// jobs are cancelled after river.soft_stop_timeout.
//	client.Stop(ctx)
//
// River's schema is never migrated implicitly; see README.md.
package riverqueue

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/spf13/viper"
)

const (
	// defaultMaxWorkers is used for the default queue when river.queues is unset.
	defaultMaxWorkers = 100
	// defaultSoftStopTimeout bounds how long Stop waits for running jobs before
	// cancelling their contexts. River's own default is to wait forever.
	defaultSoftStopTimeout = 30 * time.Second
)

// New returns a river client that uses pool. cfg may be nil (insert-only
// client) and is used as given, except:
//
//   - Queues: when cfg.Workers is set and cfg.Queues is nil, queues come from
//     river.queues (queue name → max workers); if that is unset too, the
//     client works river.QueueDefault with 100 workers.
//   - SoftStopTimeout: when zero, it comes from river.soft_stop_timeout,
//     defaulting to 30s.
//
// cfg is not modified.
func New(pool *pgxpool.Pool, cfg *river.Config) (*river.Client[pgx.Tx], error) {
	if pool == nil {
		return nil, fmt.Errorf("riverqueue: pool is nil")
	}
	c, err := buildConfig(cfg)
	if err != nil {
		return nil, err
	}
	return river.NewClient(riverpgxv5.New(pool), c)
}

// buildConfig copies cfg and fills the unset fields from viper.
func buildConfig(cfg *river.Config) (*river.Config, error) {
	var c river.Config
	if cfg != nil {
		c = *cfg
	}
	if c.Workers != nil && c.Queues == nil {
		queues, err := queuesFromViper()
		if err != nil {
			return nil, err
		}
		c.Queues = queues
	}
	if c.SoftStopTimeout == 0 {
		c.SoftStopTimeout = defaultSoftStopTimeout
		if viper.IsSet("river.soft_stop_timeout") {
			c.SoftStopTimeout = viper.GetDuration("river.soft_stop_timeout")
		}
	}
	return &c, nil
}

func queuesFromViper() (map[string]river.QueueConfig, error) {
	raw := viper.GetStringMap("river.queues")
	if len(raw) == 0 {
		return map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: defaultMaxWorkers}}, nil
	}
	queues := make(map[string]river.QueueConfig, len(raw))
	for name := range raw {
		n := viper.GetInt("river.queues." + name)
		if n < 1 {
			return nil, fmt.Errorf("river.queues.%s must be a positive number of workers", name)
		}
		queues[name] = river.QueueConfig{MaxWorkers: n}
	}
	return queues, nil
}
