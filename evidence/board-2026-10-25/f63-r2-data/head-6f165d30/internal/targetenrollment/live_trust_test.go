package targetenrollment

import (
	"context"
	"crypto/x509"
	"errors"
	"testing"
)

func TestActiveNodeAdmissionRequiresCurrentCAAfterRotationOverlap(t *testing.T) {
	oldState, oldRegistry := authenticatedNodeState(t)
	newState, newRegistry := authenticatedNodeState(t)
	oldRoot := oldState.VerifiedChains[0][len(oldState.VerifiedChains[0])-1]
	newRoot := newState.VerifiedChains[0][len(newState.VerifiedChains[0])-1]
	if oldRoot.Equal(newRoot) {
		t.Fatal("rotation requires independent test authorities")
	}
	roots := x509.NewCertPool()
	roots.AddCert(oldRoot)
	roots.AddCert(newRoot)
	load := func(context.Context) (*x509.CertPool, error) { return roots, nil }
	oldLive, err := WithLiveTrust(oldRegistry, load)
	if err != nil {
		t.Fatal(err)
	}
	newLive, err := WithLiveTrust(newRegistry, load)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if AdmitTLSNode(ctx, oldState, testScope(), oldLive) != nil || AdmitTLSNode(ctx, newState, testScope(), newLive) != nil {
		t.Fatal("overlap must admit both currently trusted, registered certificates")
	}
	// Keep the original handshake state: it still has a verified old chain.
	// Removing its root must reject before the persisted active row is consulted.
	roots = x509.NewCertPool()
	roots.AddCert(newRoot)
	before := oldRegistry.calls
	if !errors.Is(AdmitTLSNode(ctx, oldState, testScope(), oldLive), ErrDenied) || oldRegistry.calls != before {
		t.Fatal("cached handshake chain kept a retired CA admitted")
	}
	if AdmitTLSNode(ctx, newState, testScope(), newLive) != nil {
		t.Fatal("current authority certificate stopped working after overlap")
	}
	newRegistry.denied = true
	if !errors.Is(AdmitTLSNode(ctx, newState, testScope(), newLive), ErrDenied) {
		t.Fatal("current CA bypassed persisted certificate revocation")
	}
}

func TestActiveNodeAdmissionCannotFallBackWhenCurrentTrustIsUnavailable(t *testing.T) {
	state, registry := authenticatedNodeState(t)
	for _, load := range []func(context.Context) (*x509.CertPool, error){
		func(context.Context) (*x509.CertPool, error) { return nil, nil },
		func(context.Context) (*x509.CertPool, error) { return x509.NewCertPool(), nil },
		func(context.Context) (*x509.CertPool, error) { return nil, errors.New("trust reload failed") },
	} {
		live, err := WithLiveTrust(registry, load)
		if err != nil {
			t.Fatal(err)
		}
		before := registry.calls
		if !errors.Is(AdmitTLSNode(context.Background(), state, testScope(), live), ErrDenied) || registry.calls != before {
			t.Fatal("unavailable trust fell back to the original verified chain")
		}
	}
	if _, err := WithLiveTrust(nil, func(context.Context) (*x509.CertPool, error) { return nil, nil }); err == nil {
		t.Fatal("live trust accepted a missing persistent registry")
	}
	if _, err := WithLiveTrust(registry, nil); err == nil {
		t.Fatal("live trust accepted a missing current authority")
	}
}
