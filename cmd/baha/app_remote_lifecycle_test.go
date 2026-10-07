package main

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteApplicationLifecycleNeverSilentlyDropsUnqualifiedIntent(t *testing.T) {
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	m := application.New("owned", "dev", true, false, false)
	if err := checkRemoteApplicationLifecycle(m); err != nil {
		t.Fatal(err)
	}
	for _, feature := range []string{"shared", "secrets", "identity", "object-storage", "logs", "messaging", "management-ui", "runtime-permissions"} {
		t.Run(feature, func(t *testing.T) {
			requested := m
			switch feature {
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
