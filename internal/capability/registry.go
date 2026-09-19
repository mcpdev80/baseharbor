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
)

const RegistryVersion = 1

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
	ID               string            `json:"id"`
	Provider         Provider          `json:"provider"`
	Scope            ProviderScope     `json:"scope"`
	SharingBoundary  string            `json:"sharing_boundary,omitempty"`
	Ownership        ProviderOwnership `json:"ownership"`
	OwnerApplication string            `json:"owner_application,omitempty"`
	Reference        string            `json:"reference,omitempty"`
}

type ProviderBinding struct {
	Resource           Resource `json:"resource"`
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
	for _, existing := range r.Instances {
		if existing.ID == instance.ID {
			if sameProviderInstance(existing, instance) {
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

func (r Registry) Resolve(provider ProviderKind, scope ProviderScope, application, externalID string) (ProviderInstance, error) {
	application = strings.TrimSpace(application)
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
			if instance.OwnerApplication == application {
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

func (r Registry) ResolvePlacement(provider ProviderKind, placement ProviderPlacement, application string) (ProviderInstance, error) {
	if err := placement.Validate(); err != nil {
		return ProviderInstance{}, err
	}
	application = strings.TrimSpace(application)
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
			if instance.OwnerApplication == application {
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

func (r *Registry) Bind(resource Resource, providerInstanceID string) error {
	if r == nil {
		return errors.New("provider registry is nil")
	}
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
	if instance.Scope == ScopeApplication && instance.OwnerApplication != resource.Application {
		return fmt.Errorf("provider instance %q belongs to application %q, not %q", instance.ID, instance.OwnerApplication, resource.Application)
	}
	for _, binding := range r.Bindings {
		if sameLogicalResource(binding.Resource, resource) {
			if binding.ProviderInstanceID == providerInstanceID {
				return nil
			}
			return fmt.Errorf("resource %s/%s/%s is already bound to provider instance %q", resource.Application, resource.Kind, resource.Name, binding.ProviderInstanceID)
		}
	}
	r.Bindings = append(r.Bindings, ProviderBinding{Resource: resource, ProviderInstanceID: providerInstanceID})
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

func (r Registry) ApplicationLifecycle(application string, operation LifecycleOperation) ([]LifecycleAction, error) {
	application = strings.TrimSpace(application)
	if application == "" {
		return nil, errors.New("application is required")
	}
	switch operation {
	case LifecycleUpdate, LifecycleBackup, LifecycleDestroy:
	default:
		return nil, fmt.Errorf("unsupported provider lifecycle operation %q", operation)
	}
	seen := map[string]struct{}{}
	var actions []LifecycleAction
	for _, binding := range r.Bindings {
		if binding.Resource.Application != application {
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
		ownedDedicated := instance.Scope == ScopeApplication && instance.Ownership == OwnershipBaseHarbor && instance.OwnerApplication == application
		actions = append(actions, LifecycleAction{
			ProviderInstanceID: instance.ID, Scope: instance.Scope, Ownership: instance.Ownership,
			Operation: operation, MutateProvider: ownedDedicated, RemoveBinding: operation == LifecycleDestroy,
		})
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ProviderInstanceID < actions[j].ProviderInstanceID })
	return actions, nil
}

func (r *Registry) ReleaseManagedApplication(application string) {
	if r == nil {
		return
	}
	application = strings.TrimSpace(application)
	bindings := r.Bindings[:0]
	for _, binding := range r.Bindings {
		if binding.Resource.Application != application {
			bindings = append(bindings, binding)
			continue
		}
		instance, ok := r.instance(binding.ProviderInstanceID)
		if ok && instance.Ownership == OwnershipExternal {
			bindings = append(bindings, binding)
		}
	}
	r.Bindings = bindings
	r.removeOwnedApplicationInstances(application)
}

func (r *Registry) ReleaseApplication(application string) {
	if r == nil {
		return
	}
	application = strings.TrimSpace(application)
	bindings := r.Bindings[:0]
	for _, binding := range r.Bindings {
		if binding.Resource.Application != application {
			bindings = append(bindings, binding)
		}
	}
	r.Bindings = bindings
	r.removeOwnedApplicationInstances(application)
}

func (r *Registry) removeOwnedApplicationInstances(application string) {
	instances := r.Instances[:0]
	for _, instance := range r.Instances {
		if instance.Scope == ScopeApplication && instance.Ownership == OwnershipBaseHarbor && instance.OwnerApplication == application {
			continue
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
		key := binding.Resource.Application + "\x00" + string(binding.Resource.Kind) + "\x00" + binding.Resource.Name
		if _, exists := seenResources[key]; exists {
			return fmt.Errorf("duplicate binding for resource %s/%s/%s", binding.Resource.Application, binding.Resource.Kind, binding.Resource.Name)
		}
		seenResources[key] = struct{}{}
		instance, ok := r.instance(binding.ProviderInstanceID)
		if !ok {
			return fmt.Errorf("binding references missing provider instance %q", binding.ProviderInstanceID)
		}
		if binding.Resource.Provider != instance.Provider.Kind || !instance.Provider.Supports(binding.Resource.Kind) {
			return fmt.Errorf("binding for %s/%s/%s is incompatible with provider instance %q", binding.Resource.Application, binding.Resource.Kind, binding.Resource.Name, instance.ID)
		}
		if instance.Scope == ScopeApplication && instance.OwnerApplication != binding.Resource.Application {
			return fmt.Errorf("binding crosses application ownership boundary for provider instance %q", instance.ID)
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
		if a.Application != b.Application {
			return a.Application < b.Application
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
		if instance.OwnerApplication != "" {
			return fmt.Errorf("shared provider instance %q cannot have an application owner", instance.ID)
		}
		if instance.Ownership != OwnershipBaseHarbor {
			return fmt.Errorf("shared provider instance %q must be BaseHarbor-owned; use external scope for BYO providers", instance.ID)
		}
	case ScopeApplication:
		if strings.TrimSpace(instance.SharingBoundary) != "" {
			return fmt.Errorf("application-scoped provider instance %q cannot define a sharing boundary", instance.ID)
		}
		if strings.TrimSpace(instance.OwnerApplication) == "" {
			return fmt.Errorf("application-scoped provider instance %q requires an owner application", instance.ID)
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
		if instance.OwnerApplication != "" {
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

func sameProviderInstance(a, b ProviderInstance) bool {
	if a.ID != b.ID || a.Provider.Kind != b.Provider.Kind || a.Scope != b.Scope ||
		a.SharingBoundary != b.SharingBoundary || a.Ownership != b.Ownership || a.OwnerApplication != b.OwnerApplication || a.Reference != b.Reference ||
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
