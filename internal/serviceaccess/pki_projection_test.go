package serviceaccess

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Authority cells for the managed-local projection preference in
// ExistingTLSMaterial (internal/serviceaccess/pki.go), 0428-091 P2.
// TestExistingTLSMaterialPrefersTheCompleteGatewayProjection is the
// mutation-sensitive authority cell for the managed-local reader rule (P2):
// ExistingTLSMaterial, given a pki/ reference beside a gateway whose runtime
// projection is complete, resolves the projection — the bytes the boundary
// itself mounts. Deleting the projection preference in internal/serviceaccess/
// pki.go turns this cell red while the pki/ files stay untouched. A projection
// whose required client identity was retired fails closed, and a gateway
// layout WITHOUT a per-file projection (the TCP gateway writes only the
// combined runtime/server.pem) keeps resolving from the source tree.
func TestExistingTLSMaterialPrefersTheCompleteGatewayProjection(t *testing.T) {
	root := t.TempDir()
	issuer := newTestIssuer(t)
	policy, err := Resolve("prod", "tempo-reader", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	serviceAccessDir := filepath.Join(root, "service-access")
	serverNames := []string{"tempo-access", "127.0.0.1", "tempo-access.access.baseharbor.internal"}
	material, err := EnsureTLSMaterial(context.Background(), issuer, policy, filepath.Join(serviceAccessDir, "pki"), serverNames...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectGatewayMaterial(serviceAccessDir, material); err != nil {
		t.Fatal(err)
	}
	resolved, err := ExistingTLSMaterial(policy, filepath.Join(serviceAccessDir, "pki"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ServerCertificate != filepath.Join(serviceAccessDir, "runtime", "server.pem") {
		t.Fatalf("the reader must resolve the gateway's projected runtime material, got %q", resolved.ServerCertificate)
	}
	if resolved.ClientCertificate != filepath.Join(serviceAccessDir, "runtime", "client-cert.pem") {
		t.Fatalf("the reader must resolve the projected client identity, got %q", resolved.ClientCertificate)
	}
}

// TestExistingTLSMaterialFailsClosedOnRetiredProjectedIdentity pins the other
// half of the rule: where a complete projection exists but no longer carries
// the client identity the policy requires, the reader must NOT resurrect a
// stale source-tree pair beside it.
// TestExistingTLSMaterialRedirectedProjectionFailsClosedUnderRequiredPolicy
// pins the fail-closed duty of the projection preference: where a COMPLETE
// projection EXISTS and the reader's policy REQUIRES the enrolled client
// pair, a server key replaced by a symlink to foreign material must fail
// closed on the projection — the reader may never paper over a tampered
// boundary (0428-091, the Keycloak redirected-projection CI regression;
// the identityprovider read path pins the same verdict cross-package).
func TestExistingTLSMaterialRedirectedProjectionFailsClosedUnderRequiredPolicy(t *testing.T) {
	root := t.TempDir()
	issuer := newTestIssuer(t)
	policy, err := Resolve("prod", "tempo-reader", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.AuthenticationRequired || policy.Authentication != AuthenticationMTLS {
		t.Fatal("a managed-environment mTLS reader must carry the enrolled-client duty")
	}
	serviceAccessDir := filepath.Join(root, "service-access")
	serverNames := []string{"tempo-access", "127.0.0.1", "tempo-access.access.baseharbor.internal"}
	source, err := EnsureTLSMaterial(context.Background(), issuer, policy, filepath.Join(serviceAccessDir, "pki"), serverNames...)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectGatewayMaterial(serviceAccessDir, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingTLSMaterial(policy, filepath.Join(serviceAccessDir, "pki")); err != nil {
		t.Fatal(err)
	}
	// Replace the projection's key with a symlink to foreign material: the
	// key path still RESOLVES, but the bytes are not the pair the gateway
	// mounts.
	foreign := filepath.Join(t.TempDir(), "foreign-key.pem")
	if err := os.WriteFile(foreign, []byte("not a private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(projected.ServerKey); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, projected.ServerKey); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingTLSMaterial(policy, filepath.Join(serviceAccessDir, "pki")); err == nil {
		t.Fatal("a projection whose key was redirected must fail closed under a required-authentication policy")
	}
}

func TestExistingTLSMaterialFailsClosedOnRetiredProjectedIdentity(t *testing.T) {
	root := t.TempDir()
	issuer := newTestIssuer(t)
	policy, err := Resolve("prod", "tempo-reader", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	serviceAccessDir := filepath.Join(root, "service-access")
	serverNames := []string{"tempo-access", "127.0.0.1", "tempo-access.access.baseharbor.internal"}
	if _, err := EnsureTLSMaterial(context.Background(), issuer, policy, filepath.Join(serviceAccessDir, "pki"), serverNames...); err != nil {
		t.Fatal(err)
	}
	// A projection reconciled under a laxer posture: server material only.
	relaxed := policy
	relaxed.AuthenticationRequired = false
	lax, err := EnsureTLSMaterial(context.Background(), issuer, relaxed, filepath.Join(root, "lax"), serverNames...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectGatewayMaterial(serviceAccessDir, TLSMaterial{
		Source:            PKIManagedLocal,
		CA:                lax.CA,
		ServerCertificate: lax.ServerCertificate,
		ServerKey:         lax.ServerKey,
		ServerName:        policy.ServerName,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingTLSMaterial(policy, filepath.Join(serviceAccessDir, "pki")); err == nil {
		t.Fatal("a projection without the required client identity must fail closed, never fall back to a stale pki/ pair")
	}
}

// TestExistingTLSMaterialKeepsSourceTreeResolutionWithoutPerFileProjection
// pins the TCP-gateway layout (0428-091 CI regression, valkey/shared-backend
// cells): a gateway whose runtime projection is the combined
// runtime/server.pem only (projectTCPMaterial never writes a per-file
// runtime/server-key.pem) must keep resolving exactly as before, from the
// pki/ source tree. Removing the completeness marker (server-key.pem) check
// in ExistingTLSMaterial and testing against a TCP layout turns the
// application cells red; this cell pins the rule at the boundary itself.
func TestExistingTLSMaterialKeepsSourceTreeResolutionWithoutPerFileProjection(t *testing.T) {
	root := t.TempDir()
	issuer := newTestIssuer(t)
	policy, err := Resolve("prod", "valkey-access", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	serviceAccessDir := filepath.Join(root, "service-access")
	material, err := EnsureTLSMaterial(t.Context(), issuer, policy, filepath.Join(serviceAccessDir, "pki"), "valkey-access", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the TCP layout: combined server pair, no per-file key.
	if err := os.MkdirAll(filepath.Join(serviceAccessDir, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	cert, err := os.ReadFile(material.ServerCertificate)
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(material.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serviceAccessDir, "runtime", "server.pem"), append(append([]byte(nil), cert...), key...), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := ExistingTLSMaterial(policy, filepath.Join(serviceAccessDir, "pki"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(serviceAccessDir, "pki", "server-cert.pem")
	if resolved.ServerCertificate != want {
		t.Fatalf("a layout without the per-file projection must resolve from the source tree, got %q want %q", resolved.ServerCertificate, want)
	}
}
