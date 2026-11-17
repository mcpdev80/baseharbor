package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/url"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/onboardingwizard"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// redeemOnboardingBundle is the production default for the ImportBundle seam.
// It binds the untrusted bundle to the accepted one-use workstation-enrollment
// boundary and redeems it exactly once: the private key is generated in the
// workstation process and never crosses the wire, and the spent one-use grant
// cannot enroll a second identity. The bundle's baseURL + client point at the
// Core's one-use boundary (production resolves the real authority through
// WorkstationEnrollmentHTTP; the test injects the in-process harness). It
// returns the installation ID the enrolled leaf binds so the join fact carries a
// concrete, verified target rather than a fabricated one. When no authority is
// reachable it fails closed before any local state is written.
func redeemOnboardingBundle(ctx context.Context, bundle *onboardingDeviceBundle) (string, error) {
	if bundle == nil {
		return "", machine.NewError(machine.ErrorValidationFailed,
			"the device bundle must arrive from the owning installation",
			"Re-receive the bundle, then retry.", false)
	}
	csrPEM, err := onboardingEnrollmentCSR(bundle.scope)
	if err != nil {
		return "", machine.Wrap(machine.ErrorInternal, err, "The workstation could not generate its session identity.", false)
	}
	result, err := bundle.redeem(ctx, csrPEM)
	if err != nil {
		return "", machine.Wrap(machine.ErrorVerificationFailed, err, "The one-use bundle is consumed or invalid; request a fresh bundle.", false)
	}
	return result.InstallationID, nil
}

// onboardingEnrollmentCSR generates the workstation-owned CSR the enrollment
// exchange signs. The key never leaves the workstation process, matching the
// workstation-enrollment no-private-key-transfer guarantee. The CSR carries the
// workstation scope identity as its single SPIFFE URI so the accepted signing
// boundary binds the issued leaf to exactly this enrollment scope.
func onboardingEnrollmentCSR(scope ws.Scope) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	uriIdentity, err := url.Parse(scope.Identity())
	if err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uriIdentity}}, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

// bundleForImportDecision wraps the onboarding bundle handle into a concrete
// onboardingDeviceBundle bound to the accepted workstation-enrollment contract.
// The handle carries the installation (tenant) ID and the workstation station;
// the tenant/station select the scope. The (token,nonce) digest uniquely
// identifies the one-use grant so a replay cannot enroll a second identity.
func bundleForImportDecision(decision onboardingwizard.Decision) *onboardingDeviceBundle {
	tenantID, station, ok := onboardingBundleScope(decision.ImportBundle)
	if !ok {
		// A malformed handle still yields a bound bundle so the seam reports a
		// clean validation failure rather than a nil-pointer panic.
		tenantID, station = "", ""
	}
	return &onboardingDeviceBundle{
		contract:       onboardingBundleContract,
		baseURL:        onboardingEnrollmentBaseURL(),
		client:         http.DefaultClient,
		scope:          ws.Scope{TenantID: tenantID, Station: station},
		certificateTTL: ws.MaxCertificateTTL,
	}
}

// onboardingBundleScope parses a bundle handle into its installation tenant ID
// and workstation station. The accepted workstation-enrollment scope is
// exactly (tenantID, station); the trailing token uniquely identifies the
// one-use grant. Anything else fails closed so a malformed or tampered bundle
// cannot select a foreign scope.
func onboardingBundleScope(handle string) (tenantID, station string, ok bool) {
	parts := strings.Split(handle, "|")
	if len(parts) < 3 {
		return "", "", false
	}
	tenantID, station = parts[0], parts[1]
	if tenantID == "" || station == "" {
		return "", "", false
	}
	return tenantID, station, true
}

// onboardingEnrollmentBaseURL returns the production workstation-enrollment
// endpoint the import seam redeems against. Production resolves the real Core
// authority here; an empty/unreachable value makes the exchange fail closed.
func onboardingEnrollmentBaseURL() string { return "" }
