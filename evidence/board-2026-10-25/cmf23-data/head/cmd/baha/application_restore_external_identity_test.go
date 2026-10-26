package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func restoreRegisteredIdentityFixture(t *testing.T, retained bool) (resolvedApplication, string, string) {
	t.Helper()
	r := corelessResolvedFixture(t)
	r.FromRepository = false
	r.Store = application.Store{Root: t.TempDir(), Namespace: r.Target.Name}
	r.Manifest.Services.Identity = true
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	t.Setenv("BASEHARBOR_EXTERNAL_OIDC_ISSUER", "")
	for _, name := range []string{identityprovider.ExternalClientIDEnv, identityprovider.ExternalClientSecretFileEnv, identityprovider.ExternalCAFileEnv} {
		t.Setenv(name, "")
	}
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := registeredIdentityConsumerFixture(t, root, "restore-original-login", "fixture-private-original")
	_, _ = registeredIdentityConsumerFixture(t, root, "restore-destination-login", "fixture-private-destination")
	ref := application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC)
	t.Setenv(ref, "restore-destination-login")
	destination, err := application.ResolveExternalProviderRegistrationAt(r.TargetStateRoot, r.Manifest, capability.ProviderExternalOIDC)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(ref, "restore-original-login")
	if retained {
		if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
			t.Fatal(err)
		}
		files := application.RuntimeFilesFor(r.Store, r.Manifest)
		if err := os.MkdirAll(files.Dir, 0700); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{files.Env, files.ApplicationEnv} {
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		prepared, err := prepareManagedIdentity(t.Context(), nil, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := provisionAndVerifyManagedIdentity(t.Context(), io.Discard, prepared, nil); err != nil {
			t.Fatal(err)
		}
	}
	r.DeploymentIdentity, err = deployment.NewDeploymentIdentity(r.Target.Name, r.Manifest.ApplicationID, r.Manifest.Name, r.Manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	r.DeploymentStateRoot, err = deployment.DeploymentRoot(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	return r, secret, destination.Trust.CAReference
}

func TestRestoreBindsOriginalExternalIdentityAndProtectedDestination(t *testing.T) {
	for _, retained := range []bool{false, true} {
		for _, change := range []string{"issuer-reference", "credential", "client-override", "trust"} {
			name := "fresh-material/"
			if retained {
				name = "retained-material/"
			}
			t.Run(name+change, func(t *testing.T) {
				r, secret, otherCA := restoreRegisteredIdentityFixture(t, retained)
				ctx := machineNoninteractiveContext(t.Context())
				data := applicationRestoreData{manifest: r.Manifest, archiveRevision: strings.Repeat("a", 64)}
				planned, err := buildApplicationRestorePlan(ctx, r, data)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, effect := range planned.Effects {
					found = found || effect.Aspect == "external-identity-recovery"
				}
				if !found || planned.Scope.Attention != "Achtung" || planned.Scope.ConfirmationsRequired != 2 {
					t.Fatal("original recovery Identity was withdrawn from concrete restore consequences")
				}
				files := application.RuntimeFilesFor(r.Store, r.Manifest)
				before := map[string][]byte{}
				for _, path := range []string{files.Env, files.ApplicationEnv, filepath.Join(r.TargetStateRoot, "provider-registry.json")} {
					before[path], _ = os.ReadFile(path)
				}
				switch change {
				case "issuer-reference":
					t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "restore-destination-login")
				case "credential":
					if err := os.WriteFile(secret, []byte("fixture-private-changed"), 0600); err != nil {
						t.Fatal(err)
					}
				case "client-override":
					t.Setenv(identityprovider.ExternalClientIDEnv, "different-client")
				case "trust":
					reg, err := application.ResolveExternalProviderRegistrationAt(r.TargetStateRoot, r.Manifest, capability.ProviderExternalOIDC)
					if err != nil {
						t.Fatal(err)
					}
					ca, err := os.ReadFile(otherCA)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(reg.Trust.CAReference, append(ca, ca...), 0644); err != nil {
						t.Fatal(err)
					}
				}
				current, err := buildApplicationRestorePlan(ctx, r, data)
				if err != nil {
					t.Fatal(err)
				}
				if current.Scope.PlanID == planned.Scope.PlanID {
					t.Fatal("changed issuer/client/credential/trust reused original restore plan")
				}
				consent := &application.ProviderChangeConsent{PlanID: planned.Scope.PlanID, First: true, Second: true, DeleteData: true}
				if err := authorizeApplicationRestore(withProviderChangeConsent(ctx, consent), r, data, io.Discard); err == nil {
					t.Fatal("changed destination accepted stale restore acknowledgement")
				}
				for path, original := range before {
					after, _ := os.ReadFile(path)
					if !bytes.Equal(original, after) {
						t.Fatal("read-only restore plan/denial changed protected delivery or registry")
					}
				}
				public, _ := json.Marshal(current)
				if strings.Contains(string(public), "fixture-private") || strings.Contains(string(public), "BEGIN CERTIFICATE") {
					t.Fatal("restore consequences exposed private credential or trust bytes")
				}
			})
		}
	}
}
