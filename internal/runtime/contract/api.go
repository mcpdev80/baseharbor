package contract

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type ProviderKind string

const (
	ProviderDocker     ProviderKind = "docker"
	ProviderPodman     ProviderKind = "podman"
	ProviderKubernetes ProviderKind = "kubernetes"
	ProviderOpenShift  ProviderKind = "openshift"
)

type RuntimeCapability string

const RuntimeProviderContractVersion = "baseharbor.runtime/v1"

const (
	CapabilityWorkloadLifecycle RuntimeCapability = "workload-lifecycle"
	CapabilityServiceExec       RuntimeCapability = "service-exec"
	CapabilityPublishedPorts    RuntimeCapability = "published-ports"
	CapabilityResourceOwnership RuntimeCapability = "resource-ownership"
)

type ProviderCapabilities struct {
	WorkloadLifecycle bool
	ServiceExec       bool
	PublishedPorts    bool
	ResourceOwnership bool
}

func (c ProviderCapabilities) Supports(capability RuntimeCapability) bool {
	switch capability {
	case CapabilityWorkloadLifecycle:
		return c.WorkloadLifecycle
	case CapabilityServiceExec:
		return c.ServiceExec
	case CapabilityPublishedPorts:
		return c.PublishedPorts
	case CapabilityResourceOwnership:
		return c.ResourceOwnership
	default:
		return false
	}
}

type ProviderDescriptor struct {
	Kind            ProviderKind
	ContractVersion string
	ProviderVersion string
	Standards       []string
	WorkloadSources []string
	Realization     string
	Capabilities    ProviderCapabilities
}

type Provider interface {
	Kind() ProviderKind
	Descriptor() ProviderDescriptor
	Capabilities() ProviderCapabilities
}

type LogCollectionMode string

const (
	LogCollectionSyslog   LogCollectionMode = "syslog"
	LogCollectionJournald LogCollectionMode = "journald"
)

type LogSourceAdapter interface {
	LogCollectionMode() LogCollectionMode
	VerifyProjectServiceLogCollection(context.Context, string, string, string) error
}

type ProjectResource struct {
	Kind string
	Name string
}

type RuntimeContainer struct {
	Name     string
	Project  string
	Service  string
	Running  bool
	State    string
	Health   string
	ExitCode int
	Error    string
}

type ImageIdentity struct {
	Reference string
	ImageID   string
	Digest    string
}

type PublishedPort struct {
	URL           string
	TargetPort    int
	PublishedPort int
	Protocol      string
}

type ServiceState struct {
	Service    string
	State      string
	Health     string
	ExitCode   int
	Error      string
	Publishers []PublishedPort
}

func (s ServiceState) TerminalFailure() bool {
	state := strings.ToLower(strings.TrimSpace(s.State))
	if strings.TrimSpace(s.Error) != "" {
		return true
	}
	if state == "exited" || state == "dead" {
		return true
	}
	return state != "running" && s.ExitCode != 0
}

func (s ServiceState) Ready() bool {
	if !strings.EqualFold(strings.TrimSpace(s.State), "running") {
		return false
	}
	health := strings.ToLower(strings.TrimSpace(s.Health))
	return health == "" || health == "healthy"
}

