package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

// ExternalProviderRemoval observes registration/binding owners only. It never
// promises that unbinding reconfigures a running workload or revokes credentials.
type ExternalProviderRemoval struct {
	Scope        ProviderImpactScope `json:"scope"`
	registries   []externalRemovalRegistry
	registration externalprovider.Registration
}

type externalRemovalRegistry struct {
	store    capability.RegistryStore
	registry capability.Registry
	digest   string
}

func registrationRevision(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// PlanExternalProviderRemovalAt scans persisted Target owners, including Targets
// absent from current configuration. The global registration is not a Target.
func PlanExternalProviderRemovalAt(dataDir, id string) (ExternalProviderRemoval, error) {
	var result ExternalProviderRemoval
	store, err := externalProviderStoreAt(dataDir)
	if err != nil {
		return result, err
	}
	reg, err := store.Inspect(id)
	if err != nil {
		return result, err
	}
	result.registration = reg
	global := capability.RegistryStore{Path: filepath.Join(dataDir, providerRegistryFile)}
	if err := verifyExternalRemovalRegistryOwner(global.Path); err != nil {
		return result, err
	}
	registry, err := global.Load()
	if err != nil {
		return result, err
	}
	instanceID := externalProviderInstanceID(reg.Provider.Kind, reg.ID)
	if len(registry.BindingsForProviderInstance(instanceID)) != 0 {
		return result, fmt.Errorf("global provider bindings have no verified Target attribution; recover their authoritative Target before removal")
	}
	result.registries = append(result.registries, externalRemovalRegistry{global, registry, registrationRevision(registry)})
	entries, err := os.ReadDir(filepath.Join(dataDir, "targets"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	plans := []ProviderImpactPlan{}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("Target owner cannot be a symbolic link")
		}
		if !entry.IsDir() {
			continue
		}
		if err := deployment.ValidateTargetName(entry.Name()); err != nil {
			return result, err
		}
		targetStore := capability.RegistryStore{Path: filepath.Join(dataDir, "targets", entry.Name(), providerRegistryFile)}
		if err := verifyExternalRemovalRegistryOwner(targetStore.Path); err != nil {
			return result, err
		}
		r, err := targetStore.Load()
		if err != nil {
			return result, err
		}
		// Hash even unaffected registries: a new binding anywhere invalidates
		// the global consent before any registration owner is removed.
		result.registries = append(result.registries, externalRemovalRegistry{targetStore, r, registrationRevision(r)})
		var instance *capability.ProviderInstance
		for index := range r.Instances {
			if r.Instances[index].ID == instanceID {
				instance = &r.Instances[index]
				break
			}
		}
		if instance == nil {
			continue
		}
		if instance.Ownership != capability.OwnershipExternal || instance.Scope != capability.ScopeExternal || instance.Reference != reg.ID || instance.Provider.Kind != reg.Provider.Kind {
			return result, &ProviderChangeError{Code: "external-registration-owner-mismatch"}
		}
		plan, err := externalRegistrationTargetImpact(entry.Name(), targetStore.Path, r, *instance, reg)
		if err != nil {
			return result, err
		}
		plans = append(plans, plan)
	}
	owner := ProviderScopeOwner{Owner: "external-provider-registration", Reference: store.Path, Subject: reg.ID, Revision: registrationRevision(struct {
		Registration externalprovider.Registration
		Registries   []string
	}{reg, externalRemovalDigests(result.registries)})}
	// Registration-only observation cannot verify all live/management consumers,
	// even when no Target binding is present. Explicit acceptance permits
	// unregistering; absence of bindings cannot become a continuity receipt.
	result.Scope, err = ComposeProviderImpactScope(owner, plans, false)
	return result, err
}

func externalRemovalDigests(registries []externalRemovalRegistry) []string {
	result := []string{}
	for _, r := range registries {
		result = append(result, r.store.Path+":"+r.digest)
	}
	return result
}

func externalRegistrationTargetImpact(target, path string, registry capability.Registry, instance capability.ProviderInstance, reg externalprovider.Registration) (ProviderImpactPlan, error) {
	snapshot := ResolutionSnapshot{Target: target, Registry: registry, Facts: map[string]ProviderFact{}}
	for _, p := range registry.Instances {
		snapshot.Facts[p.ID] = ProviderFact{Target: target, State: ProviderUnverifiable}
	}
	snapshot.Facts[instance.ID] = ProviderFact{Target: target, State: ProviderUnverifiable, Dependencies: []string{}}
	union, err := ResolveProviderPlan(snapshot, nil, false)
	if err != nil {
		return ProviderImpactPlan{}, err
	}
	observation := ProviderImpactObservation{Revision: registrationRevision(registry), InventoryComplete: false, Providers: map[string]ProviderImpactFact{
		instance.ID: {Realization: ProviderRealization{InstanceID: instance.ID, Kind: instance.Provider.Kind, Target: target, Scope: instance.Scope, Ownership: instance.Ownership, Owner: reg.ID, State: ProviderUnverifiable,
			Locations: []ResourceLocation{{Owner: reg.ID, Target: target, Kind: "external-store", Reference: reg.Endpoint}}, Supported: true, Verified: false},
			Materials: externalRegistrationMaterialReferences(target, reg), InventoryComplete: false,
			Effects: []OperationalConsequence{{Aspect: "binding", Before: "BaseHarbor provider registration and Target bindings exist at " + path, After: "BaseHarbor registration and matching bindings are removed; foreign infrastructure and existing workload configuration are unchanged; future resolution has no selected replacement", Evidence: "protected registry observation"},
				{Aspect: "continuity", Before: "live credential, session, certificate and data validity is unobserved", After: "continued access, expiry, renewal, revocation and recovery are unknown; no handoff or replacement is performed", Evidence: "registration-only operation has no runtime verification"}}},
	}}
	seen := map[string]bool{}
	for _, binding := range registry.BindingsForProviderInstance(instance.ID) {
		consumer := "application/" + binding.ApplicationID + "/" + binding.Environment
		key := consumer + "/" + string(binding.Resource.Kind)
		if seen[key] {
			continue
		}
		seen[key] = true
		observation.Functions = append(observation.Functions, ProviderFunctionUse{Consumer: consumer, InstanceID: instance.ID, Function: string(binding.Resource.Kind), Required: true, Behavior: "Application " + binding.Resource.Application + " retains its current workload configuration; BaseHarbor removes the selected registration and bindings without a replacement or verified access continuation", Evidence: "protected Target binding at " + path})
	}
	return DescribeProviderChanges(snapshot, union, []ProviderChangeRequest{{InstanceID: instance.ID, Operation: "unbind", Selection: "explicit", Consumers: []string{}, Materials: []MaterialDisposition{}}}, observation)
}

func externalRegistrationMaterialReferences(target string, reg externalprovider.Registration) []ProviderMaterial {
	result := []ProviderMaterial{}
	for _, reference := range []struct{ kind, ref string }{{"credential-reference", reg.CredentialRef}, {"ca-reference", reg.Trust.CAReference}, {"client-certificate-reference", reg.Trust.ClientCertificate}, {"client-key-reference", reg.Trust.ClientKey}, {"trust-directory-reference", reg.Trust.Directory}} {
		if reference.ref == "" {
			continue
		}
		result = append(result, ProviderMaterial{ID: reg.ID + "/" + reference.kind, Kind: reference.kind, Ownership: capability.OwnershipExternal, Location: &ResourceLocation{Owner: reg.ID, Target: target, Kind: "external-store", Reference: reference.ref}, Recovery: []ResourceLocation{}, Verified: false, Evidence: "registration contains this reference; material content, ownership, expiry and live delivery are unobserved and no material operation is performed"})
	}
	return result
}

type ExternalProviderRemovalResult struct {
	ID             string                           `json:"id"`
	Removed        bool                             `json:"removed"`
	ForeignMutated bool                             `json:"foreign_mutated"`
	Scope          ProviderImpactScopeResult        `json:"scope"`
	Bindings       []ExternalProviderBindingRelease `json:"bindings"`
}

type ExternalProviderBindingRelease struct {
	Reference string                       `json:"reference"`
	Released  []capability.ProviderBinding `json:"released"`
	Verified  bool                         `json:"verified"`
	Evidence  string                       `json:"evidence"`
}

func verifyExternalRemovalRegistryOwner(path string) error {
	for _, candidate := range []string{filepath.Dir(filepath.Dir(path)), filepath.Dir(path), path, path + ".lock"} {
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &ProviderChangeError{Code: "provider-registry-ownership-unverifiable"}
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Getuid()) {
			return &ProviderChangeError{Code: "provider-registry-ownership-unverifiable"}
		}
		if candidate != path && candidate != path+".lock" && (!info.IsDir() || info.Mode().Perm()&0022 != 0) {
			return &ProviderChangeError{Code: "provider-registry-ownership-unverifiable"}
		}
		if candidate == path || candidate == path+".lock" {
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || stat.Nlink != 1 {
				return &ProviderChangeError{Code: "provider-registry-ownership-unverifiable"}
			}
		}
	}
	return nil
}

