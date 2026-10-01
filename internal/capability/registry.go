package capability

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

const RegistryVersion = 2

type ProviderScope string

const (
	ScopeShared      ProviderScope = "shared"
	ScopeApplication ProviderScope = "application"
	ScopeExternal    ProviderScope = "external"
)

type ProviderOwnership string

const (
	OwnershipBaseHarbor ProviderOwnership = "baseharbor"
	OwnershipExternal   ProviderOwnership = "external"
)

type ProviderInstance struct {
	ID                 string            `json:"id"`
	ProviderID         string            `json:"provider_id,omitempty"`
	ProviderVersion    string            `json:"provider_version,omitempty"`
	ProviderProtocol   string            `json:"provider_protocol,omitempty"`
	Provider           Provider          `json:"provider"`
	Scope              ProviderScope     `json:"scope"`
	SharingBoundary    string            `json:"sharing_boundary,omitempty"`
	Ownership          ProviderOwnership `json:"ownership"`
	OwnerApplicationID string            `json:"owner_application_id,omitempty"`
	OwnerApplication   string            `json:"owner_application,omitempty"`
	OwnerEnvironment   string            `json:"owner_environment,omitempty"`
	Reference          string            `json:"reference,omitempty"`
}

type ProviderBinding struct {
	ApplicationID      string   `json:"application_id"`
	Resource           Resource `json:"resource"`
	Environment        string   `json:"environment,omitempty"`
	ProviderInstanceID string   `json:"provider_instance_id"`
}

type Registry struct {
	Version   int                `json:"version"`
	Instances []ProviderInstance `json:"instances,omitempty"`
	Bindings  []ProviderBinding  `json:"bindings,omitempty"`
}

func NewRegistry() Registry { return Registry{Version: RegistryVersion} }

func (r *Registry) Register(instance ProviderInstance) error {
	if r == nil {
		return errors.New("provider registry is nil")
	}
	if r.Version == 0 {
		r.Version = RegistryVersion
	}
	if err := validateProviderInstance(instance); err != nil {
		return err
	}
	for i := range r.Instances {
		existing := r.Instances[i]
		if existing.ID == instance.ID {
			if sameProviderInstance(existing, instance) {
				return nil
			}
			if providerInstanceDistributionMetadataMissing(existing) && sameProviderInstanceWithoutDistribution(existing, instance) {
				r.Instances[i] = instance
				return nil
			}
			return fmt.Errorf("provider instance %q already exists with different metadata", instance.ID)
		}
		if instance.Scope == ScopeShared && existing.Scope == ScopeShared &&
			existing.Provider.Kind == instance.Provider.Kind &&
			strings.TrimSpace(existing.SharingBoundary) == strings.TrimSpace(instance.SharingBoundary) {
			return fmt.Errorf("shared provider %q for sharing boundary %q already registered as %q", instance.Provider.Kind, instance.SharingBoundary, existing.ID)
		}
	}
	r.Instances = append(r.Instances, instance)
	return nil
}

func (r Registry) Resolve(provider ProviderKind, scope ProviderScope, applicationID, externalID string) (ProviderInstance, error) {
	applicationID = strings.TrimSpace(applicationID)
	externalID = strings.TrimSpace(externalID)
	var matches []ProviderInstance
	for _, instance := range r.Instances {
		if instance.Provider.Kind != provider || instance.Scope != scope {
			continue
		}
		switch scope {
		case ScopeShared:
			if strings.TrimSpace(instance.SharingBoundary) == "" {
				matches = append(matches, instance)
			}
		case ScopeApplication:
			if instance.OwnerApplicationID == applicationID {
				matches = append(matches, instance)
			}
		case ScopeExternal:
			if externalID != "" && instance.ID == externalID {
				matches = append(matches, instance)
			}
		}
	}
	if len(matches) == 0 {
		return ProviderInstance{}, fmt.Errorf("provider %q with scope %q not found", provider, scope)
	}
	if len(matches) != 1 {
		return ProviderInstance{}, fmt.Errorf("provider %q with scope %q is ambiguous", provider, scope)
	}
	return matches[0], nil
}