type RuntimeProvider interface {
	Provider
	LogSourceAdapter

	PreferredLocalHTTPSPort() int

	Up(context.Context, string, string) error
	Down(context.Context, string, string) error
	Status(context.Context, string, string) (string, error)
	Config(context.Context, string, string) error
	UpProject(context.Context, string, string, string) error
	UpProjectProgress(context.Context, string, string, string, func(string)) error
	DownProject(context.Context, string, string, string) error
	StopProject(context.Context, string, string, string) error
	DownProjectRemoveOrphans(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
	DestroyProjectRemoveOrphans(context.Context, string, string, string) error
	StatusProject(context.Context, string, string, string) (string, error)
	LogsProject(context.Context, string, string, string, ...string) (string, error)
	DiagnosticsProject(context.Context, string, string, string) string
	ConfigProject(context.Context, string, string, string) error
	ExecProject(context.Context, string, string, string, string, ...string) (string, error)
	ExecProjectInput(context.Context, string, string, string, []byte, string, ...string) (string, error)
	ConfigProjectFiles(context.Context, string, string, ...string) error
	ConfigProjectFilesEnv(context.Context, string, string, map[string]string, ...string) error
	ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error)
	UpProjectFiles(context.Context, string, string, ...string) error
	UpProjectFilesSelected(context.Context, string, string, map[string]string, []string, ...string) error
	BuildProjectFilesSelectedProgress(context.Context, string, string, map[string]string, []string, func(string), ...string) error
	UpProjectFilesSelectedForceRecreateNoBuild(context.Context, string, string, map[string]string, []string, ...string) error
	UpProjectFilesSelectedNoBuildProgress(context.Context, string, string, map[string]string, []string, func(string), ...string) error
	UpProjectFilesSelectedProgress(context.Context, string, string, map[string]string, []string, func(string), ...string) error
	DownProjectFiles(context.Context, string, string, ...string) error
	DownProjectFilesEnv(context.Context, string, string, map[string]string, ...string) error
	StopProjectFilesSelected(context.Context, string, string, map[string]string, []string, ...string) error
	StatusProjectFiles(context.Context, string, string, ...string) (string, error)
	ExecProjectFiles(context.Context, string, string, string, []string, ...string) (string, error)
	ExecProjectFilesInput(context.Context, string, string, string, []string, []byte, ...string) (string, error)
	ServicesProjectFiles(context.Context, string, string, ...string) ([]string, error)
	ServicesProjectFilesEnv(context.Context, string, string, map[string]string, ...string) ([]string, error)
	RunningServicesProjectFiles(context.Context, string, string, ...string) ([]string, error)
	RunningServicesProjectFilesEnv(context.Context, string, string, map[string]string, ...string) ([]string, error)
	ServiceStatesProjectFilesEnv(context.Context, string, string, map[string]string, ...string) ([]ServiceState, error)
	RunProjectFilesEnv(context.Context, string, string, map[string]string, io.Reader, io.Writer, io.Writer, []string, ...string) error
	PullImage(context.Context, string) error
	ContainerHealthStatus(context.Context, string) (string, error)
	ContainerNetworks(context.Context, string) ([]string, error)
	NetworkProjectOwner(context.Context, string) (string, error)
	ContainerExposedTCPPorts(context.Context, string) ([]int, error)
	EnsureManagedNetwork(context.Context, string) error
	ConnectManagedNetwork(context.Context, string, string, string) error
	DisconnectManagedNetwork(context.Context, string, string) error
	RemoveManagedNetwork(context.Context, string) error
	ProjectServiceLogDriver(context.Context, string, string) (string, error)
	RunningServicesProject(context.Context, string, string, string) ([]string, error)
	InspectProjectResource(context.Context, string, ProjectResource) (bool, error)
	InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error)
	DestroyOwnedProjectResources(context.Context, string, []ProjectResource) error
	ListRuntimeContainers(context.Context) ([]RuntimeContainer, error)
	ProjectServiceImageIdentity(context.Context, string, string) (ImageIdentity, error)
	ContainerLogConfigProjectService(context.Context, string, string) (string, string, error)
	ExportOwnedVolume(context.Context, string, string) ([]byte, error)
	EnsureOwnedVolume(context.Context, string, string) error
	RestoreOwnedVolume(context.Context, string, string, []byte) error
}

type ProviderFactory func(context.Context) (Provider, error)

type ProviderRegistration struct {
	Descriptor ProviderDescriptor
	Factory    ProviderFactory
}

type ProviderRegistry struct {
	registrations map[ProviderKind]ProviderRegistration
}

var providerKindPattern = regexp.MustCompile("^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?(?:/[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?)*$")

func ParseProviderKind(value string) (ProviderKind, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return ProviderDocker, nil
	}
	if !providerKindPattern.MatchString(normalized) {
		return "", fmt.Errorf("invalid runtime provider %q", value)
	}
	return ProviderKind(normalized), nil
}

