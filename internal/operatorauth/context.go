package operatorauth

import (
	"context"
)

type enforcementKey struct{}

func WithEnforcement(ctx context.Context) context.Context {
	return context.WithValue(ctx, enforcementKey{}, true)
}

func EnforcementEnabled(ctx context.Context) bool {
	value, _ := ctx.Value(enforcementKey{}).(bool)
	return value
}
