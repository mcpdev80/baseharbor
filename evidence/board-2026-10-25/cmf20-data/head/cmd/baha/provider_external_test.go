package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	providerauthoring "github.com/mcpdev80/baseharbor/internal/provider/authoring"
)

func TestGuidedExternalProviderOnboardingUsesSingleReader(t *testing.T) {
	input := strings.NewReader(strings.Join([]string{
		"company-db",
		"database.sql",
		"company/postgresql",
		"company-postgresql",
		"postgres://db.example:5432/app",
		"file:///tmp/company-db.json",
		"auto",
		"",
		"y",
		"",
	}, "\n"))
	reader := bufio.NewReader(input)
	var out bytes.Buffer

	opts, err := completeProviderExternalArgsInteractive(providerExternalArgs{}, &out, reader)
	if err != nil {
		t.Fatal(err)
	}
	if opts.ID != "company-db" || opts.TrustMode != "auto" || len(opts.Capabilities) != 1 || opts.Capabilities[0] != "database.sql" {
		t.Fatalf("opts=%#v", opts)
	}
	ok, err := confirmExternalProviderRegistration(&out, reader)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected confirmation from same buffered reader")
	}
}

func TestProviderExternalRegistrationDefaultsToAutoTrust(t *testing.T) {
	reg, err := providerExternalRegistration(providerExternalArgs{
		ID:           "company-idp",
		ProviderID:   "company/oidc",
		Kind:         "company-oidc",
		Capabilities: []string{"identity.oidc"},
		Endpoint:     "https://id.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Trust.Mode != externalprovider.TrustAuto {
		t.Fatalf("trust mode=%q", reg.Trust.Mode)
	}
}

func TestProviderExternalRegistrationUsesDescriptor(t *testing.T) {
	root := t.TempDir() + "/provider"
	if _, err := providerauthoring.Init(root, "example/postgresql"); err != nil {
		t.Fatal(err)
	}
	descriptor, err := providerauthoring.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	descriptor.SupportedScopes = append(descriptor.SupportedScopes, string(capability.ScopeExternal))
	data, err := yaml.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "provider.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := providerExternalRegistration(providerExternalArgs{
		ID:             "company-db",
		DescriptorPath: root,
		Endpoint:       "postgres://db.example:5432/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.ProviderID != "example/postgresql" || reg.ProviderVersion != "0.1.0" || reg.ProviderProtocol != capability.ProviderProtocolV1 {
		t.Fatalf("registration metadata=%#v", reg)
	}
	if reg.Provider.Kind != capability.ProviderKind("example/postgresql") || !reg.Provider.Supports(capability.SQL) {
		t.Fatalf("provider=%#v", reg.Provider)
	}
}
