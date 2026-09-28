package runtime

import (
	"context"
	"io"
)

// RuntimeProvider is the provider-neutral execution surface used by BaseHarbor
// orchestration. Implementations may use Docker Compose, Podman Quadlet or a
// future runtime mechanism internally, but those mechanics must not leak into
// application/core orchestration.
type RuntimeProvider interface {
	Provider

	Engine() string
	PreferredLocalHTTPSPort() int
	LogCollectionMode() LogCollectionMode
	VerifyProjectServiceLogCollection(context.Context, string, string, string) error

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

var _ RuntimeProvider = DockerProvider{}
var _ RuntimeProvider = PodmanProvider{}
