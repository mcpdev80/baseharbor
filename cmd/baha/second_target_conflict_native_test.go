package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
)

// TestSecondSameHostCoreNativeConflictInCI exercises two real configured
// Targets against ONE rootless Docker/Podman engine. The first Target owns a
// running Core; the second must fail before mutating that owner's resources.
// It is deliberately in the native acceptance lane, never a mocked PASS.
func TestSecondSameHostCoreNativeConflictInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_SECOND_TARGET_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("requires explicit native second-target Core conflict acceptance")
	}
	kind := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if kind != "docker" && kind != "podman" {
		t.Fatalf("native second-target acceptance requires docker or podman, got %q", kind)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	ensureRuntimeIntegrationTrustPlane(t, ctx)
	first := runtimeIntegrationTrustPlaneTargetName
	if first == "" {
		t.Fatal("package fixture did not record first Core owning Target")
	}
	firstProject := bhruntime.SharedProjectName(first)
	provider, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("resolve actual %s engine: %v", kind, err)
	}
	list := func() []bhruntime.RuntimeContainer {
		t.Helper()
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatalf("inspect actual %s engine: %v", kind, err)
		}
		return containers
	}
	running := func(containers []bhruntime.RuntimeContainer, project string) int {
		count := 0
		for _, item := range containers {
			if item.Running && item.Project == project {
				count++
			}
		}
		return count
	}
	if count := running(list(), firstProject); count == 0 {
		t.Fatalf("first configured Core %q has no live containers under %q", first, firstProject)
	}

	isolateRuntimeForTest(t, "second-target-conflict")
	second := strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET"))
	if second == "" || second == first {
		t.Fatalf("second selection must be distinct: first=%q second=%q", first, second)
	}
	secondProject := bhruntime.SharedProjectName(second)
	conflict, err := detectSharedCoreOwnerConflict(ctx)
	if err != nil {
		t.Fatalf("inspect second Target against live engine: %v", err)
	}
	if conflict == nil {
		t.Fatal("second same-engine Target did not detect first running Core")
	}
	if !strings.Contains(conflict.Error(), first) {
		t.Fatalf("conflict did not name actual first Core owner %q: %v", first, conflict)
	}

	var output bytes.Buffer
	err = runWithIO(ctx, []string{"up", "--control-plane-only", "--yes"}, &output, &output)
	var typed *bhruntime.SharedCoreOwnerConflict
	if !errors.As(err, &typed) {
		t.Fatalf("second same-engine Core start did not fail closed with typed conflict: err=%v output=%s", err, output.String())
	}
	after := list()
	if count := running(after, firstProject); count == 0 {
		t.Fatalf("second start disrupted running first Core %q", firstProject)
	}
	if count := running(after, secondProject); count != 0 {
		t.Fatalf("refused second Target unexpectedly started %d Core containers under %q", count, secondProject)
	}
	t.Logf("PASS: %s native first Core %s remains running; second Target %s refused without a second Core", kind, first, second)
}
