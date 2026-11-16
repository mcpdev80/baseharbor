package machine

import "context"

type executionCorrelationKey struct{}

// WithExecutionCorrelation carries the authoritative execution identity to
// typed transport and provider boundaries. It contains no actor credential.
func WithExecutionCorrelation(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionCorrelationKey{}, id)
}

func ExecutionCorrelation(ctx context.Context) string {
	id, _ := ctx.Value(executionCorrelationKey{}).(string)
	return id
}
