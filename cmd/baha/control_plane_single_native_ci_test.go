package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
)

// TestControlPlaneSingleDefaultNativeRuntimeInCI checks the ACTUAL running
// control plane, not just the generated Compose template. It must run on both
// existing Docker and rootless Podman acceptance runners.
func TestControlPlaneSingleDefaultNativeRuntimeInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_SINGLE_DEFAULT_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("native control-plane topology belongs to Docker/Podman acceptance")
	}
	kind := strings.ToLower(strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME")))
	if kind != "docker" && kind != "podman" {
		t.Fatalf("native acceptance requires an explicit docker or podman runtime, got %q", kind)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// The existing fixture starts a genuine control plane using the shipped
	// default (no --ha), and fails if startup/bootstrap is unsuccessful.
	ensureRuntimeIntegrationTrustPlane(t, ctx)
	provider, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("resolve required %s runtime: %v", kind, err)
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatalf("resolve selected control-plane files: %v", err)
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatalf("list actual %s containers: %v", kind, err)
	}
	count := map[string]int{}
	for _, container := range containers {
		if container.Project != files.Project && container.Project != files.ResourceProject {
			continue
		}
		if !container.Running {
			continue
		}
		service := strings.TrimSpace(container.Service)
		name := strings.TrimSpace(container.Name)
		for _, candidate := range []string{
			"postgres-member-1", "postgres-member-2", "postgres-member-3",
			"openbao-member-1", "openbao-member-2", "openbao-member-3",
			"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3",
		} {
			// Compose and Quadlet expose project-specific container names.
			// Match whole service boundaries rather than substring "member-1".
			if service == candidate {
				count[candidate]++
				continue
			}
			if service != "" {
				continue
			}
			if name == candidate || strings.Contains(name, "-"+candidate+"-") ||
				strings.HasSuffix(name, "-"+candidate) ||
				strings.HasPrefix(name, candidate+"-") {
				count[candidate]++
			}
		}
	}
	for _, required := range []string{"postgres-member-1", "openbao-member-1"} {
		if count[required] != 1 {
			t.Fatalf("%s default requires exactly one running %s, found %d; containers=%+v", kind, required, count[required], containers)
		}
	}
	for _, forbidden := range []string{
		"postgres-member-2", "postgres-member-3",
		"openbao-member-2", "openbao-member-3",
		"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3",
	} {
		if count[forbidden] != 0 {
			t.Fatalf("%s default unexpectedly runs HA service %s (%d); containers=%+v", kind, forbidden, count[forbidden], containers)
		}
	}
}
