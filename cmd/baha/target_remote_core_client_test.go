package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteCoreTargetCreateBindsTenantAndNeverFallsBackToLocalDocker(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", "")
	const tenant = "11111111-1111-4111-8111-111111111111"
	args := []string{
		"vm-core", "--runtime-provider", "docker",
		"--access", "vm-core-management", "--access-provider", "baseharbor-core",
		"--reference", "vm-core", "--tenant-id", tenant,
	}
	var out bytes.Buffer
	if err := createTarget(context.Background(), args, &out, &out); err != nil {
		t.Fatalf("remote Core Target could not be configured: %v", err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	target, err := cfg.ResolveTarget("vm-core", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "vm-core" || target.RuntimeProvider != "docker" || target.TenantID != tenant ||
		target.AccessProvider != "baseharbor-core" {
		t.Fatalf("remote Core Target lost its scoped management authority: %+v", target)
	}
	if _, err := detectRuntimeForTarget(context.Background(), target); err == nil {
		t.Fatal("remote Core Target incorrectly entered local workstation Docker runtime")
	}
	err = bindExplicitSelectionToConnection(context.Background(), target.Name)
	var typed *machine.Error
	if !errors.As(err, &typed) || typed.Code != machine.ErrorAuthenticationFailed ||
		!strings.Contains(err.Error(), "BASEHARBOR_CONNECTION_PROFILE") {
		t.Fatalf("remote Core Target activated without pinned OIDC/CA/Core installation profile: %v", err)
	}
	if _, err := os.Stat(os.Getenv("BASEHARBOR_STATE_DIR") + "/targets/vm-core/runtime/installation.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remote Core Target fabricated a local Core installation: %v", err)
	}
}
