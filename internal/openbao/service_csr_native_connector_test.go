package openbao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/database"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// This optional black-box extension uses the actual separately built Connector,
// native runtime, production OpenBao issuer and persisted PostgreSQL admission.
// Direct grant creation qualifies exchange, not operator OIDC authorization or
// the Core-authoritative Application lifecycle. It is not release approval.
func nativeBaoConnectorEnrollment(t *testing.T, ctx context.Context, executor Executor, files bhruntime.Files, issuer *ServiceIssuer, authority *targetenrollment.Authority, registry *database.ConnectorEnrollmentStore, coreRequest serviceaccess.CSRSigningRequest, coreKey []byte, coreCertificate serviceaccess.IssuedCertificate, trust []byte) {
	t.Helper()
	fixture := newNativeConnectorFixture(t, ctx)
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "native-pki", NodeID: "node-live", Runtime: fixture.engine}
	var currentCertificate atomic.Pointer[tls.Certificate]
	var currentRoots atomic.Pointer[x509.CertPool]
	updateTLS := func(certificate serviceaccess.IssuedCertificate, rootsPEM []byte) {
		t.Helper()
		pair, err := tls.X509KeyPair(certificate.Certificate, coreKey)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(rootsPEM) {
			t.Fatal("managed trust is invalid")
		}
		currentCertificate.Store(&pair)
		currentRoots.Store(roots)
		fixture.write(t, "ca.pem", rootsPEM)
	}
	updateTLS(coreCertificate, trust)
	configuration := &tls.Config{MinVersion: tls.VersionTLS13, SessionTicketsDisabled: true, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return currentCertificate.Load(), nil }}
	handler, err := targetenrollment.NewHTTP(authority, func(_ context.Context, target, node, _ string) (targetenrollment.Scope, error) {
		if target != scope.TargetID || node != scope.NodeID {
			return targetenrollment.Scope{}, errors.New("foreign scope")
		}
		return scope, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	enrollment := httptest.NewUnstartedServer(handler)
	enrollment.TLS = configuration.Clone()
	enrollment.StartTLS()
	defer enrollment.Close()
	observed := &nativeConnectorAdmission{NodeRegistry: registry}
	live, err := targetenrollment.WithLiveTrust(observed, func(context.Context) (*x509.CertPool, error) { return currentRoots.Load(), nil })
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	sessionTLS := configuration.Clone()
	sessionTLS.ClientAuth = tls.RequireAndVerifyClientCert
	sessionTLS.ClientCAs = currentRoots.Load()
	// Every new handshake loads live roots; existing sessions revalidate the same
	// live trust and persisted certificate state before dispatch.
	sessionTLS.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		next := sessionTLS.Clone()
		next.GetConfigForClient = nil
		next.ClientCAs = currentRoots.Load()
		return next, nil
	}
	pool := targetsession.NewPool()
	serving := make(chan error, 1)
	go func() { serving <- pool.Serve(ctx, listener, sessionTLS, live, coreRequest.Identity) }()
	defer func() { _ = listener.Close(); <-serving }()
	authorize := func(renew bool) {
		t.Helper()
		create := authority.Create
		if renew {
			create = authority.CreateRenewal
		}
		grant, err := create(ctx, scope, time.Minute, time.Hour)
		if err != nil {
			t.Fatal("managed Connector grant failed", err)
		}
		data, err := json.Marshal(grant)
		if err != nil {
			t.Fatal(err)
		}
		fixture.write(t, "authorization.json", data)
	}
	authorize(false)
	stop := fixture.start(t, ctx, scope, listener.Addr().String(), enrollment.URL+targetenrollment.EnrollmentPath, coreRequest.Identity, false)
	defer func() { stop() }()
	fixture.inventory(t, ctx, pool, scope)
	if _, err := os.Stat(filepath.Join(fixture.dir, "authorization.json")); !os.IsNotExist(err) {
		t.Fatal("one-use bootstrap authorization was retained")
	}
	original := fixture.leaf(t)
	keyBefore := fixture.read(t, "node.key")
	// The managed CA rotates while the old outbound connection remains usable.
	if err := RotateServiceCA(ctx, executor, files); err != nil {
		t.Fatal(err)
	}
	rotatedTrust, err := issuer.TrustBundle(ctx)
	if err != nil || bytes.Equal(trust, rotatedTrust.PEM) {
		t.Fatal("Connector CA did not rotate", err)
	}
	rotatedCore, err := issuer.SignCoreCSR(ctx, coreRequest)
	if err != nil {
		t.Fatal(err)
	}
	overlap := append(append([]byte{}, trust...), rotatedTrust.PEM...)
	updateTLS(rotatedCore, overlap)
	fixture.inventory(t, ctx, pool, scope)
	stop()
	authorize(true)
	stop = fixture.start(t, ctx, scope, listener.Addr().String(), enrollment.URL+targetenrollment.EnrollmentPath, coreRequest.Identity, true)
	fixture.inventory(t, ctx, pool, scope)
	renewed := fixture.leaf(t)
	if original.SerialNumber.Cmp(renewed.SerialNumber) == 0 || !bytes.Equal(original.RawSubjectPublicKeyInfo, renewed.RawSubjectPublicKeyInfo) || !bytes.Equal(keyBefore, fixture.read(t, "node.key")) {
		t.Fatal("actual Connector renewal replaced its local key or reused serial")
	}
	if err := registry.AdmitCertificate(ctx, scope, original.SerialNumber.Text(16), original.NotAfter); err != nil {
		t.Fatal("persisted old node did not remain admitted during overlap", err)
	}
	updateTLS(rotatedCore, rotatedTrust.PEM)
	fixture.inventory(t, ctx, pool, scope)
	// Reconnect with retired old roots and no re-enrollment: only renewed material
	// may regain the actual outbound channel. No mutating operation is retried.
	stop()
	stop = fixture.start(t, ctx, scope, listener.Addr().String(), enrollment.URL+targetenrollment.EnrollmentPath, coreRequest.Identity, false)
	resourceID := fixture.inventory(t, ctx, pool, scope)
	fixture.exec(t, ctx, pool, scope, resourceID)
	roots := currentRoots.Load()
	if _, err := original.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err == nil {
		t.Fatal("old node survived root retirement")
	}
	if err := registry.RevokeCertificate(ctx, scope, renewed.SerialNumber.Text(16)); err != nil {
		t.Fatal(err)
	}
	if err := issuer.Revoke(ctx, renewed.SerialNumber.Text(16)); err != nil {
		t.Fatal(err)
	}
	// Admission is checked at invocation, not just eventual background expiry.
	response, err := fixture.dispatch(ctx, pool, scope, "runtime.resource.list", struct{}{})
	if err == nil && response.Success {
		t.Fatal("revoked actual session dispatched inventory")
	}
	stop()
	denials := observed.denied.Load()
	stop = fixture.start(t, ctx, scope, listener.Addr().String(), enrollment.URL+targetenrollment.EnrollmentPath, coreRequest.Identity, false)
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		response, err := fixture.dispatch(ctx, pool, scope, "runtime.resource.list", struct{}{})
		if err == nil && response.Success {
			t.Fatal("revoked actual Connector reconnected")
		}
		if observed.denied.Load() > denials {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if observed.denied.Load() <= denials {
		t.Fatal("revoked actual Connector did not attempt new persisted admission")
	}
	if _, err := authority.CreateRenewal(ctx, scope, time.Minute, time.Hour); err == nil {
		t.Fatal("revoked actual Connector regained renewal")
	}
	t.Log("actual Connector managed enrollment, local-key renewal, CA overlap/retirement, native typed exec and persisted revocation passed; Application lifecycle and operator OIDC not qualified")
}

func nativeConnectorLeaf(t *testing.T, data []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("actual Connector certificate missing")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

// Observe a real denial without replacing the persisted production registry.
type nativeConnectorAdmission struct {
	targetenrollment.NodeRegistry
	denied atomic.Uint64
}

func (r *nativeConnectorAdmission) AdmitCertificate(ctx context.Context, scope targetenrollment.Scope, serial string, expires time.Time) error {
	err := r.NodeRegistry.AdmitCertificate(ctx, scope, serial, expires)
	if err != nil {
		r.denied.Add(1)
	}
	return err
}
