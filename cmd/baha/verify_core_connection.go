package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// ErrCoreIssuerMismatch is returned when the verified Core's Keycloak-bound
// core issuer identity does not match state.IdentityIssuer. It is the
// authority-resolution correction for card #133: the real Core keeps its
// management-connection identity at its Keycloak discovery issuer
// (state.IdentityIssuer), NOT at a local base CA, so a resolver that observes a
// different authority (OpenBao service PKI, local base CA) fails closed here
// mapped to ErrForeignAuthority below.
var ErrCoreIssuerMismatch = errors.New("verified Core issuer identity mismatch, fails closed")

// ErrKeycloakCoreIssuerUnavailable is returned when the verified Core holds no
// Keycloak core issuer at all (a Core without an Identity capability, a root
// that holds no verified Core, or unreadable Core TLS material). A missing
// core-connection authority selects no local execution.
var ErrKeycloakCoreIssuerUnavailable = errors.New("Keycloak core issuer unavailable, fails closed")

// keycloakCoreIssuerIssuerRef is the Keycloak public discovery hostname that the
// verified Core binds its core issuer SAN/CN to; the public discovery URL is
// https://<host>:<publicPort>/realms/baseharbor and is compared verbatim against
// the verified Core's state.IdentityIssuer.
const keycloakCoreIssuerIssuerRef = "identity.localhost"

// VerifyCoreConnection proves the verified Core's live management connection
// through the real Core identity: its Keycloak core issuer, observed purely from
// the protected on-disk TLS material the Core materialized at runtime
// (targets/<name>/providers/keycloak/shared/core), NOT through a local base CA.
//
// The v0.4.27 use/activate path (cmd/baha/use_target.go) previously resolved the
// issuer through coreinstallation.ResolverForTargetSelection, which observes a
// local base CA under <targetStateRoot>/service-access/issuer. The real Core
// (Keycloak/OpenBao) never materializes a local base CA there — its
// management-connection identity lives at the Keycloak discovery issuer
// (state.IdentityIssuer, an HTTPS URL such as
// https://identity.localhost:<port>/realms/baseharbor). ResolverForTargetSelection
// therefore observed a foreign identity and ConnectionSelection.Resolve rejected
// the genuine Core with ErrForeignAuthority ("foreign management connection
// authority").
//
// VerifyCoreConnection reads the Core's public TLS material from disk and
// derives the Keycloak-bound discovery identity
// (https://identity.localhost:<port>/realms/baseharbor from the public CA
// SAN/CN and runtime.env BASEHARBOR_KEYCLOAK_PUBLIC_PORT), then proves:
//   - the Core owns a Keycloak core issuer on disk (ErrKeycloakCoreIssuerUnavailable
//     when absent/unreadable);
//   - the observed identity EQUALS state.IdentityIssuer (ErrCoreIssuerMismatch,
//     mapped to ErrForeignAuthority when the observed authority is foreign);
//   - the observed public CA is not expired (a zero/past not-after fails closed
//     as not-ready).
//
// PlainTargetRoot is the plain target-state root the Core owns
// (deployment.TargetStateRoot(name)); the Keycloak core issuer lives under
// providers/keycloak/shared/core there, never under the runtime/ sub-root the
// old resolver read.
//
// This function lives in cmd/baha, NOT in coreinstallation, because it consumes
// the on-disk Core TLS boundary and must stay in the application layer alongside
// bindExplicitSelectionToConnection. The caller keeps the authority boundary;
// this is only the production read.
func VerifyCoreConnection(
	ctx context.Context,
	verifiedInstallation coreinstallation.State,
	plainTargetRoot string,
) error {
	verified := coreinstallation.AssertStateIsGenuineInstallation(verifiedInstallation)
	if verified != nil {
		return coreinstallation.ErrUnverifiableInstallation
	}
	if verifiedInstallation.IdentityIssuer == "" {
		return ErrKeycloakCoreIssuerUnavailable
	}
	dir := coreKeycloakCoreIssuerDir(plainTargetRoot)
	observed, err := observedKeycloakCoreIssuer(ctx, dir)
	if err != nil {
		return ErrKeycloakCoreIssuerUnavailable
	}
	// The observed Keycloak-bound identity must equal the verified Core's own
	// IdentityIssuer. A resolver that observes a different authority (OpenBao
	// service PKI, a local base CA) is foreign and fails closed.
	if observed.Identity != verifiedInstallation.IdentityIssuer {
		return ErrCoreIssuerMismatch
	}
	// Liveness: a zero/past/future expiry fails closed as not-ready. The
	// observed public CA not-after is the freshness bound the single policy
	// checks; there is no live runtime to query on this read-only boundary.
	if observed.IssuedCertificate.ExpiresAt.IsZero() || time.Now().After(observed.IssuedCertificate.ExpiresAt) {
		return ErrKeycloakCoreIssuerUnavailable
	}
	// Certificate-chain + hostname binding (card #133 r3): the observed
	// Keycloak-bound discovery identity is only genuine when the verified Core's
	// public server certificate is cryptographically bound to that identity. The
	// r2 verifier read the server certificate only for serial + expiry and did
	// NOT verify the chain or the identity hostname, so a wrong-host certificate
	// or an untrusted CA placed at the expected Keycloak material path was
	// indistinguishable from the genuine one. Verify the server certificate
	// against the Keycloak CA from the SAME protected material using the
	// standard library: a chain that does not build to the Core's own CA, a
	// certificate whose SAN/CN does not name the observed discovery host, a
	// foreign CA, an expired or not-yet-valid leaf, or any unparseable material
	// fails closed with ErrKeycloakCoreIssuerUnavailable. The observed identity
	// host (keycloakCoreIssuerIssuerRef, e.g. identity.localhost) is the DNSName
	// the chain verification must match; the observed public CA not-after is the
	// additional freshness bound checked above.
	if err := verifyKeycloakServerCertificateChain(dir); err != nil {
		return err
	}
	return nil
}

