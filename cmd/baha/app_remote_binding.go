package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Application authority is checked at each transport boundary, not just when
// its runtime handle is constructed. A retained handle cannot outlive a changed
// installation or Target configuration. Operation authorization precedes this
// internal binding; the Connector does not become an application authority.
type applicationRemoteTransport struct {
	ctx      context.Context
	resolved resolvedApplication
	base     targetsession.ProjectTransport
	scope    targetenrollment.Scope
	access   map[string]deployment.AccessDefinition
}

func remoteApplicationTransport(ctx context.Context, resolved resolvedApplication, base targetsession.ProjectTransport) (targetsession.ProjectTransport, targetenrollment.Scope, error) {
	target := resolved.Target
	scope := targetenrollment.Scope{TenantID: target.TenantID, TargetID: target.Name, NodeID: target.AccessReference, Runtime: target.RuntimeProvider}
	bound := &applicationRemoteTransport{ctx: withTargetOverride(ctx, target.Name), resolved: resolved, base: base, scope: scope}
	if err := bound.validate(scope); err != nil {
		return nil, targetenrollment.Scope{}, err
	}
	access, err := bound.accessBindings()
	if err != nil {
		return nil, targetenrollment.Scope{}, err
	}
	bound.access = access
	return bound, scope, nil
}

func (t *applicationRemoteTransport) accessBindings() (map[string]deployment.AccessDefinition, error) {
	core, _, err := applicationCoreTarget(t.ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return nil, err
	}
	result := make(map[string]deployment.AccessDefinition, 2)
	for _, reference := range []string{core.AccessReference, t.resolved.Target.AccessReference} {
		access, ok := cfg.Access[reference]
		if !ok {
			return nil, targetsession.ErrUnavailable
		}
		result[reference] = access
	}
	return result, nil
}

func (t *applicationRemoteTransport) validate(scope targetenrollment.Scope) error {
	if t == nil || t.base == nil || scope != t.scope || scope.Validate() != nil {
		return targetsession.ErrUnavailable
	}
	if err := t.ctx.Err(); err != nil {
		return err
	}
	if _, remote, err := applicationCoreTarget(t.ctx); err != nil {
		return err
	} else if !remote {
		return targetsession.ErrUnavailable
	}
	current, err := effectiveTarget(t.ctx)
	if err != nil {
		return err
	}
	if current != t.resolved.Target {
		return machine.NewError(machine.ErrorPolicyDenied, "Application Target binding changed during execution.", "Resolve the selected Target again before retrying.", false)
	}
	if t.access != nil {
		current, err := t.accessBindings()
		if err != nil {
			return err
		}
		for reference, expected := range t.access {
			if current[reference] != expected {
				return machine.NewError(machine.ErrorPolicyDenied, "Application Access binding changed during execution.", "Resolve the selected installation and Target again before retrying.", false)
			}
		}
	}
	return nil
}

func (t *applicationRemoteTransport) LiveCapabilities(scope targetenrollment.Scope) (targetsession.Capabilities, error) {
	if err := t.validate(scope); err != nil {
		return targetsession.Capabilities{}, err
	}
	return t.base.LiveCapabilities(scope)
}

func (t *applicationRemoteTransport) Dispatch(ctx context.Context, scope targetenrollment.Scope, request targetsession.Request) (targetsession.Response, error) {
	if err := ctx.Err(); err != nil {
		return targetsession.Response{}, err
	}
	if err := t.validate(scope); err != nil {
		return targetsession.Response{}, err
	}
	return t.base.Dispatch(ctx, scope, request)
}
