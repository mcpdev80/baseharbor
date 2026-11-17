package devgateway

import "context"

type coreAddressKey struct{}
type coreAddress struct{ target, address string }

// WithCoreAccessAddress carries the already verified owning Core address through
// early provider reconciliation, before public application URLs are committed.
func WithCoreAccessAddress(ctx context.Context, target, address string) context.Context {
	return context.WithValue(ctx, coreAddressKey{}, coreAddress{target: target, address: address})
}

func requestedCoreAccessAddress(ctx context.Context, target string, explicit *string) *string {
	if explicit != nil {
		return explicit
	}
	if selected, ok := ctx.Value(coreAddressKey{}).(coreAddress); ok && selected.target == target && selected.address != "" {
		return &selected.address
	}
	return nil
}
