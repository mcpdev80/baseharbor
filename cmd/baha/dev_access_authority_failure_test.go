package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// A remote developer CLI must never interpret a missing Core or unavailable
// runtime as permission to substitute its local workstation credentials.
func TestDeveloperReconcileFailsClosedOnRemoteRuntimeError(t *testing.T) {
	ctx, _, _ := remoteApplicationCoreFixtureEmpty(t)
	unavailable := errors.New("remote target runtime is unavailable")
	old := openBaoRuntime
	openBaoRuntime = func(context.Context) (bhruntime.RuntimeProvider, bhruntime.Files, error) {
		return nil, bhruntime.Files{}, unavailable
	}
	t.Cleanup(func() { openBaoRuntime = old })

	candidate := devaccess.Credentials{Username: "stale-local", Password: "stale-password"}
	actual, err := reconcileDeveloperCredentialAuthority(ctx, "remote", "dev", candidate, false)
	if !errors.Is(err, unavailable) {
		t.Fatalf("runtime error = %v, want wrapped resolution failure", err)
	}
	if actual != (devaccess.Credentials{}) {
		t.Fatal("remote runtime failure returned local credential material")
	}
	if !strings.Contains(err.Error(), "resolve managed credential authority") {
		t.Fatalf("runtime failure lacks authority context: %v", err)
	}
}

// Local development is still allowed without installing a Core. A genuinely
// absent installation is different from a missing remote management session.
func TestDeveloperReconcileAllowsExplicitlyCorelessLocalBootstrap(t *testing.T) {
	_, core, _ := remoteApplicationCoreFixtureEmpty(t)
	ctx := withTargetOverride(context.Background(), core.Name)
	allowed, err := localCredentialBootstrapAllowed(ctx)
	if err != nil || !allowed {
		t.Fatalf("Coreless local development rejected: allowed=%v err=%v", allowed, err)
	}

	missing := errors.New("OpenBao runtime not installed")
	old := openBaoRuntime
	openBaoRuntime = func(context.Context) (bhruntime.RuntimeProvider, bhruntime.Files, error) {
		return nil, bhruntime.Files{}, missing
	}
	t.Cleanup(func() { openBaoRuntime = old })
	candidate := devaccess.Credentials{Username: "developer", Password: "local-secret"}
	actual, err := reconcileDeveloperCredentialAuthority(ctx, core.Name, "dev", candidate, false)
	if err != nil || actual != candidate {
		t.Fatalf("local Coreless bootstrap rejected: err=%v", err)
	}
}

func TestDeveloperReconcileRefusesMissingRuntimeForRegisteredCore(t *testing.T) {
	target := configureTestTarget(t)
	ctx := withTargetOverride(context.Background(), target.Name)
	missing := errors.New("registered Core runtime is unavailable")
	old := openBaoRuntime
	openBaoRuntime = func(context.Context) (bhruntime.RuntimeProvider, bhruntime.Files, error) {
		return nil, bhruntime.Files{}, missing
	}
	t.Cleanup(func() { openBaoRuntime = old })
	candidate := devaccess.Credentials{Username: "developer", Password: "untrusted-local"}
	actual, err := reconcileDeveloperCredentialAuthority(ctx, target.Name, "dev", candidate, false)
	if !errors.Is(err, missing) {
		t.Fatalf("registered Core runtime error was not preserved: %v", err)
	}
	if actual != (devaccess.Credentials{}) {
		t.Fatal("registered Core with missing runtime accepted local credentials")
	}
}