func (r Registry) ResolvePlacement(provider ProviderKind, placement ProviderPlacement, applicationID string) (ProviderInstance, error) {
	if err := placement.Validate(); err != nil {
		return ProviderInstance{}, err
	}
	applicationID = strings.TrimSpace(applicationID)
	boundary := strings.TrimSpace(placement.SharingBoundary)
	reference := strings.TrimSpace(placement.ExternalReference)
	var matches []ProviderInstance
	for _, instance := range r.Instances {
		if instance.Provider.Kind != provider || instance.Scope != placement.Scope {
			continue
		}
		switch placement.Scope {
		case ScopeShared:
			if strings.TrimSpace(instance.SharingBoundary) == boundary {
				matches = append(matches, instance)
			}
		case ScopeApplication:
			if instance.OwnerApplicationID == applicationID {
				matches = append(matches, instance)
			}
		case ScopeExternal:
			if strings.TrimSpace(instance.Reference) == reference {
				matches = append(matches, instance)
			}
		}
	}
	if len(matches) == 0 {
		return ProviderInstance{}, fmt.Errorf("provider %q with scope %q and sharing boundary %q not found", provider, placement.Scope, boundary)
	}
	if len(matches) != 1 {
		return ProviderInstance{}, fmt.Errorf("provider %q with scope %q and sharing boundary %q is ambiguous", provider, placement.Scope, boundary)
	}
	return matches[0], nil
}

func (r *Registry) Bind(resource Resource, applicationID, providerInstanceID string) error {
	return r.BindDeployment(resource, applicationID, "", providerInstanceID)
}

func (r *Registry) BindDeployment(resource Resource, applicationID, environment, providerInstanceID string) error {
	if r == nil {
		return errors.New("provider registry is nil")
	}
	applicationID = strings.TrimSpace(applicationID)
	if err := stableid.ValidateUUIDv4("application", applicationID); err != nil {
		return err
	}
	environment = strings.TrimSpace(environment)
	instance, ok := r.instance(providerInstanceID)
	if !ok {
		return fmt.Errorf("provider instance %q not found", providerInstanceID)
	}
	if resource.Provider != instance.Provider.Kind {
		return fmt.Errorf("resource provider %q does not match provider instance %q", resource.Provider, instance.Provider.Kind)
	}
	if !instance.Provider.Supports(resource.Kind) {
		return fmt.Errorf("provider instance %q does not support capability %q", instance.ID, resource.Kind)
	}
	if instance.Scope == ScopeApplication {
		if instance.OwnerApplicationID != applicationID {
			return fmt.Errorf("provider instance %q belongs to application_id %q, not %q", instance.ID, instance.OwnerApplicationID, applicationID)
		}
		if ownerEnvironment := strings.TrimSpace(instance.OwnerEnvironment); ownerEnvironment != "" && environment != "" && ownerEnvironment != environment {
			return fmt.Errorf("provider instance %q belongs to environment %q, not %q", instance.ID, ownerEnvironment, environment)
		}
	}
	for _, binding := range r.Bindings {
		if sameLogicalDeploymentResource(binding, applicationID, resource, environment) {
			if binding.ProviderInstanceID == providerInstanceID {
				return nil
			}
			return fmt.Errorf("resource %s/%s/%s/%s is already bound to provider instance %q", resource.Application, environment, resource.Kind, resource.Name, binding.ProviderInstanceID)
		}
	}
	r.Bindings = append(r.Bindings, ProviderBinding{ApplicationID: applicationID, Resource: resource, Environment: environment, ProviderInstanceID: providerInstanceID})
	return nil
}

type LifecycleOperation string

const (
	LifecycleUpdate  LifecycleOperation = "update"
	LifecycleBackup  LifecycleOperation = "backup"
	LifecycleDestroy LifecycleOperation = "destroy"
)

type LifecycleAction struct {
	ProviderInstanceID string             `json:"provider_instance_id"`
	Scope              ProviderScope      `json:"scope"`
	Ownership          ProviderOwnership  `json:"ownership"`
	Operation          LifecycleOperation `json:"operation"`
	MutateProvider     bool               `json:"mutate_provider"`
	RemoveBinding      bool               `json:"remove_binding"`
}

