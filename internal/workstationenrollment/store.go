package workstationenrollment

import (
	"context"
	"sync"
	"time"
)

// Store persists one-use workstation authorizations and consumes them exactly
// once during enrollment. Implementations must persist consumption before
// returning success, including under concurrent requests and process restarts.
//
// This interface is the bounded write-set for this slice. Implementations live
// under internal/workstationenrollment (or an owned persistence package); no
// shared node-enrollment store or register is edited.
type Store interface {
	// Create persists a one-use authorization for a scope.
	Create(ctx context.Context, b Bootstrap) error
	// Consume returns the stored authorization for scope once, then forgets it.
	// A missing, foreign, already-consumed, or already-expired authorization
	// fails closed. ttl is the requested certificate TTL used to reject an
	// authorization created with a disallowed window.
	Consume(ctx context.Context, scope Scope, token, nonce string, ttl time.Duration) (Bootstrap, error)
}

// memoryStore is a concurrency-safe, in-memory Store for the HTTP adapter and
// tests. It persists consumption before returning success so concurrent
// enrollments of the same one-use bootstrap cannot both succeed.
type memoryStore struct {
	mu     sync.Mutex
	grants map[scopeKey]grant
}

type scopeKey struct {
	tenant  string
	station string
}

// NewMemoryStore returns an empty one-use store.
func NewMemoryStore() Store {
	return &memoryStore{grants: map[scopeKey]grant{}}
}

func (m *memoryStore) Create(_ context.Context, b Bootstrap) error {
	if err := b.RequestScope.Validate(); err != nil {
		return ErrDenied
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := scopeKey{b.RequestScope.TenantID, b.RequestScope.Station}
	m.grants[key] = grant{
		scope:          b.RequestScope,
		tokenDigest:    digest(b.Token),
		nonceDigest:    digest(b.Nonce),
		expiresAt:      b.ExpiresAt,
		certificateTTL: b.CertificateTTL,
	}
	return nil
}

func (m *memoryStore) Consume(_ context.Context, scope Scope, token, nonce string, ttl time.Duration) (Bootstrap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := scopeKey{scope.TenantID, scope.Station}
	g, ok := m.grants[key]
	if !ok {
		return Bootstrap{}, ErrDenied
	}
	if time.Now().UTC().After(g.expiresAt) {
		delete(m.grants, key)
		return Bootstrap{}, ErrDenied
	}
	if digest(token) != g.tokenDigest || digest(nonce) != g.nonceDigest {
		return Bootstrap{}, ErrDenied
	}
	delete(m.grants, key)
	if ttl <= 0 || ttl > MaxCertificateTTL {
		return Bootstrap{}, ErrDenied
	}
	return Bootstrap{
		Token:          token,
		Nonce:          nonce,
		ExpiresAt:      g.expiresAt,
		CertificateTTL: g.certificateTTL,
	}, nil
}
