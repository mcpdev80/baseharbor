package installauthority

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// errProvider models an unreadable revocation provider.
var errProvider = errors.New("revocation provider failed")

// fakeIssuer is a controllable serviceaccess.Issuer used to drive every second
// state of the single authoritative tri-state RevokedStatus call.
type fakeIssuer struct {
	ready            bool
	revokedStatus    bool
	revokedStatusErr error
	serial           string
	calls            int
}

func (f *fakeIssuer) TrustBundle(context.Context) (serviceaccess.TrustBundle, error) {
	return serviceaccess.TrustBundle{}, nil
}

func (f *fakeIssuer) Status(context.Context) (serviceaccess.IssuerStatus, error) {
	return serviceaccess.IssuerStatus{Ready: f.ready}, nil
}

func (f *fakeIssuer) RevokedStatus(ctx context.Context, serial string) (bool, error) {
	f.calls++
	if serial != f.serial {
		return false, errProvider
	}
	return f.revokedStatus, f.revokedStatusErr
}

func freshObserved(serial string) serviceaccess.CertificateStatus {
	return serviceaccess.CertificateStatus{
		Identity:          "authoritative-connection",
		IssuedCertificate: serviceaccess.IssuedCertificate{Serial: serial, ExpiresAt: time.Now().UTC().Add(time.Hour)},
	}
}

// TestFreshRevokedStatusSecondStatesRejected proves the boundary rejects on every
// tri-state second state: not-revoked keeps it, a revoked serial rejects, and an
// unreadable revocation status rejects as foreign. The not-revoked case is the
// only success.
func TestFreshRevokedStatusSecondStatesRejected(t *testing.T) {
	tests := []struct {
		name           string
		ready          bool
		revokedStatus  bool
		revokedStatusErr error
		want           error
	}{
		{"not revoked accepted", true, false, nil, nil},
		{"revoked rejected", true, true, nil, serviceaccess.ErrForeignAuthority},
		{"unreadable status rejected", true, false, errProvider, serviceaccess.ErrForeignAuthority},
		{"not ready rejected", false, false, nil, serviceaccess.ErrNotReady},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issuer := &fakeIssuer{
				ready:            tc.ready,
				revokedStatus:    tc.revokedStatus,
				revokedStatusErr: tc.revokedStatusErr,
				serial:           "leaf-serial",
			}
			if err := Fresh(context.Background(), issuer, freshObserved("leaf-serial"), "authoritative-connection", time.Now().UTC(), time.Minute); err != tc.want {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

// TestFreshSingleRevokedStatusCall proves RevokedStatus is invoked exactly once
// per Fresh, so no status could change between two calls.
func TestFreshSingleRevokedStatusCall(t *testing.T) {
	issuer := &fakeIssuer{ready: true, revokedStatus: false, serial: "leaf-serial"}
	if err := Fresh(context.Background(), issuer, freshObserved("leaf-serial"), "authoritative-connection", time.Now().UTC(), time.Minute); err != nil {
		t.Fatalf("expected accept, got %v", err)
	}
	if issuer.calls != 1 {
		t.Fatalf("RevokedStatus called %d times, want exactly 1", issuer.calls)
	}
}

// TestFreshObservedAtFreshnessFailClosed proves the observed-at freshness bound
// fails closed on a zero, future and stale observation and passes on a recent
// one, through deterministic times. Certificate expiry is not the freshness
// bound here.
func TestFreshObservedAtFreshnessFailClosed(t *testing.T) {
	const window = time.Minute
	tests := []struct {
		name       string
		observedAt time.Time
		want       error
	}{
		{"recent observed fresh", time.Now().UTC().Add(-30 * time.Second), nil},
		{"zero observed fails closed", time.Time{}, serviceaccess.ErrInvalidObservation},
		{"future observed fails closed", time.Now().UTC().Add(time.Hour), serviceaccess.ErrInvalidObservation},
		{"stale observed fails closed", time.Now().UTC().Add(-(2 * window)), serviceaccess.ErrStaleObservation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issuer := &fakeIssuer{ready: true, revokedStatus: false, serial: "leaf-serial"}
			err := Fresh(context.Background(), issuer, freshObserved("leaf-serial"), "authoritative-connection", tc.observedAt, window)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("expected accept, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}
