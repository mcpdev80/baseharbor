package runtimeexplorer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

type OwnershipEvidence struct {
	Ownership      Ownership
	Relationship   Relationship
	Reconciliation ReconciliationHint
}

type OwnershipResolver interface {
	ResolveContainer(context.Context, string, runtimecontract.ProviderKind, runtimecontract.RuntimeContainer) (OwnershipEvidence, error)
}

type ContainerBackend interface {
	Kind() runtimecontract.ProviderKind
	ListRuntimeContainers(context.Context) ([]runtimecontract.RuntimeContainer, error)
	ContainerLogs(context.Context, string, *time.Time, int, bool) (io.ReadCloser, error)
	OperateContainer(context.Context, string, Operation, []string) (string, error)
}

type Service struct {
	backend  ContainerBackend
	target   string
	resolver OwnershipResolver
}

func NewService(backend ContainerBackend, target string, resolver OwnershipResolver) (*Service, error) {
	if backend == nil {
		return nil, errors.New("runtime explorer backend is required")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("runtime explorer target is required")
	}
	return &Service{backend: backend, target: target, resolver: resolver}, nil
}

func (s *Service) Capabilities(context.Context, string) (CapabilitySet, error) {
	capabilities := []Capability{
		CapabilityResourceInspect,
		CapabilityLogs,
		CapabilityContainerLifecycle,
		CapabilityContainerExec,
	}
	resourceKinds := []ResourceKind{KindContainer}
	if inventory, ok := s.backend.(InventoryBackend); ok {
		for _, kind := range inventory.InventoryResourceKinds() {
			if kind == KindContainer || containsResourceKind(resourceKinds, kind) {
				continue
			}
			resourceKinds = append(resourceKinds, kind)
			if kind == KindPod {
				capabilities = append(capabilities, CapabilityPodInspect)
			}
		}
	}
	return CapabilitySet{
		ContractVersion: ContractVersion,
		Provider:        string(s.backend.Kind()),
		Target:          s.target,
		Capabilities:    capabilities,
		ResourceKinds:   resourceKinds,
	}, nil
}

func (s *Service) List(ctx context.Context, request ListRequest) ([]Resource, error) {
	if strings.TrimSpace(request.Target) != "" && strings.TrimSpace(request.Target) != s.target {
		return nil, fmt.Errorf("runtime explorer target %q does not match active target %q", request.Target, s.target)
	}

	var resources []Resource
	if len(request.Kinds) == 0 || containsResourceKind(request.Kinds, KindContainer) {
		containers, err := s.backend.ListRuntimeContainers(ctx)
		if err != nil {
			return nil, err
		}
		for _, container := range containers {
			resource, err := s.containerResource(ctx, container)
			if err != nil {
				return nil, err
			}
			if matchesListRequest(resource, request) {
				resources = append(resources, resource)
			}
		}
	}

	if inventory, ok := s.backend.(InventoryBackend); ok {
		for _, kind := range inventory.InventoryResourceKinds() {
			if kind == KindContainer || (len(request.Kinds) > 0 && !containsResourceKind(request.Kinds, kind)) {
				continue
			}
			items, err := inventory.ListInventoryResources(ctx, kind)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				resource, err := inventoryResource(s.target, string(s.backend.Kind()), item)
				if err != nil {
					return nil, err
				}
				if matchesListRequest(resource, request) {
					resources = append(resources, resource)
				}
			}
		}
	}

	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Ref.Kind != resources[j].Ref.Kind {
			return resources[i].Ref.Kind < resources[j].Ref.Kind
		}
		if resources[i].Ownership != resources[j].Ownership {
			return resources[i].Ownership < resources[j].Ownership
		}
		return resources[i].RuntimeName < resources[j].RuntimeName
	})
	return resources, nil
}

func (s *Service) Inspect(ctx context.Context, ref ResourceRef) (Resource, error) {
	if err := s.validateRef(ref); err != nil {
		return Resource{}, err
	}
	if ref.Kind == KindContainer {
		containers, err := s.backend.ListRuntimeContainers(ctx)
		if err != nil {
			return Resource{}, err
		}
		for _, container := range containers {
			if container.ID == ref.ResourceID {
				return s.containerResource(ctx, container)
			}
		}
		return Resource{}, fmt.Errorf("runtime resource %q was not found", ref.ResourceID)
	}

	inventory, ok := s.backend.(InventoryBackend)
	if !ok || !containsResourceKind(inventory.InventoryResourceKinds(), ref.Kind) {
		return Resource{}, fmt.Errorf("runtime resource kind %q is not supported by this explorer backend", ref.Kind)
	}
	items, err := inventory.ListInventoryResources(ctx, ref.Kind)
	if err != nil {
		return Resource{}, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.ResourceID) == ref.ResourceID {
			return inventoryResource(s.target, string(s.backend.Kind()), item)
		}
	}
	return Resource{}, fmt.Errorf("runtime resource %q was not found", ref.ResourceID)
}

func (s *Service) Logs(ctx context.Context, request LogRequest) (io.ReadCloser, error) {
	if err := s.validateRef(request.Resource); err != nil {
		return nil, err
	}
	if request.Resource.Kind != KindContainer {
		return nil, fmt.Errorf("logs are not supported for runtime resource kind %q", request.Resource.Kind)
	}
	return s.backend.ContainerLogs(ctx, request.Resource.ResourceID, request.Since, request.Tail, request.Follow)
}

