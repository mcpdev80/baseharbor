package main

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

func managementSecretsDependency(target string) capability.ProviderInstance {
	return capability.ProviderInstance{ID: "runtime/" + bhruntime.SharedProjectName(target) + "/control-plane/openbao", Provider: capability.Provider{Kind: capability.ProviderOpenBao}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
}

// A Core installation record owns recovery material, not an unconditional
// runtime lease. Only currently valid enrolled clients keep the Core API's SQL
// and certificate authority modules as independent consumers.
func appendManagementProviderDependencies(ctx context.Context, resolved resolvedApplication, observations map[string]application.ProviderFact, snapshot *application.ResolutionSnapshot) ([]string, error) {
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return nil, err
	}
	state, err := coreinstallation.Load(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	if state.Spec.Target != resolved.Target.Name || state.Spec.Runtime != resolved.Target.RuntimeProvider {
		return nil, invalidProviderConsumerInventory(errors.New("installation binding differs from selected Target"))
	}
	identity, identityOwner, err := identityprovider.CoreIdentityDependency(resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	for _, item := range []struct {
		instance capability.ProviderInstance
		owner    string
	}{{managementSecretsDependency(resolved.Target.Name), root}, {identity, identityOwner}} {
		fact := application.ProviderFact{Target: resolved.Target.Name, State: application.ProviderUnverifiable, Core: application.CoreRequired, Ancillary: true, OwnerReference: item.owner, Dependencies: []string{application.RuntimeStorageDependencyID(resolved.Target.Name)}}
		if observed, ok := observations[item.instance.ID]; ok && len(observed.Dependencies) == 1 && observed.Dependencies[0] == application.RuntimeStorageDependencyID(resolved.Target.Name) {
			fact = observed
			fact.Ancillary, fact.OwnerReference = true, item.owner
			fact.Dependencies = []string{application.RuntimeStorageDependencyID(resolved.Target.Name)}
		}
		snapshot.Ancillary = append(snapshot.Ancillary, item.instance)
		snapshot.Facts[item.instance.ID] = fact
	}
	// A healthy but unused installation is not itself a consumer. An unreadable
	// enrollment/revocation inventory is treated conservatively as live so a
	// destroy can still remove the application while leaving uncertain Core
	// runtime services untouched.
	connected, known := false, false
	if state.Ready {
		connected, err = activeCoreConnections(ctx, resolved)
		known = err == nil
	}
	if !known {
		connected = true
	}
	snapshot.ProtectCoreForConnection = connected
	return managementProviderDependencyRoots(resolved, *snapshot)
}

func managementProviderDependencyRoots(resolved resolvedApplication, snapshot application.ResolutionSnapshot) ([]string, error) {
	if !snapshot.ProtectCoreForConnection {
		return []string{}, nil
	}
	// A valid device connection is an independent Core consumer: it relies on
	// the installation's SQL registry, certificate authority and Core Identity
	// login surface. Keep their dependency closure together while any such
	// connection remains enrolled.
	identity, _, err := identityprovider.CoreIdentityDependency(resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	roots := []string{application.RuntimeStorageDependencyID(resolved.Target.Name), managementSecretsDependency(resolved.Target.Name).ID, identity.ID}
	for _, instance := range snapshot.Registry.Instances {
		if instance.Ownership != capability.OwnershipBaseHarbor || instance.Scope != capability.ScopeShared {
			continue
		}
		if instance.Provider.Kind == capability.ProviderKeycloak || instance.Provider.Kind == capability.ProviderOpenBao && instance.SharingBoundary == "" && instance.Reference == "" {
			roots = append(roots, instance.ID)
		}
	}
	sort.Strings(roots)
	return roots, nil
}

func activeCoreConnections(ctx context.Context, resolved resolvedApplication) (bool, error) {
	if strings.TrimSpace(resolved.Target.TenantID) == "" {
		return false, errors.New("Core installation tenant is unavailable")
	}
	check, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	files, err := initialOperatorLocalCoreFiles(withTargetOverride(check, resolved.Target.Name), resolved.Target)
	if err != nil {
		return false, err
	}
	dsn, err := locallyOwnedCorePostgresDSN(files)
	if err != nil {
		return false, err
	}
	pool, err := database.Open(check, database.Config{DSN: dsn, ConnectTimeout: 10 * time.Second})
	if err != nil {
		return false, err
	}
	defer pool.Close()
	connectorStore := database.NewConnectorEnrollmentStore(pool)
	connectorActive, err := connectorStore.HasActiveCertificate(check, resolved.Target.TenantID)
	if err != nil {
		return false, err
	}
	if connectorActive {
		return true, nil
	}
	workstations, err := database.NewWorkstationEnrollmentStore(pool).ListRegistryFacts(check, resolved.Target.TenantID)
	if err != nil {
		return false, err
	}
	now := time.Now()
	var unexpired []database.WorkstationRegistryFact
	for _, fact := range workstations {
		if fact.ExpiresAt.After(now) {
			unexpired = append(unexpired, fact)
		}
	}
	if len(unexpired) == 0 {
		return false, nil
	}
	// OpenBao is needed only when an unexpired workstation leaf must be checked
	// against the authoritative revocation list. An empty/expired registry can
	// still be inventoried and retired while the Secrets runtime is stopped.
	owner, err := openWorkstationAuthority(withTargetOverride(check, resolved.Target.Name))
	if err != nil {
		return false, err
	}
	defer owner.close()
	if owner.target.Name != resolved.Target.Name || owner.target.TenantID != resolved.Target.TenantID {
		return false, errors.New("Core enrollment inventory belongs to a different Target")
	}
	for _, fact := range unexpired {
		revoked, err := owner.authority.RevokedState(check, ws.Scope{TenantID: owner.target.TenantID, Station: fact.Station})
		if err != nil {
			return false, err
		}
		if !revoked {
			return true, nil
		}
	}
	return false, nil
}

// Resolve the same native management module for facts and impact inventory.
func existingManagementIdentity(resolved resolvedApplication) (capability.ProviderInstance, bool) {
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return capability.ProviderInstance{}, false
	}
	state, err := coreinstallation.Load(root)
	if err != nil || state.Spec.Target != resolved.Target.Name || state.Spec.Runtime != resolved.Target.RuntimeProvider {
		return capability.ProviderInstance{}, false
	}
	instance, _, err := identityprovider.CoreIdentityDependency(resolved.TargetStateRoot, resolved.Target.Name)
	return instance, err == nil
}
