package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuntimeIdentityMissingPermissionFailsClosed asserts that a workload
// service consuming the runtime broker but NOT named in any runtime permission
// fails closed. BaseHarbor must never fall back to the shared legacy app token;
// the override is rejected with ErrRuntimeIdentityRequiresExplicitPermission.
func TestRuntimeIdentityMissingPermissionFailsClosed(t *testing.T) {
	// m requires the broker (secrets) but declares no runtime permission.
	m := New("demo", "dev", true, false, true)
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	materializeRuntimeMTLSTestFiles(t, files)
	workload := WorkloadFiles{Services: []string{"api"}}
	// api reaches the broker at runtime but has no explicit runtime permission.
	plan := WorkloadBindingPlan{RuntimeIdentityServices: []string{"api"}}

	if _, _, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, plan); !errors.Is(err, ErrRuntimeIdentityRequiresExplicitPermission) {
		t.Fatalf("missing permission must fail closed with ErrRuntimeIdentityRequiresExplicitPermission, got: %v", err)
	}
	// The shared legacy token must never be materialized for an unauthorized service.
	if _, err := os.Stat(RuntimeIdentityTokenPath(files)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shared legacy runtime token must not be materialized for an unauthorized service: %v", err)
	}
}

// TestRuntimeIdentityExplicitPermissionRejectsSiblingAccess proves that a valid
// explicit identity stays scoped to its authorized service and cannot be
// reached by a sibling service that lacks its own permission.
func TestRuntimeIdentityExplicitPermissionRejectsSiblingAccess(t *testing.T) {
	m := New("demo", "dev", false, false, true)
	m = WithRuntimePermission(m, "object-storage.s3/v1", []string{"api"}, "runtime.create")
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	materializeRuntimeMTLSTestFiles(t, files)

	// api is explicitly authorized; a sibling service "worker" is not.
	workload := WorkloadFiles{Services: []string{"api", "worker"}}
	plan := WorkloadBindingPlan{RuntimeIdentityServices: []string{"api"}}

	path, enabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, plan)
	if err != nil {
		t.Fatalf("explicit permission must succeed: %v", err)
	}
	if !enabled {
		t.Fatal("override was not enabled")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	// The authorized service uses its own service-scoped token.
	serviceSecret := runtimeServiceIdentitySecretName("api")
	if !strings.Contains(text, "source: "+serviceSecret) {
		t.Fatalf("explicit permission did not use service-scoped token %q:\n%s", serviceSecret, text)
	}
	if strings.Contains(text, "source: baseharbor-runtime-token\n") {
		t.Fatalf("explicit permission unexpectedly fell back to legacy app token:\n%s", text)
	}
	// The sibling service must not receive any runtime identity binding.
	if strings.Contains(text, "worker:") {
		t.Fatalf("sibling service without permission received a runtime identity binding:\n%s", text)
	}
}

// TestRuntimeIdentityLegacyFallbackRemoved ensures no code path still resolves an
// unauthorized service to the shared legacy app token.
func TestRuntimeIdentityLegacyFallbackRemoved(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manifest Manifest
		plan     WorkloadBindingPlan
	}{
		{
			name:     "broker via secrets only",
			manifest: New("demo", "dev", true, false, true),
			plan:     WorkloadBindingPlan{RuntimeIdentityServices: []string{"api"}},
		},
		{
			name:     "broker via permission plus extra consumer",
			manifest: WithRuntimePermission(New("demo", "dev", false, false, true), "object-storage.s3/v1", []string{"api"}, "runtime.create"),
			plan:     WorkloadBindingPlan{RuntimeIdentityServices: []string{"api", "worker"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
			files.Bindings = filepath.Join(files.Dir, "bindings")
			materializeRuntimeMTLSTestFiles(t, files)
			workload := WorkloadFiles{Services: []string{"api"}}
			_, _, err := MaterializeRuntimeIdentityWorkloadOverride(tt.manifest, workload, files, tt.plan)
			if !errors.Is(err, ErrRuntimeIdentityRequiresExplicitPermission) {
				t.Fatalf("expected fail-closed ErrRuntimeIdentityRequiresExplicitPermission, got: %v", err)
			}
			if _, err := os.Stat(RuntimeIdentityTokenPath(files)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("shared legacy token must not be materialized: %v", err)
			}
		})
	}
}