func (r Registry) ApplicationLifecycle(applicationID string, operation LifecycleOperation) ([]LifecycleAction, error) {
	applicationID = strings.TrimSpace(applicationID)
	if err := stableid.ValidateUUIDv4("application", applicationID); err != nil {
		return nil, err
	}
	switch operation {
	case LifecycleUpdate, LifecycleBackup, LifecycleDestroy:
	default:
		return nil, fmt.Errorf("unsupported provider lifecycle operation %q", operation)
	}
	seen := map[string]struct{}{}
	var actions []LifecycleAction
	for _, binding := range r.Bindings {
		if binding.ApplicationID != applicationID {
			continue
		}
		if _, exists := seen[binding.ProviderInstanceID]; exists {
			continue
		}
		instance, ok := r.instance(binding.ProviderInstanceID)
		if !ok {
			return nil, fmt.Errorf("binding references missing provider instance %q", binding.ProviderInstanceID)
		}
		seen[instance.ID] = struct{}{}
		ownedDedicated := instance.Scope == ScopeApplication && instance.Ownership == OwnershipBaseHarbor && instance.OwnerApplicationID == applicationID
		actions = append(actions, LifecycleAction{
			ProviderInstanceID: instance.ID, Scope: instance.Scope, Ownership: instance.Ownership,
			Operation: operation, MutateProvider: ownedDedicated, RemoveBinding: operation == LifecycleDestroy,
		})
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ProviderInstanceID < actions[j].ProviderInstanceID })
	return actions, nil
}

func (r *Registry) ReleaseManagedApplication(applicationID string) {
	if r == nil {
		return
	}
	applicationID = strings.TrimSpace(applicationID)
	bindings := r.Bindings[:0]
	for _, binding := range r.Bindings {
		if binding.ApplicationID != applicationID {
			bindings = append(bindings, binding)
			continue
		}
		instance, ok := r.instance(binding.ProviderInstanceID)
		if ok && instance.Ownership == OwnershipExternal {
			bindings = append(bindings, binding)
		}
	}
	r.Bindings = bindings
	r.removeUnboundOwnedInstances(applicationID)
}

func (r *Registry) ReleaseManagedDeployment(applicationID, environment string) {
	r.releaseDeployment(applicationID, environment, true)
}

func (r *Registry) ReleaseApplication(applicationID string) {
	if r == nil {
		return
	}
	applicationID = strings.TrimSpace(applicationID)
	bindings := r.Bindings[:0]
	for _, binding := range r.Bindings {
		if binding.ApplicationID != applicationID {
			bindings = append(bindings, binding)
		}
	}
	r.Bindings = bindings
	r.removeUnboundOwnedInstances(applicationID)
}

func (r *Registry) ReleaseApplicationDeployment(applicationID, environment string) {
	r.releaseDeployment(applicationID, environment, false)
}

func (r *Registry) releaseDeployment(applicationID, environment string, preserveExternal bool) {
	if r == nil {
		return
	}
	applicationID = strings.TrimSpace(applicationID)
	environment = strings.TrimSpace(environment)
	bindings := r.Bindings[:0]
	for _, binding := range r.Bindings {
		if binding.ApplicationID != applicationID {
			bindings = append(bindings, binding)
			continue
		}
		bindingEnvironment := strings.TrimSpace(binding.Environment)
		if bindingEnvironment != "" && bindingEnvironment != environment {
			bindings = append(bindings, binding)
			continue
		}
		if preserveExternal {
			if instance, ok := r.instance(binding.ProviderInstanceID); ok && instance.Ownership == OwnershipExternal {
				bindings = append(bindings, binding)
			}
		}
	}
	r.Bindings = bindings
	r.removeUnboundOwnedInstances(applicationID)
}

func (r *Registry) removeUnboundOwnedInstances(applicationID string) {
	referenced := make(map[string]struct{}, len(r.Bindings))
	for _, binding := range r.Bindings {
		referenced[binding.ProviderInstanceID] = struct{}{}
	}
	instances := r.Instances[:0]
	for _, instance := range r.Instances {
		if instance.Ownership != OwnershipBaseHarbor {
			instances = append(instances, instance)
			continue
		}
		if _, stillReferenced := referenced[instance.ID]; stillReferenced {
			instances = append(instances, instance)
			continue
		}
		switch instance.Scope {
		case ScopeShared:
			continue
		case ScopeApplication:
			if instance.OwnerApplicationID == applicationID {
				continue
			}
		}
		instances = append(instances, instance)
	}
	r.Instances = instances
}

