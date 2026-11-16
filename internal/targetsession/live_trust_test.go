package targetsession

import (
	"context"
	"crypto/x509"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

func TestCurrentCARemovalRetiresAuthenticatedSessionBeforeDispatch(t *testing.T) {
	var trust atomic.Pointer[x509.CertPool]
	session, _, registry := newSessionPairWithRegistry(t, false, func(registry *registry, initial *x509.CertPool) targetenrollment.NodeRegistry {
		trust.Store(initial)
		live, err := targetenrollment.WithLiveTrust(registry, func(context.Context) (*x509.CertPool, error) { return trust.Load(), nil })
		if err != nil {
			t.Fatal(err)
		}
		return live
	})
	trust.Store(x509.NewCertPool())
	before := registry.calls.Load()
	if _, err := session.Dispatch(context.Background(), requestNow("retired-ca")); !errors.Is(err, ErrUnavailable) || session.ctx.Err() == nil {
		t.Fatal("old handshake kept session usable after its CA was removed", err)
	}
	if registry.calls.Load() != before {
		t.Fatal("untrusted certificate reached persisted admission")
	}
}

func TestIdleCurrentCARemovalWithdrawsCapabilitiesWithinAdmissionBound(t *testing.T) {
	var trust atomic.Pointer[x509.CertPool]
	session, _, _ := newSessionPairWithRegistry(t, false, func(registry *registry, initial *x509.CertPool) targetenrollment.NodeRegistry {
		trust.Store(initial)
		live, err := targetenrollment.WithLiveTrust(registry, func(context.Context) (*x509.CertPool, error) { return trust.Load(), nil })
		if err != nil {
			t.Fatal(err)
		}
		return live
	})
	trust.Store(x509.NewCertPool())
	select {
	case <-session.ctx.Done():
		if _, err := session.LiveCapabilities(); !errors.Is(err, ErrUnavailable) {
			t.Fatal("idle session kept capability after its CA was removed", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("idle CA retirement exceeded admission check bound")
	}
}
