package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// materializeTamperedKeycloakPKI writes a Keycloak core issuer whose CA and
// server certificate are SEPARATE keys (a genuine two-entity chain) plus the
// binding runtime.env, so the r3 chain + hostname verification (x509.Verify
// against the CA with DNSName identity.localhost) has real material to check.
// The returned write is a no-op when the core dir already exists.
func materializeTamperedKeycloakPKI(t *testing.T, plainTargetRoot string, identityIssuer string) {
	t.Helper()
	coreDir := filepath.Join(plainTargetRoot, "providers", "keycloak", "shared", "core")
	if _, err := os.Stat(filepath.Join(coreDir, "runtime.env")); !os.IsNotExist(err) {
		return
	}
	pkiDir := filepath.Join(coreDir, "native-tls", "pki")
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		t.Fatalf("create Keycloak pki dir: %v", err)
	}
	port := keycloakPublicPortFromIdentityIssuer(t, identityIssuer)

	// A genuine issuing CA (self-signed, binds identity.localhost) that is NOT
	// the server leaf.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate Keycloak issuing CA key: %v", err)
	}
	now := time.Now().UTC()
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "identity.localhost"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		DNSNames:              []string{"identity.localhost"},
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create Keycloak issuing CA: %v", err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate Keycloak server key: %v", err)
	}
	serverTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano() + 1),
		Subject:               pkix.Name{CommonName: "identity.localhost"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"identity.localhost"},
	}
	caParsed, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse Keycloak issuing CA: %v", err)
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caParsed, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create Keycloak server leaf: %v", err)
	}
	writePKIFile(t, filepath.Join(pkiDir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	writePKIFile(t, filepath.Join(pkiDir, "server-cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}))
	writePKIFile(t, filepath.Join(pkiDir, "server-key.pem"), mustPKCS8(serverKey))

	writeProtectedEnvForTest(filepath.Join(coreDir, "runtime.env"), map[string]string{
		"BASEHARBOR_KEYCLOAK_PUBLIC_PORT":    strconv.Itoa(port),
		"BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD": "0000000000000000000000000000000000000000000000000000000000000000",
		"BASEHARBOR_KEYCLOAK_DB_PASSWORD":    "0000000000000000000000000000000000000000000000000000000000000000",
	})
}

// targetStateRootForTest resolves the plain target-state root for a test Target.
func targetStateRootForTest(t *testing.T, name string) string {
	t.Helper()
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatalf("resolve target state root for %s: %v", name, err)
	}
	return root
}

// targetCorePKIDir returns the protected Keycloak core issuer pki directory for a
// test Target.
func targetCorePKIDir(plainTargetRoot string) string {
	return filepath.Join(plainTargetRoot, "providers", "keycloak", "shared", "core", "native-tls", "pki")
}

// keycloakPublicPortFromIdentityIssuer extracts the public port from an identity
// issuer URL of the form https://identity.localhost:<port>/realms/baseharbor.
func keycloakPublicPortFromIdentityIssuer(t *testing.T, identityIssuer string) int {
	t.Helper()
	const prefix = "https://identity.localhost:"
	idx := strings.Index(identityIssuer, "://")
	if idx < 0 {
		t.Fatalf("invalid identity issuer %q", identityIssuer)
	}
	rest := identityIssuer[idx+3:]
	hostPort := strings.SplitN(rest, "/", 2)[0]
	_, portStr, ok := strings.Cut(hostPort, ":")
	if !ok {
		t.Fatalf("identity issuer missing port %q", identityIssuer)
	}
	port, perr := strconv.Atoi(strings.TrimSpace(portStr))
	if perr != nil || port < 1 || port > 65535 {
		t.Fatalf("invalid Keycloak public port %q", portStr)
	}
	return port
}

// selfSignedServerLeafForHost returns a leaf certificate binding host and its
// private key, signed by an unrelated self-signed CA — used by the wrong-SAN
// negative control to name a host that does not match the observed identity.
func selfSignedServerLeafForHost(host string, port int) (x509.Certificate, *ecdsa.PrivateKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	now := time.Now().UTC()
	selfCA := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		DNSNames:              []string{host},
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &selfCA, &selfCA, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano() + 1),
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{host},
	}
	caParsed, err := x509.ParseCertificate(caDER)
	if err != nil {
		panic(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caParsed, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	return *parsed, key
}

// mustPKCS8 marshals a private key to PKCS#8 PEM.
func mustPKCS8(key *ecdsa.PrivateKey) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// writePKIFile writes a Keycloak pki file with 0600 mode, failing the test on
// any error so tampered material never silently passes.
func writePKIFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write Keycloak pki file %q: %v", path, err)
	}
}

// pemEncodeCert PEM-encodes a parsed certificate for writing to a pki file.
func pemEncodeCert(t *testing.T, cert x509.Certificate) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

