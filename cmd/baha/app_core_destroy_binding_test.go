package main

import (
	"errors"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"testing"
)

func TestApplicationDestroyRejectsChangedBindingBeforeProviderInspection(t *testing.T) {
	for _, change := range []string{"Core", "Node"} {
		t.Run(change, func(t *testing.T) {
			ctx, core, node := remoteApplicationCoreFixture(t)
			ctx = withCoreAuthority(ctx, core)
			cfg, err := deployment.LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			name := core.Name
			if change == "Node" {
				name = node.Name
			}
			definition := cfg.Targets[name]
			if change == "Core" {
				definition.Runtime.Provider = "podman"
			} else {
				definition.Runtime.Provider = "docker"
			}
			cfg.Targets[name] = definition
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			// No provider or terminal is installed: rejection must precede native
			// inspections, cleanup confirmation and resource mutation.
			execution := &applicationDestroyExecution{resolved: resolvedApplication{Target: node}}
			err = execution.runPreflight(ctx)
			var failure *machine.Error
			if !errors.As(err, &failure) || failure.Code != machine.ErrorPolicyDenied {
				t.Fatal("destroy reached provider inspection after binding changed", err)
			}
		})
	}
}
