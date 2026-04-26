package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
)

// Pool wraps pgxpool for metadata access and health checks.
type Pool struct {
	pgx *pgxpool.Pool
}

// New opens a connection pool from [config.Postgres] (DSN or composed fields) and pool tuning.
func New(ctx context.Context, pg config.Postgres) (*Pool, error) {
	dsn, err := pg.ResolveDSN()
	if err != nil {
		return nil, err
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, apperr.Wrap(err, "postgres: parse pool config")
	}
	if pg.PoolMaxConns > 0 {
		poolCfg.MaxConns = pg.PoolMaxConns
	}
	if pg.PoolMinConns >= 0 {
		poolCfg.MinConns = pg.PoolMinConns
	}
	if pg.PoolMaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = pg.PoolMaxConnLifetime
	}
	if pg.PoolMaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = pg.PoolMaxConnIdleTime
	}
	if pg.PoolHealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = pg.PoolHealthCheckPeriod
	}
	if pg.PoolMaxConnLifetimeJitter > 0 {
		poolCfg.MaxConnLifetimeJitter = pg.PoolMaxConnLifetimeJitter
	}

	pgxPool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, apperr.Wrap(err, "postgres: new pool")
	}
	p := &Pool{pgx: pgxPool}
	if err := p.pgx.Ping(ctx); err != nil {
		pgxPool.Close()
		return nil, apperr.Wrap(err, "postgres: ping")
	}
	return p, nil
}

// Close releases pool resources.
func (p *Pool) Close() {
	p.pgx.Close()
}

// Pgx returns the underlying pool for repositories (transactions, queries).
func (p *Pool) Pgx() *pgxpool.Pool {
	return p.pgx
}

// Health implements usecase health probe via Ping.
func (p *Pool) Health(ctx context.Context) error {
	return p.pgx.Ping(ctx)
}