func (s *Service) Metrics(context.Context, ResourceRef) (MetricsHandle, error) {
	return MetricsHandle{Available: false}, nil
}

func (s *Service) Exec(ctx context.Context, request OperationRequest) (io.ReadCloser, error) {
	if request.Operation != OperationExec {
		return nil, errors.New("runtime exec stream requires exec operation")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if err := s.validateRef(request.Resource); err != nil {
		return nil, err
	}
	if request.Resource.Kind != KindContainer {
		return nil, fmt.Errorf("runtime exec is not supported for resource kind %q", request.Resource.Kind)
	}
	resource, err := s.Inspect(ctx, request.Resource)
	if err != nil {
		return nil, err
	}
	if resource.Ownership == OwnershipExternal || resource.Ownership == OwnershipUnmanaged {
		return nil, fmt.Errorf("runtime resource ownership %q does not permit exec", resource.Ownership)
	}
	output, err := s.backend.OperateContainer(ctx, request.Resource.ResourceID, OperationExec, request.Command)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(output)), nil
}

func (s *Service) Operate(ctx context.Context, request OperationRequest) (OperationResult, error) {
	if err := request.Validate(); err != nil {
		return OperationResult{}, err
	}
	if err := s.validateRef(request.Resource); err != nil {
		return OperationResult{}, err
	}
	if request.Resource.Kind != KindContainer {
		return OperationResult{}, fmt.Errorf("runtime lifecycle mutation is not supported for resource kind %q", request.Resource.Kind)
	}
	resource, err := s.Inspect(ctx, request.Resource)
	if err != nil {
		return OperationResult{}, err
	}
	if resource.Ownership == OwnershipExternal || resource.Ownership == OwnershipUnmanaged {
		return OperationResult{}, fmt.Errorf("runtime resource ownership %q does not permit direct mutation", resource.Ownership)
	}
	if _, err := s.backend.OperateContainer(ctx, request.Resource.ResourceID, request.Operation, request.Command); err != nil {
		return OperationResult{}, err
	}
	updated, err := s.Inspect(ctx, request.Resource)
	if err != nil {
		return OperationResult{}, err
	}
	return OperationResult{
		Resource:       request.Resource,
		Operation:      request.Operation,
		State:          updated.State,
		Reconciliation: updated.Reconciliation,
	}, nil
}

func (s *Service) validateRef(ref ResourceRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Provider != string(s.backend.Kind()) {
		return fmt.Errorf("runtime resource provider %q does not match active provider %q", ref.Provider, s.backend.Kind())
	}
	if ref.Target != s.target {
		return fmt.Errorf("runtime resource target %q does not match active target %q", ref.Target, s.target)
	}
	return nil
}

func (s *Service) containerResource(ctx context.Context, container runtimecontract.RuntimeContainer) (Resource, error) {
	if strings.TrimSpace(container.ID) == "" {
		return Resource{}, errors.New("runtime container has no stable id")
	}
	evidence := OwnershipEvidence{Ownership: OwnershipUnmanaged}
	if s.resolver != nil {
		resolved, err := s.resolver.ResolveContainer(ctx, s.target, s.backend.Kind(), container)
		if err != nil {
			return Resource{}, err
		}
		evidence = resolved
	}
	ready := container.Running && (container.Health == "" || strings.EqualFold(container.Health, "healthy"))
	state := ResourceState{
		Observed: strings.TrimSpace(container.State),
		Health:   strings.TrimSpace(container.Health),
		Ready:    &ready,
	}
	if state.Observed == "" {
		if container.Running {
			state.Observed = "running"
		} else {
			state.Observed = "stopped"
		}
	}
	resource := Resource{
		ContractVersion: ContractVersion,
		Ref: ResourceRef{
			Provider:   string(s.backend.Kind()),
			Target:     s.target,
			Kind:       KindContainer,
			ResourceID: container.ID,
		},
		DisplayName:    container.Name,
		RuntimeName:    container.Name,
		Ownership:      evidence.Ownership,
		Relationship:   evidence.Relationship,
		State:          state,
		Reconciliation: evidence.Reconciliation,
		References: map[string]string{
			"runtime.project": container.Project,
			"runtime.service": container.Service,
		},
	}
	if err := resource.Validate(); err != nil {
		return Resource{}, err
	}
	return resource, nil
}

func containsResourceKind(kinds []ResourceKind, want ResourceKind) bool {
	for _, kind := range kinds {
		if kind == want {
			return true
		}
	}
	return false
}

func matchesListRequest(resource Resource, request ListRequest) bool {
	if len(request.Ownership) > 0 {
		found := false
		for _, ownership := range request.Ownership {
			if resource.Ownership == ownership {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if request.ApplicationID != "" && request.ApplicationID != resource.Relationship.ApplicationID {
		return false
	}
	if request.DeploymentID != "" && request.DeploymentID != resource.Relationship.DeploymentID {
		return false
	}
	if request.Environment != "" && resource.Relationship.Environment != "" && request.Environment != resource.Relationship.Environment {
		return false
	}
	if request.Component != "" && request.Component != resource.Relationship.Component {
		return false
	}
	return true
}
