package podman

import (
	"context"
	"sort"
	"strings"
	"sync"
	"os/exec"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

var ErrRuntimeNotFound = bhruntime.ErrRuntimeNotFound
var ErrResourceOwnership = bhruntime.ErrResourceOwnership

type Provider struct{ bhruntime.Compose }
type PodmanProvider = Provider

type ProjectResource = runtimecontract.ProjectResource
type RuntimeContainer = runtimecontract.RuntimeContainer
type ImageIdentity = runtimecontract.ImageIdentity
type ServiceState = runtimecontract.ServiceState
type ProviderKind = runtimecontract.ProviderKind
type ProviderDescriptor = runtimecontract.ProviderDescriptor
type ProviderCapabilities = runtimecontract.ProviderCapabilities
type LogCollectionMode = runtimecontract.LogCollectionMode

const (
	ProviderPodman         = runtimecontract.ProviderPodman
	LogCollectionJournald  = runtimecontract.LogCollectionJournald
)

var capabilities = ProviderCapabilities{
	WorkloadLifecycle: true,
	ServiceExec:       true,
	PublishedPorts:    true,
	ResourceOwnership: true,
}

func New(ctx context.Context) (*Provider, error) {
	path, err := exec.LookPath("podman")
	if err != nil {
		return nil, ErrRuntimeNotFound
	}
	if !QuadletAvailable(ctx) {
		return nil, ErrRuntimeNotFound
	}
	return &Provider{Compose: bhruntime.NewCLIBackend(path)}, nil
}

func (p Provider) Kind() ProviderKind { return ProviderPodman }

func (p Provider) Descriptor() ProviderDescriptor {
	return Descriptor()
}

func Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Kind:            ProviderPodman,
		ContractVersion: runtimecontract.RuntimeProviderContractVersion,
		ProviderVersion: "0.4.17",
		Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
		WorkloadSources: []string{"compose-spec"},
		Realization:     "podman-quadlet-systemd-user",
		Capabilities:    capabilities,
	}
}

func (p Provider) Capabilities() ProviderCapabilities { return capabilities }
func (p Provider) PreferredLocalHTTPSPort() int       { return 8443 }

var projectEnvironmentCache = struct {
	sync.Mutex
	items map[string]map[string]string
}{items: map[string]map[string]string{}}

func cacheProjectEnvironment(project string, environment map[string]string) {
	if len(environment) == 0 {
		return
	}
	clone := make(map[string]string, len(environment))
	for key, value := range environment {
		clone[key] = value
	}
	projectEnvironmentCache.Lock()
	projectEnvironmentCache.items[project] = clone
	projectEnvironmentCache.Unlock()
}

func takeProjectEnvironment(project string) map[string]string {
	projectEnvironmentCache.Lock()
	defer projectEnvironmentCache.Unlock()
	environment := projectEnvironmentCache.items[project]
	if len(environment) == 0 {
		return nil
	}
	clone := make(map[string]string, len(environment))
	for key, value := range environment {
		clone[key] = value
	}
	return clone
}

func clearProjectEnvironment(project string) {
	projectEnvironmentCache.Lock()
	delete(projectEnvironmentCache.items, project)
	projectEnvironmentCache.Unlock()
}

func consolidatedProject(project string) bool {
	return strings.HasPrefix(strings.TrimSpace(project), "bh-")
}

func firstRuntimeLabel(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && value != "<no value>" {
			return value
		}
	}
	return ""
}

func serviceStatesFromRuntimeLabels(ctx context.Context, backend bhruntime.Compose, project string) ([]ServiceState, error) {
	containers, err := backend.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, err
	}
	byService := map[string]ServiceState{}
	for _, container := range containers {
		if container.Project != project {
			continue
		}
		state := "exited"
		if container.Running {
			state = "running"
		}
		current, exists := byService[container.Service]
		if !exists || (current.State != "running" && state == "running") {
			byService[container.Service] = ServiceState{
				Service: container.Service,
				State:   state,
				Health:  container.Health,
			}
		}
	}
	names := make([]string, 0, len(byService))
	for service := range byService {
		names = append(names, service)
	}
	sort.Strings(names)
	states := make([]ServiceState, 0, len(names))
	for _, service := range names {
		states = append(states, byService[service])
	}
	return states, nil
}

var _ runtimecontract.RuntimeProvider = (*Provider)(nil)
