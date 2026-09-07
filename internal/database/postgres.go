package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrMissingDSN = errors.New("database DSN is required")

// Config defines the PostgreSQL connection used by BaseHarbor.
type Config struct {
	DSN            string
	ConnectTimeout time.Duration
	MaxConns       int32
	MinConns       int32
}

// Open creates and verifies a PostgreSQL pool. It returns only after a successful ping.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	if cfg.DSN == "" {
		return nil, ErrMissingDSN
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, err
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}

	connectTimeout := cfg.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 10 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(pingCtx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Ping verifies that PostgreSQL is reachable within the supplied context.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("database pool is nil")
	}
	return pool.Ping(ctx)
}
