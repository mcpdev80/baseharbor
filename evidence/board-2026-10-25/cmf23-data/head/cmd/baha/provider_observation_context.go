package main

import (
	"context"
	"sync"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// retainedProviderObservationsKey carries one provider-fact observation through
// a revalidation chain so every plan built under the same context consumes the
// SAME timepoint instead of a fresh observation taken after materialization or
// container removal. Consent revalidation compares digest-bearing facts;
// re-observing at a later lifecycle moment turns an unchanged plan into
// stale-impact-consent drift. A retained observation is preview-equal, never a
// runtime measurement: consumers still distinguish it through
// providerFactIsRuntimeMeasurement.
type retainedProviderObservationsKey struct{}

func withRetainedProviderObservations(ctx context.Context, observations map[string]application.ProviderFact) context.Context {
	return context.WithValue(ctx, retainedProviderObservationsKey{}, observations)
}

func retainedProviderObservations(ctx context.Context) (map[string]application.ProviderFact, bool) {
	observations, ok := ctx.Value(retainedProviderObservationsKey{}).(map[string]application.ProviderFact)
	return observations, ok && len(observations) > 0
}

// providerObservationRetention is the stable holder the Apply lifecycle
// installs once under its context. Each read-only planner or authorization
// step that observes replaces the retained observation, so the consent
// revalidation compares against the confirmation-timepoint observation
// itself, not against a state a later lifecycle step already materialized or
// retired.
type providerObservationRetention struct {
	mu           sync.Mutex
	observations map[string]application.ProviderFact
}

func (r *providerObservationRetention) set(observations map[string]application.ProviderFact) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observations = observations
}

func (r *providerObservationRetention) current() (map[string]application.ProviderFact, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.observations) == 0 {
		return nil, false
	}
	return r.observations, true
}

type providerObservationRetentionKey struct{}

func withProviderObservationRetention(ctx context.Context, retention *providerObservationRetention) context.Context {
	return context.WithValue(ctx, providerObservationRetentionKey{}, retention)
}

func providerObservationRetentionFrom(ctx context.Context) (*providerObservationRetention, bool) {
	retention, ok := ctx.Value(providerObservationRetentionKey{}).(*providerObservationRetention)
	return retention, ok && retention != nil
}