func NewProviderRegistry(registrations ...ProviderRegistration) (*ProviderRegistry, error) {
	registry := &ProviderRegistry{registrations: make(map[ProviderKind]ProviderRegistration, len(registrations))}
	for _, registration := range registrations {
		if err := registry.Register(registration); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *ProviderRegistry) Register(registration ProviderRegistration) error {
	if r == nil {
		return fmt.Errorf("runtime provider registry is required")
	}
	if registration.Descriptor.Kind == "" {
		return fmt.Errorf("runtime provider descriptor kind is required")
	}
	kind, err := ParseProviderKind(string(registration.Descriptor.Kind))
	if err != nil {
		return err
	}
	if kind != registration.Descriptor.Kind {
		return fmt.Errorf("runtime provider descriptor kind %q is not normalized", registration.Descriptor.Kind)
	}
	if registration.Descriptor.ContractVersion != RuntimeProviderContractVersion {
		return fmt.Errorf("runtime provider %q uses contract %q, require %q", kind, registration.Descriptor.ContractVersion, RuntimeProviderContractVersion)
	}
	if strings.TrimSpace(registration.Descriptor.ProviderVersion) == "" {
		return fmt.Errorf("runtime provider %q has no provider version", kind)
	}
	if registration.Factory == nil {
		return fmt.Errorf("runtime provider %q has no factory", kind)
	}
	if r.registrations == nil {
		r.registrations = map[ProviderKind]ProviderRegistration{}
	}
	if _, exists := r.registrations[kind]; exists {
		return fmt.Errorf("runtime provider %q is already registered", kind)
	}
	registration.Descriptor.Standards = append([]string(nil), registration.Descriptor.Standards...)
	registration.Descriptor.WorkloadSources = append([]string(nil), registration.Descriptor.WorkloadSources...)
	if len(registration.Descriptor.WorkloadSources) == 0 {
		return fmt.Errorf("runtime provider %q declares no workload sources", kind)
	}
	if strings.TrimSpace(registration.Descriptor.Realization) == "" {
		return fmt.Errorf("runtime provider %q declares no realization", kind)
	}
	r.registrations[kind] = registration
	return nil
}

func (r *ProviderRegistry) Descriptor(kind ProviderKind) (ProviderDescriptor, error) {
	normalized, err := ParseProviderKind(string(kind))
	if err != nil {
		return ProviderDescriptor{}, err
	}
	registration, ok := r.registrations[normalized]
	if !ok {
		return ProviderDescriptor{}, fmt.Errorf("runtime provider %q is not registered", normalized)
	}
	descriptor := registration.Descriptor
	descriptor.Standards = append([]string(nil), descriptor.Standards...)
	descriptor.WorkloadSources = append([]string(nil), descriptor.WorkloadSources...)
	return descriptor, nil
}

func (r *ProviderRegistry) Resolve(ctx context.Context, kind ProviderKind) (Provider, error) {
	normalized, err := ParseProviderKind(string(kind))
	if err != nil {
		return nil, err
	}
	registration, ok := r.registrations[normalized]
	if !ok {
		return nil, fmt.Errorf("runtime provider %q is not registered", normalized)
	}
	provider, err := registration.Factory(ctx)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("runtime provider %q factory returned nil", normalized)
	}
	descriptor := provider.Descriptor()
	if descriptor.Kind != registration.Descriptor.Kind ||
		descriptor.ContractVersion != registration.Descriptor.ContractVersion ||
		descriptor.ProviderVersion != registration.Descriptor.ProviderVersion ||
		descriptor.Realization != registration.Descriptor.Realization ||
		descriptor.Capabilities != registration.Descriptor.Capabilities {
		return nil, fmt.Errorf("runtime provider %q descriptor does not match registry declaration", normalized)
	}
	return provider, nil
}

func (r *ProviderRegistry) ResolveRuntimeProvider(ctx context.Context, kind ProviderKind) (RuntimeProvider, error) {
	provider, err := r.Resolve(ctx, kind)
	if err != nil {
		return nil, err
	}
	runtimeProvider, ok := provider.(RuntimeProvider)
	if !ok {
		return nil, fmt.Errorf("runtime provider %q does not implement the BaseHarbor runtime contract", provider.Kind())
	}
	return runtimeProvider, nil
}

func RequireCapabilities(provider Provider, required ...RuntimeCapability) error {
	if provider == nil {
		return fmt.Errorf("runtime provider is required")
	}
	descriptor := provider.Descriptor()
	if descriptor.ContractVersion != RuntimeProviderContractVersion {
		return fmt.Errorf("runtime provider %s uses contract %q, require %q", provider.Kind(), descriptor.ContractVersion, RuntimeProviderContractVersion)
	}
	if descriptor.Kind != provider.Kind() {
		return fmt.Errorf("runtime provider descriptor kind %q does not match provider %q", descriptor.Kind, provider.Kind())
	}
	for _, capability := range required {
		if capability == "" {
			return fmt.Errorf("runtime provider %s: empty capability requirement", provider.Kind())
		}
		if !descriptor.Capabilities.Supports(capability) {
			return fmt.Errorf("runtime provider %s does not support required capability %q", provider.Kind(), capability)
		}
	}
	return nil
}
