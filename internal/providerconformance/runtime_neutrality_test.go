package providerconformance

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestAllShippedReferenceProvidersDeclareExplicitPlacement(t *testing.T) {
	integrations := capability.ReferenceIntegrations()
	if len(integrations) == 0 {
		t.Fatal("reference provider catalog is empty")
	}
	seen := map[capability.ProviderKind]struct{}{}
	for _, descriptor := range integrations {
		provider := descriptor.Provider.Kind
		if _, duplicate := seen[provider]; duplicate {
			t.Fatalf("provider %s appears more than once in reference catalog", provider)
		}
		seen[provider] = struct{}{}
		if err := descriptor.Validate(); err != nil {
			t.Fatalf("%s reference integration: %v", provider, err)
		}
		if len(descriptor.Provider.Capabilities) == 0 {
			t.Fatalf("%s declares no portable capability", provider)
		}
		if len(descriptor.SupportedScopes) == 0 {
			t.Fatalf("%s declares no supported provider placement", provider)
		}
		for _, scope := range descriptor.SupportedScopes {
			switch scope {
			case capability.ScopeApplication, capability.ScopeShared, capability.ScopeExternal:
			default:
				t.Fatalf("%s declares unsupported placement %q", provider, scope)
			}
		}
	}
}

func TestFrozenRuntimeControlContractsContainNoRuntimeNativeTopology(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve conformance test location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	files := []string{
		"spec/runtime-api/v1/openapi.yaml",
		"internal/runtimeexecutor/client.go",
		"internal/runtimeexecutor/handler.go",
	}
	forbidden := []string{
		"compose",
		"docker",
		"podman",
		"kubernetes",
		"statefulset",
		"deployment/",
		"container_name",
		"hostpath",
		"host_path",
	}
	for _, relative := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		text := strings.ToLower(string(data))
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s leaks runtime-native topology token %q into frozen runtime-control contract", relative, token)
			}
		}
	}
}

func TestManagementUISemanticsRemainOperatorFacing(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve conformance test location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	data, err := os.ReadFile(filepath.Join(root, "internal", "providerui", "application.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"Class: \"administration\"",
		"Scope: \"application\"",
		"WorkloadServiceBindingProjectionDir",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("provider UI contract lost operator/binding boundary marker %q", required)
		}
	}
}
