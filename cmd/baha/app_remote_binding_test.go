package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type applicationBindingTransport struct {
	capabilities int
	dispatches   int
}

func (t *applicationBindingTransport) LiveCapabilities(targetenrollment.Scope) (targetsession.Capabilities, error) {
	t.capabilities++
	return targetsession.Capabilities{}, nil
}

func (t *applicationBindingTransport) Dispatch(context.Context, targetenrollment.Scope, targetsession.Request) (targetsession.Response, error) {
	t.dispatches++
	return targetsession.Response{}, nil
}

func TestRetainedApplicationTransportRevalidatesAuthorityBeforeEveryCall(t *testing.T) {
	for _, change := range []string{"core", "node", "tenant", "runtime", "access-provider"} {
		t.Run(change, func(t *testing.T) {
			ctx, core, node := remoteApplicationCoreFixture(t)
			base := &applicationBindingTransport{}
			transport, scope, err := remoteApplicationTransport(withCoreAuthority(ctx, core), resolvedApplication{Target: node}, base)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transport.LiveCapabilities(scope); err != nil {
				t.Fatal(err)
			}
			if _, err := transport.Dispatch(ctx, scope, targetsession.Request{}); err != nil {
				t.Fatal(err)
			}
			cfg, err := deployment.LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "core":
				definition := cfg.Targets[core.Name]
				definition.Runtime.Provider = "podman"
				if core.RuntimeProvider == "podman" {
					definition.Runtime.Provider = "docker"
				}
				cfg.Targets[core.Name] = definition
			case "node":
				access := cfg.Access["connector"]
				access.Reference = "foreign-node"
				cfg.Access["connector"] = access
			case "tenant":
				definition := cfg.Targets[node.Name]
				definition.TenantID = "22222222-2222-4222-8222-222222222222"
				cfg.Targets[node.Name] = definition
			case "runtime":
				definition := cfg.Targets[node.Name]
				definition.Runtime.Provider = "docker"
				cfg.Targets[node.Name] = definition
			case "access-provider":
				definition := cfg.Targets[node.Name]
				cfg.Access["replacement"] = deployment.AccessDefinition{Provider: "local", Reference: "local"}
				definition.Access = deployment.TargetAccess{Reference: "replacement"}
				cfg.Targets[node.Name] = definition
			}
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			for _, run := range []func() error{
				func() error { _, err := transport.LiveCapabilities(scope); return err },
				func() error { _, err := transport.Dispatch(ctx, scope, targetsession.Request{}); return err },
			} {
				var failure *machine.Error
				if err := run(); !errors.As(err, &failure) || failure.Code != machine.ErrorPolicyDenied {
					t.Fatal("retained remote handle admitted changed authority", err)
				}
			}
			if base.capabilities != 1 || base.dispatches != 1 {
				t.Fatal("changed authority reached native transport", base)
			}
		})
	}
}

func TestApplicationTransportRejectsScopeSubstitutionAndCanceledDispatch(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	base := &applicationBindingTransport{}
	transport, scope, err := remoteApplicationTransport(withCoreAuthority(ctx, core), resolvedApplication{Target: node}, base)
	if err != nil {
		t.Fatal(err)
	}
	foreign := scope
	foreign.NodeID = "foreign-node"
	if _, err := transport.LiveCapabilities(foreign); !errors.Is(err, targetsession.ErrUnavailable) {
		t.Fatal("scope substitution admitted", err)
	}
	if _, err := transport.Dispatch(ctx, foreign, targetsession.Request{}); !errors.Is(err, targetsession.ErrUnavailable) {
		t.Fatal("substituted mutation admitted", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := transport.Dispatch(canceled, scope, targetsession.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled mutation reached transport", err)
	}
	if base.capabilities != 0 || base.dispatches != 0 {
		t.Fatal("denied call reached transport", base)
	}
}
