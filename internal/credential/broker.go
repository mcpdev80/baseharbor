package credential

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBrokerNotConfigured = errors.New("credential broker not configured")
	ErrEmptyRef            = errors.New("credential ref is empty")
)

// Data contains resolved secret material for a single operation.
// Payload and expiry metadata are deliberately excluded from JSON output.
type Data struct {
	Payload map[string][]byte `json:"-"`
	Expires *time.Time        `json:"-"`
}

// Broker resolves opaque credential references into usable secret material.
// Implementations must never leak secret values through returned errors.
type Broker interface {
	Resolve(ctx context.Context, ref string, scope string) (Data, error)
}

// NoopBroker is the safe default when no secret backend is configured.
type NoopBroker struct{}

func (NoopBroker) Resolve(_ context.Context, ref string, _ string) (Data, error) {
	if ref == "" {
		return Data{}, ErrEmptyRef
	}
	return Data{}, ErrBrokerNotConfigured
}