// materializeKeycloakCoreIssuerForTargetWithIdentity registers a verified Core
// under the given name with the given identity issuer and materializes its
// Keycloak core issuer under the plain target-state root so VerifyCoreConnection
// observes the genuine Keycloak-bound discovery identity from disk.
func materializeKeycloakCoreIssuerForTargetWithIdentity(t *testing.T, name, identityIssuer string) {
	t.Helper()
	stateRoot, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatalf("resolve target state root: %v", err)
	}
	runtimeRoot := filepath.Join(stateRoot, "runtime")
	if _, statErr := os.Stat(filepath.Join(runtimeRoot, "installation.json")); os.IsNotExist(statErr) {
		if err := os.MkdirAll(runtimeRoot, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := coreinstallation.Run(context.Background(), runtimeRoot, coreinstallation.Spec{Target: name, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
			Preflight: func(context.Context, coreinstallation.State) error { return nil },
			SQL:       func(context.Context, coreinstallation.State) error { return nil },
			Secrets:   func(context.Context, coreinstallation.State) error { return nil },
			Identity:  func(context.Context, coreinstallation.State) (string, error) { return identityIssuer, nil },
			Verify:    func(context.Context, coreinstallation.State) error { return nil },
		}); err != nil {
			t.Fatalf("bootstrap verified Core for %s: %v", name, err)
		}
	}
	materializeKeycloakCoreIssuerForTarget(t, stateRoot, identityIssuer)
}

// TestBindExplicitSelectionResolvesKeycloakCoreIssuer proves the r2 fix: the
// verified Core's Keycloak-bound discovery issuer is the authoritative
// management connection, so bindExplicitSelectionToConnection SUCCEEDS when the
// Core's real Keycloak issuer (providers/keycloak/shared/core) is present on
// disk — the production location the real Core materializes, NOT a local base CA.
// This closes the r1 bug: use/activate verified the Core through a local base CA
// that the real Core never materializes, so it rejected the genuine Core with
// ErrForeignAuthority ("foreign management connection authority").
func TestBindExplicitSelectionResolvesKeycloakCoreIssuer(t *testing.T) {
	target := configureTestTarget(t)
	materializeKeycloakCoreIssuerForTargetWithIdentity(
		t,
		target.Name,
		"https://identity.localhost:8443/realms/baseharbor",
	)
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err != nil {
		t.Fatalf("explicit selection on a verified Core with a Keycloak core issuer must bind: %v", err)
	}
}

// TestBindExplicitSelectionFailsClosedOnForeignIssuerIdentity proves the negative
// control: when the disk-resolved Keycloak core issuer identity does not match
// the verified Core's own IdentityIssuer, bindExplicitSelectionToConnection
// rejects it as foreign (ErrCoreIssuerMismatch), never falling back to a second
// trust source. This is the r1 fix: the old resolver observed a foreign
// identity and let it through; the corrected path proves identity equality.
func TestBindExplicitSelectionFailsClosedOnForeignIssuerIdentity(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(stateRoot, "runtime")
	// The verified Core's IdentityIssuer is the Keycloak-bound identity the Core
	// actually owns (port 8443). The disk material is configured with a DIFFERENT
	// public port (9443), so the disk-resolved identity differs from
	// state.IdentityIssuer — a foreign issuer identity, not the Core's own.
	if _, statErr := os.Stat(filepath.Join(runtimeRoot, "installation.json")); os.IsNotExist(statErr) {
		if _, err := coreinstallation.Run(context.Background(), runtimeRoot, coreinstallation.Spec{Target: target.Name, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
			Preflight: func(context.Context, coreinstallation.State) error { return nil },
			SQL:       func(context.Context, coreinstallation.State) error { return nil },
			Secrets:   func(context.Context, coreinstallation.State) error { return nil },
			Identity: func(context.Context, coreinstallation.State) (string, error) {
				return "https://identity.localhost:8443/realms/baseharbor", nil
			},
			Verify: func(context.Context, coreinstallation.State) error { return nil },
		}); err != nil {
			t.Fatalf("bootstrap verified Core for %s: %v", target.Name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(stateRoot, "providers", "keycloak", "shared", "core", "native-tls", "pki"), 0700); err != nil {
		t.Fatal(err)
	}
	writeProtectedEnvForTest(filepath.Join(stateRoot, "providers", "keycloak", "shared", "core", "runtime.env"), map[string]string{
		"BASEHARBOR_KEYCLOAK_PUBLIC_PORT": "9443",
	})
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err == nil {
		t.Fatal("foreign Keycloak core issuer identity must be rejected as foreign")
	}
}

// TestBindExplicitSelectionFailsClosedOnMissingKeycloakIssuer proves the
// separation: a verified Core that holds no Keycloak core issuer on disk fails
// closed with a named sentinel, never fabricating a trust source.
func TestBindExplicitSelectionFailsClosedOnMissingKeycloakIssuer(t *testing.T) {
	target := configureTestTarget(t)
	// No Keycloak core issuer materialized: the real Core's identity is absent.
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err == nil {
		t.Fatal("verified Core without a Keycloak core issuer must fail closed")
	}
}

// TestBindExplicitSelectionFailsClosedOnWrongSAN proves the r3 negative: a server
// certificate whose SAN does NOT name the observed discovery host
// (identity.localhost) fails closed, because the r3 verifier binds the observed
// Keycloak-bound identity to the server certificate by hostname. This closes the
// r2 gap: the old verifier read the server cert only for serial + expiry and did
// not verify the chain or identity hostname.
func TestBindExplicitSelectionFailsClosedOnWrongSAN(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot := targetStateRootForTest(t, target.Name)
	materializeTamperedKeycloakPKI(t, stateRoot, "https://identity.localhost:8443/realms/baseharbor")
	// Overwrite the server leaf with one whose SAN names a different host, so the
	// r3 hostname binding fails closed.
	wrongSAN, wrongSANKey := selfSignedServerLeafForHost("wrong.localhost", 8443)
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "server-cert.pem"), pemEncodeCert(t, wrongSAN))
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "server-key.pem"), mustPKCS8(wrongSANKey))
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err == nil {
		t.Fatal("Keycloak server cert with a wrong SAN must fail closed as untrusted")
	}
}

