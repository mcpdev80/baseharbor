package application

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

func globalExternalOIDCFixture(t *testing.T) (string, string, Manifest) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	t.Setenv("BASEHARBOR_EXTERNAL_OIDC_ISSUER", "")
	t.Setenv(ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "company-login")
	reg := externalprovider.Registration{ID: "company-login", ProviderID: "company/oidc", Endpoint: "https://actual-issuer.example.test", Provider: capability.ExternalOIDC, CredentialRef: "file:///run/secrets/company-client", Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem}}
	if err := RegisterExternalProvider(reg); err != nil {
		t.Fatal(err)
	}
	target, err := deployment.TargetStateRoot("test")
	if err != nil {
		t.Fatal(err)
	}
	m := New("app", "dev", false, false, false)
	m.Services.Identity = true
	return root, target, m
}

func TestGlobalExternalOfferResolutionIsReadOnlyAndSeparatesReferenceFromIssuer(t *testing.T) {
	root, target, m := globalExternalOIDCFixture(t)
	before := map[string][]byte{}
	for _, name := range []string{providerRegistryFile, externalProviderRegistryFile} {
		before[name], _ = os.ReadFile(filepath.Join(root, name))
	}
	reg, err := ResolveExternalProviderRegistrationAt(target, m, capability.ProviderExternalOIDC)
	if err != nil || reg.ID != "company-login" || reg.Endpoint != "https://actual-issuer.example.test" {
		t.Fatalf("reference ID was not resolved through actual registered owner: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("read-only resolution allocated Target state")
	}
	for name, data := range before {
		after, _ := os.ReadFile(filepath.Join(root, name))
		if !bytes.Equal(data, after) {
			t.Fatal("read-only resolution changed canonical registration owner")
		}
	}
	preview, err := PreviewReferenceProviderRegistryAt(target, m)
	if err != nil || len(preview.Bindings) != 1 || len(preview.Instances) != 1 {
		t.Fatalf("canonical global offer absent from actual read-only Target preview: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("read-only preview allocated Target state")
	}
	if err := RetainPendingProviderOwnersAt(target, m); err != nil {
		t.Fatal(err)
	}
	pending, err := (capability.RegistryStore{Path: filepath.Join(target, providerRegistryFile)}).Load()
	if err != nil || len(pending.Instances) != 1 || len(pending.Bindings) != 0 {
		t.Fatal("pending projection published an unverified consumer binding")
	}
	if err := ReconcileReferenceProviderRegistryAt(target, m); err != nil {
		t.Fatal(err)
	}
	applied, err := (capability.RegistryStore{Path: filepath.Join(target, providerRegistryFile)}).Load()
	if err != nil || len(applied.Bindings) != 1 || applied.Bindings[0].ProviderInstanceID != preview.Bindings[0].ProviderInstanceID {
		t.Fatal("normal apply did not preserve the selected canonical external offer")
	}
	for name, data := range before {
		after, _ := os.ReadFile(filepath.Join(root, name))
		if !bytes.Equal(data, after) {
			t.Fatal("Target materialization changed canonical global registration owner")
		}
	}
}

func TestExternalOfferResolutionRejectsBorrowedRedirectedOrConflictingOwner(t *testing.T) {
	for _, change := range []string{"unrelated-root", "redirected-target", "descriptor-conflict"} {
		t.Run(change, func(t *testing.T) {
			root, target, m := globalExternalOIDCFixture(t)
			switch change {
			case "unrelated-root":
				target = filepath.Join(t.TempDir(), "targets", "test")
			case "redirected-target":
				if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
			case "descriptor-conflict":
				path := filepath.Join(root, externalProviderRegistryFile)
				data, _ := os.ReadFile(path)
				var doc map[string]any
				if err := json.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				doc["registrations"].([]any)[0].(map[string]any)["provider_id"] = "foreign/conflicting"
				data, _ = json.Marshal(doc)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ResolveExternalProviderRegistrationAt(target, m, capability.ProviderExternalOIDC); err == nil {
				t.Fatal("unverifiable selected registration was admitted")
			}
		})
	}
}
