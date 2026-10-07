package main

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteApplicationLifecycleNeverSilentlyDropsUnqualifiedIntent(t *testing.T) {
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	m := application.New("owned", "dev", true, false, false)
	if err := checkRemoteApplicationLifecycle(m); err != nil {
		t.Fatal(err)
	}
	for _, feature := range []string{"ha", "shared", "secrets", "identity", "object-storage", "logs", "messaging", "management-ui", "runtime-permissions"} {
		t.Run(feature, func(t *testing.T) {
			requested := m
			switch feature {
			case "ha":
				requested.HA = true
			case "shared":
				t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
			case "secrets":
				requested.Services.Secrets = true
			case "identity":
				requested.Services.Identity = true
			case "object-storage":
				requested.Services.ObjectStorage = true
			case "logs":
				requested.Logs.Collect = []string{"application"}
			case "messaging":
				requested.Services.MessagingQueue = true
			case "management-ui":
				requested.Services.SQLManagementUI = true
			case "runtime-permissions":
				requested.Runtime.Permissions = []application.RuntimePermission{{Capability: "sql", Services: []string{"api"}}}
			}
			var failure *machine.Error
			if err := checkRemoteApplicationLifecycle(requested); !errors.As(err, &failure) || failure.Code != machine.ErrorCapabilityMissing {
				t.Fatal("unqualified remote intent accepted or downgraded", err)
			}
		})
	}
}

func TestRemoteApplicationConstructorsDoNotRequireLocalRuntimeSource(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	ctx = withCoreAuthority(ctx, core)
	store := application.Store{Root: t.TempDir()}
	manifest := application.New("retained", "dev", false, false, false)
	if _, err := store.Create(manifest); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := store.Load(manifest.Name)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := deployment.NewDeploymentIdentity(node.Name, manifest.ApplicationID, manifest.Name, manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{Identity: identity, Applied: deployment.AppliedDeployment{Intent: intent, RuntimeProvider: node.RuntimeProvider}}); err != nil {
		t.Fatal(err)
	}
	up, err := newApplicationUpExecution(ctx, store, []string{manifest.Name}, io.Discard, io.Discard)
	if err != nil || !isRemoteApplication(up.resolved) {
		t.Fatal("remote up depended on missing local generated source", err)
	}
	destroy, err := newApplicationDestroyExecution(ctx, store, []string{manifest.Name, "--yes"}, io.Discard, io.Discard)
	if err != nil || destroy.term == nil || !destroy.confirmed {
		t.Fatal("remote teardown lacked retained-path rendering or depended on local generated source", err)
	}
}
