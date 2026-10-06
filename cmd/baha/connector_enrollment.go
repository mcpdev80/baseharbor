package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// ConnectorEnrollmentHTTP pins the protected issuer to Core's startup-selected
// local authority Target. Request preferences never select a signing authority.
func (e *bahaMachineExecutor) ConnectorEnrollmentHTTP(ctx context.Context, store targetenrollment.Store, authorityTarget string) (http.Handler, error) {
	authorityTarget = strings.TrimSpace(authorityTarget)
	if authorityTarget == "" {
		return nil, errors.New("Connector enrollment requires an explicit Core authority Target")
	}
	policy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil || policy.PKISource != serviceaccess.PKIManagedLocal {
		return nil, errors.New("Connector enrollment requires the existing managed-local Core authority")
	}
	startup := withTargetOverride(ctx, authorityTarget)
	target, err := effectiveTarget(startup)
	if err != nil || target.Name != authorityTarget || target.AccessProvider != string(targetaccess.ProviderLocal) ||
		(target.RuntimeProvider != "docker" && target.RuntimeProvider != "podman") {
		return nil, errors.New("Connector authority Target must resolve to a local Docker or Podman Core")
	}
	check, cancel := context.WithTimeout(startup, 30*time.Second)
	defer cancel()
	provider, files, err := openBaoRuntime(check)
	if err != nil {
		return nil, errors.New("Connector authority runtime is unavailable")
	}
	state, err := platformopenbao.Inspect(check, provider, files)
	if err != nil || !state.Initialized || state.Sealed || platformopenbao.CheckManager(check, provider, files) != nil {
		return nil, errors.New("Connector authority requires initialized, unsealed OpenBao and protected manager authentication")
	}
	issuer := platformopenbao.NewServiceIssuer(provider, files)
	if _, err := issuer.TrustBundle(check); err != nil {
		return nil, errors.New("Connector authority trust is unavailable")
	}
	authority, err := targetenrollment.New(store, issuer)
	if err != nil {
		return nil, err
	}
	return targetenrollment.NewHTTP(authority, resolveConnectorEnrollmentScope)
}

func resolveConnectorEnrollmentScope(ctx context.Context, targetID, nodeID, environment string) (targetenrollment.Scope, error) {
	tenant, ok := tenancy.FromContext(ctx)
	if !ok || tenant.TenantID == "" || tenant.ExternalIdentityID == "" || len(environment) > 64 ||
		environment != strings.ToLower(strings.TrimSpace(environment)) || deployment.ValidateTargetName(environment) != nil {
		return targetenrollment.Scope{}, targetenrollment.ErrDenied
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return targetenrollment.Scope{}, targetenrollment.ErrDenied
	}
	definition, exists := cfg.Targets[targetID]
	if !exists || definition.TenantID != tenant.TenantID || cfg.Access[definition.Access.Reference].Reference != nodeID {
		return targetenrollment.Scope{}, targetenrollment.ErrDenied
	}
	selected := withOrganizationEnvironment(withTargetOverride(ctx, targetID), environment)
	target, err := effectiveTarget(selected)
	if err != nil || target.Name != targetID || target.TenantID != tenant.TenantID ||
		target.AccessProvider != string(targetaccess.ProviderNodeConnector) {
		return targetenrollment.Scope{}, targetenrollment.ErrDenied
	}
	scope := targetenrollment.Scope{TenantID: tenant.TenantID, TargetID: target.Name, NodeID: nodeID, Runtime: target.RuntimeProvider}
	if scope.Validate() != nil {
		return targetenrollment.Scope{}, targetenrollment.ErrDenied
	}
	return scope, nil
}