func (r Registry) Validate() error {
	if r.Version != RegistryVersion {
		return fmt.Errorf("unsupported provider registry version %d", r.Version)
	}
	seenIDs := map[string]struct{}{}
	shared := map[string]string{}
	for _, instance := range r.Instances {
		if err := validateProviderInstance(instance); err != nil {
			return err
		}
		if _, exists := seenIDs[instance.ID]; exists {
			return fmt.Errorf("duplicate provider instance %q", instance.ID)
		}
		seenIDs[instance.ID] = struct{}{}
		if instance.Scope == ScopeShared {
			key := string(instance.Provider.Kind) + "\x00" + strings.TrimSpace(instance.SharingBoundary)
			if previous, exists := shared[key]; exists {
				return fmt.Errorf("duplicate shared provider %q for sharing boundary %q: %q and %q", instance.Provider.Kind, instance.SharingBoundary, previous, instance.ID)
			}
			shared[key] = instance.ID
		}
	}
	seenResources := map[string]struct{}{}
	for _, binding := range r.Bindings {
		environment := strings.TrimSpace(binding.Environment)
		if err := stableid.ValidateUUIDv4("application", binding.ApplicationID); err != nil {
			return fmt.Errorf("provider binding: %w", err)
		}
		key := binding.ApplicationID + "\x00" + environment + "\x00" + string(binding.Resource.Kind) + "\x00" + binding.Resource.Name
		if _, exists := seenResources[key]; exists {
			return fmt.Errorf("duplicate binding for resource %s/%s/%s/%s", binding.Resource.Application, environment, binding.Resource.Kind, binding.Resource.Name)
		}
		seenResources[key] = struct{}{}
		instance, ok := r.instance(binding.ProviderInstanceID)
		if !ok {
			return fmt.Errorf("binding references missing provider instance %q", binding.ProviderInstanceID)
		}
		if binding.Resource.Provider != instance.Provider.Kind || !instance.Provider.Supports(binding.Resource.Kind) {
			return fmt.Errorf("binding for %s/%s/%s is incompatible with provider instance %q", binding.Resource.Application, binding.Resource.Kind, binding.Resource.Name, instance.ID)
		}
		if instance.Scope == ScopeApplication {
			if instance.OwnerApplicationID != binding.ApplicationID {
				return fmt.Errorf("binding crosses application identity ownership boundary for provider instance %q", instance.ID)
			}
			if ownerEnvironment := strings.TrimSpace(instance.OwnerEnvironment); ownerEnvironment != "" && environment != "" && ownerEnvironment != environment {
				return fmt.Errorf("binding crosses environment ownership boundary for provider instance %q", instance.ID)
			}
		}
	}
	return nil
}

type RegistryStore struct{ Path string }

func (s RegistryStore) Load() (Registry, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return NewRegistry(), nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("read provider registry: %w", err)
	}
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return Registry{}, fmt.Errorf("decode provider registry: %w", err)
	}
	if err := registry.Validate(); err != nil {
		return Registry{}, fmt.Errorf("validate provider registry: %w", err)
	}
	return registry, nil
}

