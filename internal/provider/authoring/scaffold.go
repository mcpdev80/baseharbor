package authoring

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type InitResult struct {
	Root  string   `json:"root"`
	Files []string `json:"files"`
}

func Init(root, id string) (InitResult, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return InitResult{}, err
	}
	entries, err := os.ReadDir(abs)
	if err == nil && len(entries) != 0 {
		return InitResult{}, fmt.Errorf("provider directory %s is not empty", abs)
	}
	if err != nil && !os.IsNotExist(err) {
		return InitResult{}, err
	}
	descriptor := Descriptor{
		ID:               strings.TrimSpace(id),
		Version:          "0.1.0",
		ProviderProtocol: "baseharbor.provider/v1",
		ServiceKinds:     []string{"sql"},
		ServiceContracts: []string{"database.sql/v1"},
		Capabilities:     []string{"database.sql/v1"},
		SupportedScopes:  []string{"application"},
	}
	if err := descriptor.Validate(); err != nil {
		return InitResult{}, err
	}
	data, err := yaml.Marshal(descriptor)
	if err != nil {
		return InitResult{}, err
	}
	files := map[string][]byte{
		"provider.yaml":      data,
		"config.schema.json": []byte("{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n  \"type\": \"object\",\n  \"additionalProperties\": false\n}\n"),
		"go.mod":              []byte(renderStarterGoMod(descriptor)),
		"provider.go":         []byte(renderStarterProvider(descriptor)),
		"README.md":           []byte(renderStarterREADME(descriptor)),
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return InitResult{}, err
	}
	var written []string
	for _, name := range []string{"provider.yaml", "config.schema.json", "go.mod", "provider.go", "README.md"} {
		if err := os.WriteFile(filepath.Join(abs, name), files[name], 0o644); err != nil {
			_ = os.RemoveAll(abs)
			return InitResult{}, err
		}
		written = append(written, name)
	}
	return InitResult{Root: abs, Files: written}, nil
}

func Load(root string) (Descriptor, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	data, err := os.ReadFile(filepath.Join(root, "provider.yaml"))
	if err != nil {
		return Descriptor{}, fmt.Errorf("read provider.yaml: %w", err)
	}
	var descriptor Descriptor
	if err := yaml.Unmarshal(data, &descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("decode provider.yaml: %w", err)
	}
	return descriptor, descriptor.Validate()
}


func renderStarterGoMod(descriptor Descriptor) string {
	name := strings.ReplaceAll(strings.TrimSpace(descriptor.ID), "/", "-")
	return fmt.Sprintf("module example.com/%s\n\ngo 1.25.0\n\nrequire github.com/mcpdev80/baseharbor v0.4.18\n", name)
}

func renderStarterProvider(descriptor Descriptor) string {
	kind := "provider.Kind(" + fmt.Sprintf("%q", descriptor.ID) + ")"
	capabilityKind := "provider.SQL"
	if len(descriptor.ServiceContracts) > 0 {
		switch strings.SplitN(descriptor.ServiceContracts[0], "/", 2)[0] {
		case "cache.key-value":
			capabilityKind = "provider.KeyValue"
		case "object-storage.s3":
			capabilityKind = "provider.ObjectStorageS3"
		case "secrets":
			capabilityKind = "provider.Secrets"
		case "telemetry.otlp":
			capabilityKind = "provider.TelemetryOTLP"
		case "metrics":
			capabilityKind = "provider.Metrics"
		case "logs":
			capabilityKind = "provider.Logs"
		case "traces":
			capabilityKind = "provider.Traces"
		case "identity.oidc":
			capabilityKind = "provider.Identity"
		}
	}
	return fmt.Sprintf(`package providerimpl

import (
	"context"
	"errors"

	"github.com/mcpdev80/baseharbor/sdk/provider"
)

// Driver is the provider implementation boundary. Keep product-specific APIs,
// SDKs and credentials behind this type.
type Driver struct{}

var _ provider.Driver = (*Driver)(nil)

func (*Driver) Descriptor() provider.Provider {
	return provider.Provider{
		Kind: %s,
		Capabilities: []provider.Kind{%s},
	}
}

func (*Driver) Preflight(context.Context, provider.Resource, provider.Binding) error {
	// Validate configuration, reachability and required semantics here.
	// Preflight MUST NOT mutate provider state.
	return nil
}

func (*Driver) Provision(context.Context, provider.Resource, provider.Binding) error {
	return errors.New("TODO: implement convergent provider provisioning")
}

func (*Driver) Bind(context.Context, provider.Resource, provider.Binding) error {
	return errors.New("TODO: implement application-facing binding")
}

func (*Driver) Verify(context.Context, provider.Resource, provider.Binding) error {
	return errors.New("TODO: verify application-facing capability semantics")
}
`, kind, capabilityKind)
}

func renderStarterREADME(descriptor Descriptor) string {
	return fmt.Sprintf(`# BaseHarbor Capability Provider

Provider: %s

This scaffold implements the public BaseHarbor provider boundary from:

~~~text
github.com/mcpdev80/baseharbor/sdk/provider
~~~

Implement the lifecycle in provider.go:

~~~text
Preflight -> Provision -> Bind -> Verify
~~~

Rules:

- Preflight is side-effect free.
- Provision converges and is idempotent.
- Bind exposes only application-facing contract data.
- Verify tests capability semantics, not only process health.
- Keep credentials out of diagnostics and portable intent.
- Enforce ownership before destructive operations.
- Use reconciliation hooks for drift/retry when the provider manages state.

Validate the descriptor with:

~~~bash
baha provider test .
~~~

Then exercise the SDK conformance harness from provider tests before publishing.
`, descriptor.ID)
}