// verifyKeycloakServerCertificateChain proves that the verified Core's public
// Keycloak server certificate binds the observed discovery identity: it verifies
// the server certificate against the Keycloak CA from the SAME protected
// material (targets/<name>/providers/keycloak/shared/core/native-tls/pki) with
// the standard library x509.Verify, binding the observed discovery hostname
// (keycloakCoreIssuerIssuerRef, e.g. identity.localhost) as the DNSName and the
// server-auth EKU. A missing/unreadable CA, an unparseable chain, a chain that
// does not build to the Core's own CA, a hostname that does not appear in the
// leaf SAN/CN, an expired or not-yet-valid leaf, or a foreign CA all fail closed
// with ErrKeycloakCoreIssuerUnavailable. This is the read-only production read;
// it changes no Core state, it only confirms the on-disk material is genuine.
func verifyKeycloakServerCertificateChain(dir string) error {
	caPath := filepath.Join(dir, "native-tls", "pki", "ca.pem")
	serverCertPath := filepath.Join(dir, "native-tls", "pki", "server-cert.pem")
	caDER, err := os.ReadFile(caPath)
	if err != nil {
		return ErrKeycloakCoreIssuerUnavailable
	}
	caBlock, _ := pem.Decode(caDER)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" {
		return ErrKeycloakCoreIssuerUnavailable
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return ErrKeycloakCoreIssuerUnavailable
	}
	serverDER, err := os.ReadFile(serverCertPath)
	if err != nil {
		return ErrKeycloakCoreIssuerUnavailable
	}
	serverBlock, _ := pem.Decode(serverDER)
	if serverBlock == nil || serverBlock.Type != "CERTIFICATE" {
		return ErrKeycloakCoreIssuerUnavailable
	}
	serverCert, err := x509.ParseCertificate(serverBlock.Bytes)
	if err != nil {
		return ErrKeycloakCoreIssuerUnavailable
	}
	// The observed public CA not-after is the additional freshness bound the
	// caller already checked; here the chain is verified including the
	// not-before/not-after window of the leaf, so a not-yet-valid leaf is
	// rejected as untrusted material.
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err := serverCert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   keycloakCoreIssuerIssuerRef,
	}); err != nil {
		return ErrKeycloakCoreIssuerUnavailable
	}
	return nil
}

// coreKeycloakCoreIssuerDir returns the Core's protected Keycloak core issuer
// directory under the plain target-state root.
func coreKeycloakCoreIssuerDir(plainTargetRoot string) string {
	return filepath.Join(plainTargetRoot, "providers", "keycloak", "shared", "core")
}

// observedKeycloakCoreIssuer reads the verified Core's Keycloak core issuer
// public identity purely from the protected on-disk TLS material the Core
// materialized at runtime: the public CA (native-tls/pki) and the runtime
// environment (runtime.env) that bind its public discovery URL. It observes the
// connection identity (IssuerReference) and its base CA serial + expiry, the
// same read-only shape production uses — without issuing, renewing, revoking or
// rewriting the Core identity, and without a live Keycloak runtime. A missing,
// unreadable or malformed authority fails closed with ErrKeycloakCoreIssuerUnavailable.
func observedKeycloakCoreIssuer(
	_ context.Context,
	dir string,
) (serviceaccess.CertificateStatus, error) {
	publicPort, err := readKeycloakPublicPort(filepath.Join(dir, "runtime.env"))
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	policy, err := serviceaccess.Resolve("prod", "keycloak-public", serviceaccess.AuthenticationNative)
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	policy.ServerName = keycloakCoreIssuerIssuerRef
	material, err := serviceaccess.ExistingTLSMaterial(
		policy,
		filepath.Join(dir, "native-tls", "pki"),
	)
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	cert, err := readTLSCert(material.ServerCertificate)
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	publicURL := "https://" + keycloakCoreIssuerIssuerRef + ":" + strconv.Itoa(publicPort)
	issuerRef := publicURL + "/realms/baseharbor"
	return serviceaccess.CertificateStatus{
		Identity: issuerRef,
		IssuedCertificate: serviceaccess.IssuedCertificate{
			Serial:    cert.SerialNumber.Text(16),
			ExpiresAt: cert.NotAfter.UTC(),
		},
		ObservedAt: time.Now().UTC(),
	}, nil
}

// readKeycloakPublicPort parses runtime.env for the Core's public discovery
// port (BASEHARBOR_KEYCLOAK_PUBLIC_PORT). Missing or malformed material fails
// closed.
func readKeycloakPublicPort(path string) (publicPort int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "BASEHARBOR_KEYCLOAK_PUBLIC_PORT" {
			continue
		}
		port, perr := strconv.Atoi(strings.TrimSpace(value))
		if perr != nil || port < 1 || port > 65535 {
			return 0, fmt.Errorf("invalid Keycloak public port %q", value)
		}
		return port, nil
	}
	return 0, fmt.Errorf("missing BASEHARBOR_KEYCLOAK_PUBLIC_PORT in runtime env")
}

// readTLSCert parses the Core's public Keycloak issuer certificate from disk.
func readTLSCert(pemPath string) (*x509.Certificate, error) {
	data, err := os.ReadFile(pemPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("unreadable Keycloak core issuer public certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unparseable Keycloak core issuer public certificate")
	}
	return cert, nil
}
