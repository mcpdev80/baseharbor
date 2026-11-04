package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
	wshttp "github.com/mcpdev80/baseharbor/internal/workstationenrollment/http"
)

// workstationEnrollmentProduction binds the one-use workstation-enrollment
// boundary to the durable, tenant-scoped store and the managed-local OpenBao
// issuer at the authenticated management entry point. Scope is bound by the
// authenticated installation (tenancy), never by the workstation request: the
// request may carry the workstation-owned station label, but the installation
// tenant is the authenticated operator's tenant.
func (e *bahaMachineExecutor) WorkstationEnrollmentHTTP(ctx context.Context, store ws.Store, authorityTarget string) (http.Handler, error) {
	authorityTarget = strings.TrimSpace(authorityTarget)
	if authorityTarget == "" {
		return nil, errors.New("Workstation enrollment requires an explicit Core authority Target")
	}
	policy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil || policy.PKISource != serviceaccess.PKIManagedLocal {
		return nil, errors.New("Workstation enrollment requires the existing managed-local Core authority")
	}
	startup := withTargetOverride(ctx, authorityTarget)
	target, err := effectiveTarget(startup)
	if err != nil || target.Name != authorityTarget || target.AccessProvider != string(targetaccess.ProviderLocal) ||
		(target.RuntimeProvider != "docker" && target.RuntimeProvider != "podman") {
		return nil, errors.New("Workstation authority Target must resolve to a local Docker or Podman Core")
	}
	check, cancel := context.WithTimeout(startup, 30*time.Second)
	defer cancel()
	provider, files, err := openBaoRuntime(check)
	if err != nil {
		return nil, errors.New("Workstation authority runtime is unavailable")
	}
	state, err := platformopenbao.Inspect(check, provider, files)
	if err != nil || !state.Initialized || state.Sealed || platformopenbao.CheckManager(check, provider, files) != nil {
		return nil, errors.New("Workstation authority requires initialized, unsealed OpenBao and protected manager authentication")
	}
	issuer := platformopenbao.NewServiceIssuer(provider, files)
	if _, err := issuer.TrustBundle(check); err != nil {
		return nil, errors.New("Workstation authority trust is unavailable")
	}
	authority, err := ws.New(store, issuer)
	if err != nil {
		return nil, err
	}
	// Pin the same installation that owns enrollment. Machine input may select
	// an execution node, but never a different SQL/Secrets/Identity authority.
	e.coreAuthority = &target
	return wshttp.NewProductionHandler(wsProductionEnrollmentBridge{authority, store}, wshttp.ScopeResolverFromContext())
}

// wsProductionEnrollmentBridge adapts the one-use authority and the durable
// store to the production HTTP boundary. Create/Enroll delegate to the authority;
// RecordRegistryFact persists the approved registry owner via the durable store.
type wsProductionEnrollmentBridge struct {
	*ws.Authority
	store ws.Store
}

func (e wsProductionEnrollmentBridge) RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error {
	store, ok := e.store.(interface {
		RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error
	})
	if !ok {
		return errors.New("workstation production store does not support registry-fakt persistence")
	}
	return store.RecordRegistryFact(ctx, scope, issuedAt, recipient, scopeID, serial, expiresAt)
}
