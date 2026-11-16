package targetsession

import "context"

type poolKey struct{}

func WithPool(ctx context.Context, pool *Pool) context.Context {
	return context.WithValue(ctx, poolKey{}, pool)
}
func PoolFromContext(ctx context.Context) *Pool { pool, _ := ctx.Value(poolKey{}).(*Pool); return pool }
