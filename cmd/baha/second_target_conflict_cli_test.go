package main

import (
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// TestSecondSameHostUpNamesOwningTargetAndRefuses verifies the operator-facing
// contract of the 0427-163 fail-closed second-target conflict gate: the
// SharedCoreOwnerConflict message names the owning target and offers concrete
// options instead of silently joining the first target's Core.
func TestSecondSameHostUpNamesOwningTargetAndRefuses(t *testing.T) {
	conflict := &bhruntime.SharedCoreOwnerConflict{
		OwningTargets:   []string{"boot"},
		RunningProjects: []string{"bh-boot-shared"},
	}
	message := conflict.Error()

	if !strings.Contains(message, "boot") {
		t.Fatalf("conflict must name the owning target, got: %s", message)
	}
	if !strings.Contains(message, "baha use") || !strings.Contains(message, "baha up --target") {
		t.Fatalf("conflict must offer concrete options (use/up --target), got: %s", message)
	}
	if !strings.Contains(message, "cannot silently share") {
		t.Fatalf("conflict must fail closed (refuse silent share), got: %s", message)
	}
}
