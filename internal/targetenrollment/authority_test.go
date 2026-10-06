package targetenrollment

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type memoryStore struct {
	mu       sync.Mutex
	grants   map[string]Grant
	consumed map[string]string
}

func newStore() *memoryStore {
	return &memoryStore{grants: map[string]Grant{}, consumed: map[string]string{}}
}
func (s *memoryStore) Create(_ context.Context, grant Grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[grant.TokenDigest] = grant
	return nil
}
func (s *memoryStore) Consume(_ context.Context, scope Scope, token, nonce, csr string, now time.Time) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.grants[token]
	if !ok || grant.Scope != scope || grant.NonceDigest != nonce || !grant.ExpiresAt.After(now) || s.consumed[token] != "" {
		return Grant{}, ErrDenied
	}
	s.consumed[token] = csr
	return grant, nil
}
func (s *memoryStore) RecordIssued(context.Context, Scope, string, string, time.Time) error {
	return nil
}
func testScope() Scope {
	return Scope{TenantID: "00000000-0000-0000-0000-000000000001", TargetID: "lab", NodeID: "node-a", Runtime: "docker"}
}
func requestFor(t *testing.T, scope Scope, bootstrap Bootstrap) Request {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse(scope.Identity())
	raw, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uri}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return Request{Scope: scope, Token: bootstrap.Token, Nonce: bootstrap.Nonce, CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: raw})}
}
func TestEnrollmentConcurrentOneUseAndRestart(t *testing.T) {
	store := newStore()
	issuer := serviceissuer.New(t)
	authority, _ := New(store, issuer)
	bootstrap, err := authority.Create(context.Background(), testScope(), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := requestFor(t, testScope(), bootstrap)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			result, err := authority.Enroll(context.Background(), request)
			if err == nil {
				successes.Add(1)
				if len(result.Certificate.PrivateKey) != 0 || len(result.Trust.PEM) == 0 {
					t.Error("enrollment lost key-locality or trust")
				}
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful simultaneous enrollments = %d", successes.Load())
	}
	restarted, _ := New(store, issuer)
	if _, err := restarted.Enroll(context.Background(), request); !errors.Is(err, ErrDenied) {
		t.Fatalf("replay after authority restart: %v", err)
	}
	if store.consumed[digest(bootstrap.Token)] == "" {
		t.Fatal("consumption did not retain CSR digest")
	}
}
func TestEnrollmentScopeNonceExpiryAndCSRFailClosed(t *testing.T) {
	for _, field := range []string{"tenant", "target", "node", "runtime", "nonce", "token", "csr", "expired"} {
		t.Run(field, func(t *testing.T) {
			store := newStore()
			authority, _ := New(store, serviceissuer.New(t))
			bootstrap, err := authority.Create(context.Background(), testScope(), time.Minute, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			request := requestFor(t, testScope(), bootstrap)
			switch field {
			case "tenant":
				request.Scope.TenantID = "00000000-0000-0000-0000-000000000002"
			case "target":
				request.Scope.TargetID = "foreign"
			case "node":
				request.Scope.NodeID = "foreign"
			case "runtime":
				request.Scope.Runtime = "podman"
			case "nonce":
				request.Nonce, _ = randomCredential()
			case "token":
				request.Token, _ = randomCredential()
			case "csr":
				request.CSRPEM = append([]byte("secret-marker\n"), request.CSRPEM...)
			case "expired":
				grant := store.grants[digest(bootstrap.Token)]
				grant.ExpiresAt = time.Now().Add(-time.Second)
				store.grants[digest(bootstrap.Token)] = grant
			}
			if _, err := authority.Enroll(context.Background(), request); !errors.Is(err, ErrDenied) {
				t.Fatalf("scope mismatch accepted: %v", err)
			}
			if len(store.consumed) != 0 {
				t.Fatal("invalid request consumed an authorization")
			}
		})
	}
}

type failingIssuer struct{ serviceaccess.CSRIssuer }

func (f failingIssuer) SignCSR(context.Context, serviceaccess.CSRSigningRequest) (serviceaccess.IssuedCertificate, error) {
	return serviceaccess.IssuedCertificate{}, errors.New("SECRET ISSUER ERROR")
}
func TestSigningFailureDoesNotRestoreGrantOrLeakProviderError(t *testing.T) {
	store := newStore()
	authority, _ := New(store, failingIssuer{serviceissuer.New(t)})
	bootstrap, _ := authority.Create(context.Background(), testScope(), time.Minute, time.Hour)
	request := requestFor(t, testScope(), bootstrap)
	if _, err := authority.Enroll(context.Background(), request); err == nil || err.Error() != "enrollment signing failed; request a new authorization" {
		t.Fatalf("unsanitized signing error: %v", err)
	}
	if _, err := authority.Enroll(context.Background(), request); !errors.Is(err, ErrDenied) {
		t.Fatalf("failed signing restored grant: %v", err)
	}
}
func TestGrantRejectsUnsupportedScopeAndBounds(t *testing.T) {
	authority, _ := New(newStore(), serviceissuer.New(t))
	for _, scope := range []Scope{{}, {TenantID: "tenant", TargetID: "lab", NodeID: "node", Runtime: "docker"}, {TenantID: testScope().TenantID, TargetID: "../escape", NodeID: "node", Runtime: "docker"}} {
		if _, err := authority.Create(context.Background(), scope, time.Minute, time.Hour); !errors.Is(err, ErrDenied) {
			t.Fatal("invalid scope admitted")
		}
	}
	for _, lifetime := range []time.Duration{0, -time.Second, 11 * time.Minute} {
		if _, err := authority.Create(context.Background(), testScope(), lifetime, time.Hour); !errors.Is(err, ErrDenied) {
			t.Fatal("invalid grant lifetime admitted")
		}
	}
}

type failedRecordingStore struct{ *memoryStore }

func (s failedRecordingStore) RecordIssued(context.Context, Scope, string, string, time.Time) error {
	return errors.New("SECRET DATABASE ERROR")
}

type revocationIssuer struct {
	serviceaccess.CSRIssuer
	revoked string
}

func (i *revocationIssuer) Revoke(_ context.Context, serial string) error {
	i.revoked = serial
	return nil
}
func TestCertificatePersistenceFailureRevokesAndReturnsNoMaterial(t *testing.T) {
	issuer := &revocationIssuer{CSRIssuer: serviceissuer.New(t)}
	authority, _ := New(failedRecordingStore{newStore()}, issuer)
	bootstrap, err := authority.Create(context.Background(), testScope(), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result, err := authority.Enroll(context.Background(), requestFor(t, testScope(), bootstrap))
	if err == nil || err.Error() != "enrollment identity persistence failed; certificate admission is unavailable" || len(result.Certificate.Certificate) != 0 || issuer.revoked == "" {
		t.Fatalf("orphan certificate exposed or not revoked: %v", err)
	}
}

func TestEnrollmentCancelledAdmissionDoesNotCreateOrConsume(t *testing.T) {
	store := newStore()
	authority, err := New(store, serviceissuer.New(t))
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := authority.Create(cancelled, testScope(), time.Minute, time.Hour); !errors.Is(err, ErrDenied) || len(store.grants) != 0 {
		t.Fatal("cancelled admission created an authorization", err)
	}
	bootstrap, err := authority.Create(context.Background(), testScope(), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Enroll(cancelled, requestFor(t, testScope(), bootstrap)); !errors.Is(err, ErrDenied) || len(store.consumed) != 0 {
		t.Fatal("cancelled admission consumed an authorization", err)
	}
}