// ApplyExternalProviderRemovalAt replans the entire scope, then compares each
// protected registry inside its update lock. A partial error retains the global
// owner for a fresh plan/retry; foreign runtime and material are never touched.
func ApplyExternalProviderRemovalAt(dataDir, id string, consent ProviderChangeConsent) (ExternalProviderRemovalResult, error) {
	result := ExternalProviderRemovalResult{ID: id, Bindings: []ExternalProviderBindingRelease{}}
	plan, err := PlanExternalProviderRemovalAt(dataDir, id)
	if err != nil {
		return result, err
	}
	if err := ValidateProviderImpactScopeConsent(plan.Scope, consent); err != nil {
		return result, err
	}
	result.Scope, err = RecordProviderImpactScopeResult(plan.Scope, ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: "retained", Verified: true, Evidence: "protected registration remains available for retry"}, nil)
	if err != nil {
		return result, err
	}
	instanceID := externalProviderInstanceID(plan.registration.Provider.Kind, id)
	for index, owner := range plan.registries {
		found := false
		for _, instance := range owner.registry.Instances {
			if instance.ID == instanceID {
				found = true
			}
		}
		if !found {
			continue
		}
		if err := verifyExternalRemovalRegistryOwner(owner.store.Path); err != nil {
			return result, err
		}
		var updatedDigest string
		if err := owner.store.Update(func(registry *capability.Registry) error {
			if registrationRevision(*registry) != owner.digest {
				return &ProviderChangeError{Code: "stale-impact-consent"}
			}
			bindings := []capability.ProviderBinding{}
			for _, binding := range registry.Bindings {
				if binding.ProviderInstanceID != instanceID {
					bindings = append(bindings, binding)
				}
			}
			registry.Bindings = bindings
			if err := registry.UnregisterExternal(instanceID); err != nil {
				return err
			}
			updatedDigest = registrationRevision(*registry)
			return nil
		}); err != nil {
			return result, err
		}
		actual, err := owner.store.Load()
		if err != nil {
			return result, err
		}
		if registrationRevision(actual) != updatedDigest {
			return result, &ProviderChangeError{Code: "stale-impact-consent"}
		}
		plan.registries[index].digest = updatedDigest
		result.Bindings = append(result.Bindings, ExternalProviderBindingRelease{Reference: owner.store.Path, Released: owner.registry.BindingsForProviderInstance(instanceID), Verified: true, Evidence: "protected registry read-back confirms matching projection and bindings absent; live workload is unchanged"})
	}
	store, err := externalProviderStoreAt(dataDir)
	if err != nil {
		return result, err
	}
	if err := store.Update(func(document *externalprovider.Document) error {
		// Registration ownership is checked while its existing lock is held.
		current, err := PlanExternalProviderRemovalAt(dataDir, id)
		if err != nil {
			return err
		}
		if registrationRevision(current.registration) != registrationRevision(plan.registration) || registrationRevision(externalRemovalDigests(current.registries)) != registrationRevision(externalRemovalDigests(plan.registries)) {
			return &ProviderChangeError{Code: "stale-impact-consent"}
		}
		items := []externalprovider.Registration{}
		found := false
		for _, reg := range document.Registrations {
			if reg.ID == id {
				if registrationRevision(reg.Public()) != registrationRevision(plan.registration) {
					return &ProviderChangeError{Code: "stale-impact-consent"}
				}
				found = true
			} else {
				items = append(items, reg)
			}
		}
		if !found {
			return &ProviderChangeError{Code: "stale-impact-consent"}
		}
		document.Registrations = items
		return nil
	}); err != nil {
		return result, err
	}
	items, err := store.List()
	if err != nil {
		return result, err
	}
	for _, item := range items {
		if item.ID == id {
			return result, fmt.Errorf("registration removal could not be verified")
		}
	}
	result.Removed = true
	// Only registration absence has been verified. No live consumer function
	// or data-location success is manufactured from deleted registry entries.
	result.Scope, err = RecordProviderImpactScopeResult(plan.Scope, ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: "removed", Verified: true, Evidence: "protected registration read-back confirms absence"}, nil)
	return result, err
}
