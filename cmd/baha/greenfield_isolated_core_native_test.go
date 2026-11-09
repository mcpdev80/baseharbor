package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
)

// TestGreenfieldIsolatedCoreNativeInCI proves a fresh package-owned Core can
// be selected and inspected using the real Docker/Podman engine. TestMain owns
// the isolated bootstrap Target and teardown; no foreign Core is adopted.
func TestGreenfieldIsolatedCoreNativeInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_GREENFIELD_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("requires explicit native Greenfield acceptance")
	}
	kind := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if kind != "docker" && kind != "podman" {
		t.Fatalf("native Greenfield acceptance requires docker or podman, got %q", kind)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	// The ambient implicit local Target must remain untouched by Greenfield.
	foreignRoot, err := deployment.TargetStateRoot("local")
	if err != nil {
		t.Fatalf("resolve foreign Target root: %v", err)
	}
	if _, err := os.Lstat(foreignRoot); !os.IsNotExist(err) {
		t.Fatalf("native Greenfield fixture is not isolated from implicit local Target: %v", err)
	}
	ensureRuntimeIntegrationTrustPlane(t, ctx)
	name := runtimeIntegrationTrustPlaneTargetName
	if name == "" || !strings.HasPrefix(name, "w169-ci-bootstrap-") {
		t.Fatalf("fresh isolated bootstrap Target missing: %q", name)
	}
	provider, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("resolve native %s runtime: %v", kind, err)
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatalf("inspect native %s runtime: %v", kind, err)
	}
	owned := 0
	for _, container := range containers {
		if container.Project == bhruntime.SharedProjectName(name) && container.Running {
			owned++
		}
	}
	if owned == 0 {
		t.Fatalf("fresh %s Core Target %q has no running native services", kind, name)
	}

	t.Setenv("BASEHARBOR_TARGET", "")
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"use", name}, &output, &output); err != nil {
		t.Fatalf("use verified fresh Core %q: %v output=%s", name, err, output.String())
	}
	output.Reset()
	if err := runWithIO(ctx, []string{"target", "activate", name}, &output, &output); err != nil {
		t.Fatalf("activate verified Core %q: %v output=%s", name, err, output.String())
	}
	selected, err := effectiveTarget(ctx)
	if err != nil || selected.Name != name || selected.RuntimeProvider != kind {
		t.Fatalf("activated Greenfield Target mismatch: selected=%+v err=%v", selected, err)
	}
	// Re-select the same independently owned Target through the explicit
	// environment path before the read-only live status verification.
	t.Setenv("BASEHARBOR_TARGET", name)
	output.Reset()
	if err := runWithIO(ctx, []string{"status"}, &output, &output); err != nil {
		t.Fatalf("fresh Core status failed on %s: %v output=%s", kind, err, output.String())
	}
	if strings.TrimSpace(output.String()) == "" {
		t.Fatal("native Core status returned empty evidence")
	}
	if _, err := os.Lstat(foreignRoot); !os.IsNotExist(err) {
		t.Fatalf("Greenfield lifecycle wrote into implicit local Target owner: %v", err)
	}
	t.Logf("PASS: native %s Greenfield bootstrap/use/activate/status on isolated Core %s with %d running services", kind, name, owned)
}
