package applicationbackup

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestIdentityBindingSurvivesRestoreCycle guards H-V39/F6: an application that
// declares identity must keep its portable OIDC binding intent across a backup
// -> restore round trip, and its recovery contributor must be discovered with a
// portable classification. The manifest round trip is the restore boundary;
// application.metadata is the only source the reconstructed identity reads from.
func TestIdentityBindingSurvivesRestoreCycle(t *testing.T) {
	m := application.New("demo", "dev", true, false, true)
	m = application.WithIdentity(m)
	m = application.WithIdentityCallbackPath(m, "/auth/callback")
	m = application.WithIdentityLogoutPath(m, "/auth/logout")
	m = application.WithIdentityAuthentication(m, "optional", []string{"totp", "webauthn"}, false)

	if !application.HasIdentity(m) {
		t.Fatal("test setup: identity must be declared")
	}

	// 1) Discovery must classify the identity contributor for an identity-declaring app.
	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatalf("identity discovery failed: %v", err)
	}

	var identityContributor *RecoveryContributor
	for i := range selection.Contributors {
		if selection.Contributors[i].StateClass == StateIdentity {
			identityContributor = &selection.Contributors[i]
			break
		}
	}
	if identityContributor == nil {
		t.Fatal("regression: identity.oidc contributor missing from recovery manifest for identity-declaring app")
	}
	if identityContributor.LogicalResource != "application-identity" {
		t.Fatalf("identity logical resource = %q, want application-identity", identityContributor.LogicalResource)
	}
	if identityContributor.Ownership != "application" {
		t.Fatalf("identity ownership = %q, want application for default keycloak placement", identityContributor.Ownership)
	}
	if identityContributor.Support != RecoveryUnsupported {
		t.Fatalf("identity support = %q, want unsupported (scoped export not implemented; portable intent reconstructed)", identityContributor.Support)
	}

	// 2) The portable manifest round trip is the restore boundary. The identity
	//    binding intent lives in application.metadata and must survive it.
	entry, err := ApplicationManifestPayloadEntry(m)
	if err != nil {
		t.Fatalf("application manifest payload entry failed: %v", err)
	}
	payload := Payload{
		Manifest: Manifest{SchemaVersion: SchemaVersion, Application: "demo", Environment: "dev"},
		Entries:  []PayloadEntry{entry},
	}
	restored, err := ApplicationManifestFromPayload(payload)
	if err != nil {
		t.Fatalf("restore: application manifest payload rejected identity: %v", err)
	}
	if !application.HasIdentity(restored) {
		t.Fatal("regression: identity binding intent lost across restore round trip")
	}
	if len(restored.Identity.CallbackPaths) != 1 || restored.Identity.CallbackPaths[0] != "/auth/callback" {
		t.Fatalf("restore: callback paths not preserved: %#v", restored.Identity.CallbackPaths)
	}
	if restored.Identity.Authentication.MFA != "optional" {
		t.Fatalf("restore: MFA requirement not preserved: %q", restored.Identity.Authentication.MFA)
	}
	if len(restored.Identity.Authentication.Methods) != 2 {
		t.Fatalf("restore: auth methods not preserved: %#v", restored.Identity.Authentication.Methods)
	}
}

// TestIdentityRecoveryManifestRoundTripPreservesContributor guards that a
// recovery manifest that lists the identity contributor round trips without
// dropping or reclassifying it, so restore cannot silently drop identity scope.
func TestIdentityRecoveryManifestRoundTripPreservesContributor(t *testing.T) {
	m := application.New("demo", "dev", true, false, true)
	m = application.WithIdentity(m)

	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	entry, err := RecoveryManifestPayloadEntry(selection)
	if err != nil {
		t.Fatalf("recovery manifest payload entry failed: %v", err)
	}
	payload := Payload{Entries: []PayloadEntry{entry}}
	manifest, found, err := RecoveryManifestFromPayload(payload)
	if err != nil {
		t.Fatalf("recovery manifest round trip failed: %v", err)
	}
	if !found {
		t.Fatal("recovery manifest not found in payload")
	}

	var count int
	for _, c := range manifest.Contributors {
		if c.StateClass == StateIdentity {
			count++
			if c.Selected {
				t.Fatal("identity.oidc must never be selected by default: scoped export is unsupported")
			}
		}
	}
	if count == 0 {
		t.Fatal("regression: identity contributor dropped from recovery manifest round trip")
	}
}

// TestIdentityExternalPlacementIsExternal guards that an externally-placed
// identity-directory is classified external and explicitly excluded from
// BaseHarbor recovery ownership while the portable binding intent is still
// reconstructed on restore.
func TestIdentityExternalPlacementIsExternal(t *testing.T) {
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	t.Setenv("BASEHARBOR_EXTERNAL_OIDC_ISSUER", "https://external.example.com/realms/app")

	m := application.New("demo", "dev", true, false, true)
	m = application.WithIdentity(m)

	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatalf("external identity discovery failed: %v", err)
	}

	var external bool
	for _, c := range selection.Contributors {
		if c.StateClass == StateIdentity {
			if c.Ownership != "external" {
				t.Fatalf("external identity ownership = %q, want external", c.Ownership)
			}
			if c.Support != RecoveryExternal {
				t.Fatalf("external identity support = %q, want external", c.Support)
			}
			if !c.ExplicitlyExcluded {
				t.Fatal("external identity must be explicitly excluded from recovery ownership")
			}
			external = true
		}
	}
	if !external {
		t.Fatal("regression: external identity contributor missing from recovery manifest")
	}

	// The portable binding intent must still survive restore even for an external
	// identity-directory, so the reconstructed OIDC binding re-verifies.
	entry, err := ApplicationManifestPayloadEntry(m)
	if err != nil {
		t.Fatalf("external identity manifest payload entry failed: %v", err)
	}
	payload := Payload{
		Manifest: Manifest{SchemaVersion: SchemaVersion, Application: "demo", Environment: "dev"},
		Entries:  []PayloadEntry{entry},
	}
	restored, err := ApplicationManifestFromPayload(payload)
	if err != nil {
		t.Fatalf("external identity restore failed: %v", err)
	}
	if !application.HasIdentity(restored) {
		t.Fatal("regression: external identity binding intent lost across restore")
	}
}

// TestIdentityDiscoveryFailsClosedWithoutReference guards that identity
// discovery cannot silently pass an unresolved external identity reference, which
// would reconstruct identity from an unknown provider on restore.
func TestIdentityDiscoveryFailsClosedWithoutReference(t *testing.T) {
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	// No BASEHARBOR_EXTERNAL_OIDC_ISSUER set.

	m := application.New("demo", "dev", true, false, true)
	m = application.WithIdentity(m)

	_, err := DiscoverManifestRecovery(m)
	if err == nil {
		t.Fatal("regression: identity discovery must fail closed without a resolved external OIDC reference")
	}
	if !strings.Contains(err.Error(), "oidc") && !strings.Contains(err.Error(), "OIDC") {
		t.Fatalf("expected OIDC reference error, got: %v", err)
	}
}