func (s RegistryStore) Update(mutate func(*Registry) error) error {
	if strings.TrimSpace(s.Path) == "" {
		return errors.New("provider registry path is required")
	}
	if mutate == nil {
		return errors.New("provider registry mutation is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("create provider registry directory: %w", err)
	}
	lock, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open provider registry lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock provider registry: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	registry, err := s.Load()
	if err != nil {
		return err
	}
	if err := mutate(&registry); err != nil {
		return err
	}
	return s.Save(registry)
}

func (s RegistryStore) Save(registry Registry) error {
	if strings.TrimSpace(s.Path) == "" {
		return errors.New("provider registry path is required")
	}
	if err := registry.Validate(); err != nil {
		return err
	}
	sort.Slice(registry.Instances, func(i, j int) bool { return registry.Instances[i].ID < registry.Instances[j].ID })
	sort.Slice(registry.Bindings, func(i, j int) bool {
		a, b := registry.Bindings[i].Resource, registry.Bindings[j].Resource
		ai, bi := registry.Bindings[i].ApplicationID, registry.Bindings[j].ApplicationID
		if ai != bi {
			return ai < bi
		}
		if a.Application != b.Application {
			return a.Application < b.Application
		}
		ae, be := strings.TrimSpace(registry.Bindings[i].Environment), strings.TrimSpace(registry.Bindings[j].Environment)
		if ae != be {
			return ae < be
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode provider registry: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("create provider registry directory: %w", err)
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write provider registry: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("secure provider registry: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace provider registry: %w", err)
	}
	return nil
}

func validateProviderInstance(instance ProviderInstance) error {
	if strings.TrimSpace(instance.ID) == "" {
		return errors.New("provider instance id is required")
	}
	if instance.Provider.Kind == "" || len(instance.Provider.Capabilities) == 0 {
		return fmt.Errorf("provider instance %q requires a provider descriptor", instance.ID)
	}
	switch instance.Scope {
	case ScopeShared:
		if instance.OwnerApplicationID != "" || instance.OwnerApplication != "" {
			return fmt.Errorf("shared provider instance %q cannot have an application owner", instance.ID)
		}
		if instance.Ownership != OwnershipBaseHarbor {
			return fmt.Errorf("shared provider instance %q must be BaseHarbor-owned; use external scope for BYO providers", instance.ID)
		}
	case ScopeApplication:
		if strings.TrimSpace(instance.SharingBoundary) != "" {
			return fmt.Errorf("application-scoped provider instance %q cannot define a sharing boundary", instance.ID)
		}
		if err := stableid.ValidateUUIDv4("application", instance.OwnerApplicationID); err != nil {
			return fmt.Errorf("application-scoped provider instance %q: %w", instance.ID, err)
		}
		if strings.TrimSpace(instance.OwnerApplication) == "" {
			return fmt.Errorf("application-scoped provider instance %q requires a readable owner application", instance.ID)
		}
		if instance.Ownership != OwnershipBaseHarbor {
			return fmt.Errorf("application-scoped provider instance %q must be BaseHarbor-owned", instance.ID)
		}
	case ScopeExternal:
		if strings.TrimSpace(instance.SharingBoundary) != "" {
			return fmt.Errorf("external provider instance %q cannot define a sharing boundary", instance.ID)
		}
		if instance.Ownership != OwnershipExternal {
			return fmt.Errorf("external provider instance %q must have external ownership", instance.ID)
		}
		if strings.TrimSpace(instance.Reference) == "" {
			return fmt.Errorf("external provider instance %q requires a non-secret reference", instance.ID)
		}
		if instance.OwnerApplicationID != "" || instance.OwnerApplication != "" {
			return fmt.Errorf("external provider instance %q cannot be lifecycle-owned by an application", instance.ID)
		}
	default:
		return fmt.Errorf("provider instance %q has unsupported scope %q", instance.ID, instance.Scope)
	}
	return nil
}

func (r Registry) instance(id string) (ProviderInstance, bool) {
	for _, instance := range r.Instances {
		if instance.ID == id {
			return instance, true
		}
	}
	return ProviderInstance{}, false
}

func sameLogicalResource(a, b Resource) bool {
	return a.Application == b.Application && a.Kind == b.Kind && a.Name == b.Name
}

func sameLogicalDeploymentResource(binding ProviderBinding, applicationID string, resource Resource, environment string) bool {
	return binding.ApplicationID == strings.TrimSpace(applicationID) &&
		binding.Resource.Kind == resource.Kind &&
		binding.Resource.Name == resource.Name &&
		binding.Resource.Provider == resource.Provider &&
		strings.TrimSpace(binding.Environment) == strings.TrimSpace(environment)
}

func providerInstanceDistributionMetadataMissing(instance ProviderInstance) bool {
	return strings.TrimSpace(instance.ProviderID) == "" &&
		strings.TrimSpace(instance.ProviderVersion) == "" &&
		strings.TrimSpace(instance.ProviderProtocol) == ""
}

func sameProviderInstanceWithoutDistribution(a, b ProviderInstance) bool {
	a.ProviderID, a.ProviderVersion, a.ProviderProtocol = "", "", ""
	b.ProviderID, b.ProviderVersion, b.ProviderProtocol = "", "", ""
	return sameProviderInstance(a, b)
}

func sameProviderInstance(a, b ProviderInstance) bool {
	if a.ID != b.ID || a.ProviderID != b.ProviderID || a.ProviderVersion != b.ProviderVersion || a.ProviderProtocol != b.ProviderProtocol ||
		a.Provider.Kind != b.Provider.Kind || a.Scope != b.Scope ||
		a.SharingBoundary != b.SharingBoundary || a.Ownership != b.Ownership || a.OwnerApplicationID != b.OwnerApplicationID || a.OwnerApplication != b.OwnerApplication || a.OwnerEnvironment != b.OwnerEnvironment || a.Reference != b.Reference ||
		len(a.Provider.Capabilities) != len(b.Provider.Capabilities) {
		return false
	}
	for i := range a.Provider.Capabilities {
		if a.Provider.Capabilities[i] != b.Provider.Capabilities[i] {
			return false
		}
	}
	return true
}
