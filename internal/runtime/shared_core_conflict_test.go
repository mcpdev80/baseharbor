package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSharedCoreOwnerConflictNamesTheOwningTarget(t *testing.T) {
	provider := &conflictProbeFake{containers: []probeContainer{
		{Project: "bh-boot-shared", Running: true},
	}}
	conflict, err := SharedCoreOwnerConflictForEngine(context.Background(), provider, "bh-podman2test-shared")
	if err != nil {
		t.Fatal(err)
	}
	if conflict == nil {
		t.Fatal("expected a shared-core owner conflict for a foreign running Core")
	}
	if got := conflict.Error(); !strings.Contains(got, "boot") {
		t.Fatalf("conflict must name the owning target, got: %s", got)
	}
}

func TestSharedCoreOwnerConflictIgnoresTheOwningTarget(t *testing.T) {
	provider := &conflictProbeFake{containers: []probeContainer{
		{Project: "bh-podman2test-shared", Running: true},
	}}
	conflict, err := SharedCoreOwnerConflictForEngine(context.Background(), provider, "bh-podman2test-shared")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != nil {
		t.Fatalf("owning Core must not self-conflict, got: %+v", conflict)
	}
}

func TestSharedCoreOwnerConflictIgnoresApplicationProjects(t *testing.T) {
	provider := &conflictProbeFake{containers: []probeContainer{
		{Project: "bh-boot-hp1app-dev", Running: true},
	}}
	conflict, err := SharedCoreOwnerConflictForEngine(context.Background(), provider, "bh-podman2test-shared")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != nil {
		t.Fatalf("application projects must not trigger the Core conflict, got: %+v", conflict)
	}
}

func TestSharedCoreOwnerConflictIgnoresStoppedCore(t *testing.T) {
	provider := &conflictProbeFake{containers: []probeContainer{
		{Project: "bh-first-shared", Running: false},
	}}
	conflict, err := SharedCoreOwnerConflictForEngine(context.Background(), provider, "bh-second-shared")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != nil {
		t.Fatalf("stopped Core must not prevent a second Target: %+v", conflict)
	}
}

func TestSharedCoreOwnerConflictNoCoreReportsNil(t *testing.T) {
	provider := &conflictProbeFake{containers: []probeContainer{
		{Project: "some-foreign-stack", Running: true},
	}}
	conflict, err := SharedCoreOwnerConflictForEngine(context.Background(), provider, "bh-podman2test-shared")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != nil {
		t.Fatalf("a non-Core stack must not trigger the conflict, got: %+v", conflict)
	}
}

func TestSharedCoreOwnerConflictErrorWrapsToConflict(t *testing.T) {
	provider := &conflictProbeFake{containers: []probeContainer{{Project: "bh-boot-shared", Running: true}}}
	conflict, err := SharedCoreOwnerConflictForEngine(context.Background(), provider, "bh-podman2test-shared")
	if err != nil {
		t.Fatal(err)
	}
	var concrete *SharedCoreOwnerConflict
	if !errors.As(conflict, &concrete) {
		t.Fatal("conflict must be a *SharedCoreOwnerConflict")
	}
	if base := conflict.Unwrap(); base == nil {
		t.Fatal("conflict must unwrap to a base error for machine.Classify")
	}
}

func TestIsSharedCoreProjectClassification(t *testing.T) {
	for project, want := range map[string]bool{
		"bh-boot-shared":       true,
		"bh-my.app-shared":     true,
		"bh-boot-hp1app-dev":   false,
		"baseharbor-boot":      false,
		"some-foreign-stack":   false,
		"":                     false,
		"bh-shared-extra":      false,
		"bh-boot-shared-extra": false,
	} {
		if got := isSharedCoreProject(project); got != want {
			t.Errorf("isSharedCoreProject(%q)=%t want %t", project, got, want)
		}
	}
}

type probeContainer struct {
	Project string
	Running bool
}

// conflictProbeFake is a minimal contract.RuntimeProvider stub exposing only
// ListRuntimeContainers, enough to exercise the conflict gate without an engine.
type conflictProbeFake struct {
	containers []probeContainer
}

func (f *conflictProbeFake) ListRuntimeContainers(context.Context) ([]RuntimeContainer, error) {
	out := make([]RuntimeContainer, 0, len(f.containers))
	for _, container := range f.containers {
		out = append(out, RuntimeContainer{Project: container.Project, Running: container.Running})
	}
	return out, nil
}
