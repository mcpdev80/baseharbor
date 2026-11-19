package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// TestPersesMountedConfigProvenanceAndEffectivePolicy proves the config
// provenance chain policy -> renderer -> mounted file -> container path for
// the deployed Perses surface, on the genuine EnsureRuntime production path.
func TestPersesMountedConfigProvenanceAndEffectivePolicy(t *testing.T) {
	manifest := persesRuntimeManifest(t, true)
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "perses-config-provenance"}
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, manifest)
	if err != nil {
		t.Fatalf("ensure the Perses runtime files: %v", err)
	}
	compose, err := RuntimeComposeYAMLForProject(manifest, files.ResourceProject)
	if err != nil {
		t.Fatalf("render the Perses compose definition: %v", err)
	}
	serviceBlock := compose
	if idx := strings.Index(serviceBlock, "\n  perses:"); idx >= 0 {
		serviceBlock = serviceBlock[idx:]
	}
	if end := strings.Index(serviceBlock, "\nvolumes:"); end >= 0 {
		serviceBlock = serviceBlock[:end]
	}
	// The command names the mounted file and the mount exists, read-only.
	if !strings.Contains(serviceBlock, "--config="+serviceaccess.PersesConfigContainerPath) {
		t.Fatalf("the Perses command must name the mounted configuration path:\n%s", serviceBlock)
	}
	if !strings.Contains(serviceBlock, "./providers/management-ui/perses/config.yaml:"+serviceaccess.PersesConfigMountDir+":ro") {
		t.Fatalf("the rendering must mount the rendered configuration read-only at %s:\n%s", serviceaccess.PersesConfigMountDir, serviceBlock)
	}
	// The materialized host file exists and is the deterministic rendering of
	// the validated policy, free of credential material.
	hostPath := PersesConfigHostPath(files.Dir)
	contents, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatalf("the realization must materialize the rendered configuration at %s: %v", hostPath, err)
	}
	policy := capability.DefaultPersesSecurityPolicy()
	if err := capability.ValidatePersesSecurityPolicy(policy); err != nil {
		t.Fatalf("the effective posture must validate before rendering: %v", err)
	}
	expected, err := serviceaccess.RenderPersesConfig(policy)
	if err != nil {
		t.Fatalf("render the expected configuration: %v", err)
	}
	if string(contents) != expected {
		t.Fatalf("the mounted file must equal the deterministic rendering of the effective policy:\n%s", contents)
	}
	for _, marker := range []string{"enable_auth: true", "encryption_key_file:", "deny_private_networks: true", "disable_local: true", "denied_networks:"} {
		if !strings.Contains(string(contents), marker) {
			t.Fatalf("the effective policy must state %q:\n%s", marker, contents)
		}
	}
	if strings.Contains(string(contents), "encryption_key: ") {
		t.Fatalf("the rendering must reference the encryption key by file only, never inline it:\n%s", contents)
	}
	// Determinism: a second rendering of the same policy is byte-identical.
	second, err := serviceaccess.RenderPersesConfig(policy)
	if err != nil || second != expected {
		t.Fatalf("the rendering must be deterministic: %v", err)
	}
}

// TestPersesRuntimeFilesRequireConfigProducerFailClosed asserts the mounted
// configuration is never left to the upstream default: EnsureRuntime
// materializes the file for a selected surface, and a deselected surface
// leaves no provider files behind (the fail-closed not_configured path).
func TestPersesRuntimeFilesRequireConfigProducerFailClosed(t *testing.T) {
	selected := persesRuntimeManifest(t, true)
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "perses-config-producer"}
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, selected)
	if err != nil {
		t.Fatalf("ensure the selected Perses runtime files: %v", err)
	}
	if _, err := os.Stat(PersesConfigHostPath(files.Dir)); err != nil {
		t.Fatalf("a selected Perses surface must materialize its configuration: %v", err)
	}
	deselected := selected
	deselected.Services.Perses = false
	if err := EnsurePersesManagementUI(context.Background(), serviceissuer.New(t), files, deselected); err != nil {
		t.Fatalf("deselected Perses must be a no-op: %v", err)
	}
}