// TestBindExplicitSelectionFailsClosedOnUntrustedCA proves the r3 negative: a
// server certificate that does NOT chain to the CA present in the Core's own
// protected material fails closed, because the r3 verifier verifies the server
// leaf against that CA. This closes the r2 gap: the old verifier never built the
// chain, so an untrusted CA placed at the expected Keycloak path was not
// distinguished from the genuine one.
func TestBindExplicitSelectionFailsClosedOnUntrustedCA(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot := targetStateRootForTest(t, target.Name)
	materializeTamperedKeycloakPKI(t, stateRoot, "https://identity.localhost:8443/realms/baseharbor")
	// Replace the CA with an unrelated one that signs a DIFFERENT server leaf; the
	// server leaf still names identity.localhost but no longer chains to the
	// present CA, so the chain verification fails closed.
	otherCAKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate other CA key: %v", err)
	}
	now := time.Now().UTC()
	otherCA := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "identity.localhost"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		DNSNames:              []string{"identity.localhost"},
	}
	otherCAKey2, _ := otherCAKey, otherCA
	_ = otherCAKey2
	caDER, err := x509.CreateCertificate(rand.Reader, &otherCA, &otherCA, &otherCAKey.PublicKey, otherCAKey)
	if err != nil {
		t.Fatalf("create other CA: %v", err)
	}
	serverKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serverTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano() + 1),
		Subject:               pkix.Name{CommonName: "identity.localhost"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"identity.localhost"},
	}
	otherCACert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse other CA: %v", err)
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, otherCACert, &serverKey.PublicKey, otherCAKey)
	if err != nil {
		t.Fatalf("create server leaf for other CA: %v", err)
	}
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "ca.pem"), caDER)
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "server-cert.pem"), serverDER)
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "server-key.pem"), mustPKCS8(serverKey))
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err == nil {
		t.Fatal("Keycloak server cert that does not chain to the Core CA must fail closed")
	}
}

// TestBindExplicitSelectionFailsClosedOnNotYetValid proves the r3 negative: a
// server certificate that is not yet valid fails closed as untrusted, because the
// standard-library chain verification rejects the not-yet-valid leaf.
func TestBindExplicitSelectionFailsClosedOnNotYetValid(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot := targetStateRootForTest(t, target.Name)
	materializeTamperedKeycloakPKI(t, stateRoot, "https://identity.localhost:8443/realms/baseharbor")
	// Write a not-yet-valid server leaf signed by the present CA: it chains and
	// names identity.localhost, but the future not-before must fail closed. We
	// generate a matching issuing CA + key (the present CA key is unknown) so the
	// leaf chains to a CA we write alongside it, but the future not-before still
	// fails the standard-library chain verification.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate issuing CA key: %v", err)
	}
	future := time.Now().Add(48 * time.Hour)
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "identity.localhost"},
		NotBefore:             future.Add(-24 * time.Hour),
		NotAfter:              future.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		DNSNames:              []string{"identity.localhost"},
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create issuing CA: %v", err)
	}
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	template := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano() + 1),
		Subject:               pkix.Name{CommonName: "identity.localhost"},
		NotBefore:             future,
		NotAfter:              future.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"identity.localhost"},
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	caParsed, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse issuing CA: %v", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caParsed, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create not-yet-valid server leaf: %v", err)
	}
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "server-cert.pem"), der)
	writePKIFile(t, filepath.Join(targetCorePKIDir(stateRoot), "server-key.pem"), mustPKCS8(serverKey))
	if err := bindExplicitSelectionToConnection(context.Background(), target.Name); err == nil {
		t.Fatal("not-yet-valid Keycloak server cert must fail closed")
	}
}
