package application

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

var ErrWorkloadComposeAmbiguous = errors.New("multiple application Compose files found")
var ErrWorkloadComposeNotFound = errors.New("application Compose file not found")

type WorkloadFiles struct {
	RepositoryRoot string
	Compose        string
	Override       string
	Services       []string
	Project        string
	Partial        bool
}

var conventionalWorkloadComposePaths = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yml",
	"docker-compose.yaml",
	"infrastructure/compose.yaml",
	"infrastructure/compose.yml",
	"infrastructure/docker-compose.yml",
	"infrastructure/docker-compose.yaml",
}

func WorkloadProjectName(m Manifest) string {
	return bhruntime.ApplicationProjectName("", m.Name, m.Environment)
}

func WorkloadProjectNameForNamespace(m Manifest, namespace string) string {
	return bhruntime.ApplicationProjectName(namespace, m.Name, m.Environment)
}

func WorkloadProjectNameForRuntime(m Manifest, runtime RuntimeFiles) string {
	if project := strings.TrimSpace(runtime.Project); project != "" {
		return project
	}
	return WorkloadProjectName(m)
}

func ResolveWorkloadCompose(repositoryRoot string) (string, bool, error) {
	if strings.TrimSpace(repositoryRoot) == "" {
		return "", false, nil
	}
	var found []string
	for _, candidate := range conventionalWorkloadComposePaths {
		path := filepath.Join(repositoryRoot, candidate)
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			found = append(found, path)
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
	}
	if len(found) == 0 {
		return "", false, nil
	}
	if len(found) > 1 {
		rel := make([]string, 0, len(found))
		for _, path := range found {
			value, _ := filepath.Rel(repositoryRoot, path)
			rel = append(rel, value)
		}
		sort.Strings(rel)
		return "", false, fmt.Errorf("%w: %s; select the authoritative repository workload source", ErrWorkloadComposeAmbiguous, strings.Join(rel, ", "))
	}
	return found[0], true, nil
}

func ResolveWorkloadComposeSource(repositoryRoot, sourcePath string) (string, error) {
	if strings.TrimSpace(repositoryRoot) == "" {
		return "", errors.New("repository root is required")
	}
	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	if sourcePath == "" || sourcePath == "." || filepath.IsAbs(sourcePath) || sourcePath == ".." || strings.HasPrefix(sourcePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Compose source path %q must stay inside the repository", sourcePath)
	}
	path := filepath.Join(repositoryRoot, sourcePath)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrWorkloadComposeNotFound, sourcePath)
		}
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Compose source path %s is not a regular file", sourcePath)
	}
	return path, nil
}

func MaterializeWorkload(repositoryRoot string, m Manifest, runtime RuntimeFiles) (WorkloadFiles, bool, error) {
	composePath, found, err := ResolveWorkloadCompose(repositoryRoot)
	if err != nil || !found {
		return WorkloadFiles{}, found, err
	}
	return materializeWorkloadFromComposePath(repositoryRoot, composePath, m, runtime)
}

func MaterializeWorkloadFromCompose(repositoryRoot, sourcePath string, m Manifest, runtime RuntimeFiles) (WorkloadFiles, bool, error) {
	composePath, err := ResolveWorkloadComposeSource(repositoryRoot, sourcePath)
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	return materializeWorkloadFromComposePath(repositoryRoot, composePath, m, runtime)
}

func materializeWorkloadFromComposePath(repositoryRoot, composePath string, m Manifest, runtime RuntimeFiles) (WorkloadFiles, bool, error) {
	services, err := composeServiceNames(composePath)
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	selected, err := selectWorkloadServices(m, services, WorkloadComponentNames(m))
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	values, err := readRuntimeEnv(runtime.Env)
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	override, err := workloadOverrideYAMLForFiles(m, selected, values, runtime)
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	overridePath := filepath.Join(runtime.Dir, "workload.override.yaml")
	if err := writeOwnerOnlyFile(overridePath, []byte(override)); err != nil {
		return WorkloadFiles{}, false, fmt.Errorf("write application workload override: %w", err)
	}
	return WorkloadFiles{
		RepositoryRoot: repositoryRoot,
		Compose:        composePath,
		Override:       overridePath,
		Services:       selected,
		Project:        WorkloadProjectNameForRuntime(m, runtime),
		Partial:        len(selected) != len(services),
	}, true, nil
}

func SelectedWorkloadServices(repositoryRoot string, m Manifest) ([]string, string, bool, error) {
	composePath, found, err := ResolveWorkloadCompose(repositoryRoot)
	if err != nil || !found {
		return nil, composePath, found, err
	}
	services, err := composeServiceNames(composePath)
	if err != nil {
		return nil, composePath, true, err
	}
	selected, err := selectWorkloadServices(m, services, WorkloadComponentNames(m))
	if err != nil {
		return nil, composePath, true, err
	}
	return selected, composePath, true, nil
}

func SelectedWorkloadServicesFromCompose(repositoryRoot, sourcePath string, m Manifest) ([]string, string, error) {
	composePath, err := ResolveWorkloadComposeSource(repositoryRoot, sourcePath)
	if err != nil {
		return nil, "", err
	}
	services, err := composeServiceNames(composePath)
	if err != nil {
		return nil, composePath, err
	}
	selected, err := selectWorkloadServices(m, services, WorkloadComponentNames(m))
	if err != nil {
		return nil, composePath, err
	}
	return selected, composePath, nil
}

func composeServiceNames(path string) ([]string, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open application Compose file: %w", err)
	}
	var document struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(source, &document); err != nil {
		return nil, fmt.Errorf("parse application Compose file %s: %w", path, err)
	}
	if len(document.Services) == 0 {
		return nil, fmt.Errorf("application Compose file %s contains no services", path)
	}
	services := make([]string, 0, len(document.Services))
	for name := range document.Services {
		if err := validateComposeServiceName(name); err != nil {
			return nil, fmt.Errorf("invalid application Compose service: %w", err)
		}
		services = append(services, name)
	}
	sort.Strings(services)
	return services, nil
}

func selectWorkloadServices(m Manifest, available, requested []string) ([]string, error) {
	availableSet := make(map[string]struct{}, len(available))
	for _, service := range available {
		if strings.HasPrefix(service, "baseharbor-internal-") {
			return nil, fmt.Errorf("application Compose service %q uses the reserved BaseHarbor internal service namespace", service)
		}
		availableSet[service] = struct{}{}
	}
	if len(requested) > 0 {
		selected := append([]string(nil), requested...)
		for _, service := range selected {
			if _, ok := availableSet[service]; !ok {
				return nil, fmt.Errorf("workload service %q is not present in the application Compose file", service)
			}
		}
		sort.Strings(selected)
		return selected, nil
	}

	shadowed := map[string]struct{}{}
	for _, instance := range SQLInstanceNames(m) {
		shadowed[runtimeServiceName("postgres", instance)] = struct{}{}
	}
	for _, instance := range ValkeyInstanceNames(m) {
		shadowed[runtimeServiceName("valkey", instance)] = struct{}{}
		if instance == defaultServiceInstance {
			shadowed["redis"] = struct{}{}
		}
	}
	selected := make([]string, 0, len(available))
	for _, service := range available {
		if _, skip := shadowed[service]; skip {
			continue
		}
		selected = append(selected, service)
	}
	if len(selected) == 0 {
		return nil, errors.New("application workload contains no services after managed backend services were excluded")
	}
	sort.Strings(selected)
	return selected, nil
}

func workloadOverrideYAML(m Manifest, services []string, values map[string]string) (string, error) {
	return workloadOverrideYAMLForRuntime(m, services, values, "")
}

func workloadOverrideYAMLForRuntime(m Manifest, services []string, values map[string]string, runtimeProject string) (string, error) {
	return workloadOverrideYAMLForFiles(m, services, values, RuntimeFiles{Project: runtimeProject})
}

type workloadOverrideTopology struct {
	runtimeProject               string
	objectStorageNetworkName     string
	telemetryNetworkName         string
	identityNetworkName          string
	metricsNetworkName           string
	canonicalDevNetworkName      string
	env                          map[string]string
	runtimeObjectStorageServices map[string]struct{}
	metricsServices              map[string]struct{}
	exposedServices              map[string]struct{}
	managedRuntime               bool
	serviceBindings              bool
	applicationBackendNetwork    bool
	sharedBackendNetwork         bool
	backendNetwork               bool
	objectStorage                bool
	hasRuntimeObjectStorage      bool
	telemetryManaged             bool
	identityManaged              bool
	canonicalDevWorkload         bool
}

func workloadOverrideYAMLForFiles(m Manifest, services []string, values map[string]string, runtime RuntimeFiles) (string, error) {
	topology, err := resolveWorkloadOverrideTopology(m, services, values, runtime)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("services:\n")
	if err := writeWorkloadOverrideServices(&b, m, services, values, runtime, topology); err != nil {
		return "", err
	}
	writeWorkloadOverrideNetworks(&b, m, runtime, topology)
	return b.String(), nil
}

func resolveWorkloadOverrideTopology(m Manifest, services []string, values map[string]string, runtime RuntimeFiles) (workloadOverrideTopology, error) {
	topology := workloadOverrideTopology{
		runtimeProject:               runtime.ResourceProject,
		runtimeObjectStorageServices: map[string]struct{}{},
		metricsServices:              map[string]struct{}{},
		exposedServices:              make(map[string]struct{}, len(m.Exposures)),
	}
	if strings.TrimSpace(topology.runtimeProject) == "" {
		topology.runtimeProject = RuntimeProjectName(m)
	}
	namespace := strings.TrimSpace(strings.ReplaceAll(runtime.Namespace, ".", "-"))
	topology.objectStorageNetworkName = scopedWorkloadNetworkName("baseharbor-object-storage", namespace)
	topology.telemetryNetworkName = scopedWorkloadNetworkName("baseharbor-telemetry", namespace)

	env, err := containerRuntimeEnvironment(m, values)
	if err != nil {
		return workloadOverrideTopology{}, err
	}
	topology.env = env
	topology.managedRuntime = HasManagedRuntimeServices(m)
	topology.serviceBindings = topology.managedRuntime || HasIdentity(m)
	runtimeBroker := RequiresRuntimeBroker(m)
	topology.applicationBackendNetwork = HasApplicationScopedRuntimeServices(m) || runtimeBroker
	topology.sharedBackendNetwork = HasSharedBackends(m)
	topology.backendNetwork = topology.applicationBackendNetwork || topology.sharedBackendNetwork
	topology.objectStorage = HasObjectStorage(m)

	for _, permission := range m.Runtime.Permissions {
		if permission.Capability != string(capability.ObjectStorageS3V1.ID) {
			continue
		}
		for _, service := range permission.Services {
			topology.runtimeObjectStorageServices[service] = struct{}{}
		}
	}
	topology.hasRuntimeObjectStorage = len(topology.runtimeObjectStorageServices) > 0
	topology.telemetryManaged = HasOTLPTelemetry(m) && values["OTLP_PROVIDER"] == string(capability.ProviderOTelCollector)

	if HasIdentity(m) {
		provider, err := referenceCapabilityProvider(capability.Identity)
		if err != nil {
			return workloadOverrideTopology{}, err
		}
		topology.identityManaged = provider.Kind == capability.ProviderKeycloak
		if topology.identityManaged {
			topology.identityNetworkName, err = IdentityProviderNetworkName(m, namespace)
			if err != nil {
				return workloadOverrideTopology{}, err
			}
		}
	}

	topology.canonicalDevWorkload = strings.EqualFold(strings.TrimSpace(m.Environment), "dev") && len(services) == 1
	if topology.canonicalDevWorkload {
		topology.canonicalDevNetworkName = DevelopmentWorkloadNetworkNameForProject(topology.runtimeProject)
	}

	hasMetricsIntent := len(m.Metrics.Sources) > 0 || HasRuntimeMetricsPermissions(m)
	if hasMetricsIntent {
		metricsPolicy, err := MetricsPolicy(m)
		if err != nil {
			return workloadOverrideTopology{}, err
		}
		if metricsPolicy.Enabled && metricsPolicy.Collect[MetricsSourceApplication] {
			metricsPlacement, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
			if err != nil {
				return workloadOverrideTopology{}, err
			}
			if metricsPlacement.Scope != capability.ScopeExternal {
				for _, source := range m.Metrics.Sources {
					topology.metricsServices[source.Service] = struct{}{}
				}
				for _, permission := range m.Runtime.Permissions {
					if permission.Capability != string(capability.MetricsV1.ID) {
						continue
					}
					for _, service := range permission.Services {
						topology.metricsServices[service] = struct{}{}
					}
				}
				topology.metricsNetworkName = MetricsProviderNetworkNameForNamespace(m, namespace)
			}
		}
	}

	for _, exposure := range m.Exposures {
		topology.exposedServices[exposure.Service] = struct{}{}
	}
	return topology, nil
}

func writeWorkloadOverrideServices(b *strings.Builder, m Manifest, services []string, values map[string]string, runtime RuntimeFiles, topology workloadOverrideTopology) error {
	for _, service := range services {
		_, exposed := topology.exposedServices[service]
		_, metricsSource := topology.metricsServices[service]
		_, runtimeObjectStorage := topology.runtimeObjectStorageServices[service]
		serviceObjectStorage := topology.objectStorage || runtimeObjectStorage
		hasEnvironment := len(topology.env) > 0 || HasOTLPTelemetry(m) || HasIdentity(m)
		hasNetworks := topology.backendNetwork || serviceObjectStorage || topology.telemetryManaged || topology.identityManaged || metricsSource || exposed || topology.canonicalDevWorkload
		hasTelemetryTLS := HasOTLPTelemetry(m) && strings.TrimSpace(values[OTLPTLSHostCAEnv]) != ""
		hasObjectStorageTLS := serviceObjectStorage && strings.TrimSpace(values[S3TLSHostCAEnv]) != ""
		hasBackendTLS := topology.managedRuntime

		if !hasEnvironment && !hasNetworks {
			fmt.Fprintf(b, "  %s: {}\n", service)
			continue
		}

		fmt.Fprintf(b, "  %s:\n", service)
		if hasEnvironment {
			if err := writeWorkloadServiceEnvironment(b, m, service, values, topology); err != nil {
				return err
			}
		}
		if hasTelemetryTLS || hasObjectStorageTLS || hasBackendTLS || topology.serviceBindings {
			writeWorkloadServiceVolumes(b, m, values, runtime, topology.serviceBindings, hasTelemetryTLS, hasObjectStorageTLS, hasBackendTLS)
		}
		if hasNetworks {
			writeWorkloadServiceNetworks(b, m, service, topology, serviceObjectStorage, metricsSource, exposed)
		}
	}
	return nil
}

func writeWorkloadServiceEnvironment(b *strings.Builder, m Manifest, service string, values map[string]string, topology workloadOverrideTopology) error {
	b.WriteString("    environment:\n")
	serviceEnv := make(map[string]string, len(topology.env)+6)
	for key, value := range topology.env {
		serviceEnv[key] = value
	}
	if topology.serviceBindings {
		serviceEnv["SERVICE_BINDING_ROOT"] = workloadServiceBindingRoot
	}
	if HasIdentity(m) {
		issuer, err := requireRuntimeValue(values, "IDENTITY_CONTAINER_ISSUER")
		if err != nil {
			return err
		}
		clientID, err := requireRuntimeValue(values, "IDENTITY_CLIENT_ID")
		if err != nil {
			return err
		}
		serviceEnv["OIDC_ISSUER"] = issuer
		serviceEnv["OIDC_CLIENT_ID"] = clientID
		if len(m.Identity.Scopes) > 0 {
			serviceEnv["OIDC_SCOPES"] = strings.Join(m.Identity.Scopes, " ")
		}
		if strings.TrimSpace(values["IDENTITY_CLIENT_SECRET"]) != "" {
			serviceEnv["OIDC_CLIENT_SECRET_FILE"] = IdentityWorkloadClientSecretFile
		}
		if strings.TrimSpace(values["IDENTITY_CA_FILE"]) != "" {
			serviceEnv["OIDC_CA_FILE"] = "/run/baseharbor/service-bindings/identity/ca.crt"
		}
	}
	if HasOTLPTelemetry(m) {
		serviceEnv["OTEL_SERVICE_NAME"] = service
		serviceEnv["OTEL_RESOURCE_ATTRIBUTES"] = telemetryResourceAttributes(m, service, values["OTLP_PROVIDER"])
	}
	keys := make([]string, 0, len(serviceEnv))
	for key := range serviceEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(b, "      %s: %s\n", key, strconv.Quote(serviceEnv[key]))
	}
	return nil
}

func writeWorkloadServiceVolumes(b *strings.Builder, m Manifest, values map[string]string, runtime RuntimeFiles, serviceBindings, telemetryTLS, objectStorageTLS, backendTLS bool) {
	b.WriteString("    volumes:\n")
	if serviceBindings {
		projection := workloadServiceBindingProjectionDir(runtime)
		fmt.Fprintf(b, "      - %s\n", strconv.Quote(projection+":"+workloadServiceBindingRoot+":ro"))
	}
	if telemetryTLS {
		fmt.Fprintf(b, "      - %s\n", strconv.Quote(values[OTLPTLSHostCAEnv]+":"+OTLPTLSContainerCA+":ro"))
		if strings.TrimSpace(values[OTLPTLSHostClientCertEnv]) != "" {
			fmt.Fprintf(b, "      - %s\n", strconv.Quote(values[OTLPTLSHostClientCertEnv]+":"+OTLPTLSContainerClientCert+":ro"))
			fmt.Fprintf(b, "      - %s\n", strconv.Quote(values[OTLPTLSHostClientKeyEnv]+":"+OTLPTLSContainerClientKey+":ro"))
		}
	}
	if objectStorageTLS {
		fmt.Fprintf(b, "      - %s\n", strconv.Quote(values[S3TLSHostCAEnv]+":"+S3TLSContainerCA+":ro"))
	}
	if backendTLS {
		for _, instance := range SQLInstanceNames(m) {
			if ca := strings.TrimSpace(values[postgresTLSCAKey(instance)]); ca != "" {
				fmt.Fprintf(b, "      - %s\n", strconv.Quote(ca+":"+postgresTLSCAContainerPath(instance)+":ro"))
			}
		}
		for _, instance := range ValkeyInstanceNames(m) {
			if ca := strings.TrimSpace(values[valkeyTLSCAKey(instance)]); ca != "" {
				fmt.Fprintf(b, "      - %s\n", strconv.Quote(ca+":"+valkeyTLSCAContainerPath(instance)+":ro"))
			}
		}
	}
}

func writeWorkloadServiceNetworks(b *strings.Builder, m Manifest, service string, topology workloadOverrideTopology, serviceObjectStorage, metricsSource, exposed bool) {
	b.WriteString("    networks:\n")
	if !metricsSource && !exposed && !topology.canonicalDevWorkload {
		if topology.applicationBackendNetwork {
			b.WriteString("      - baseharbor-backend\n")
		}
		if topology.sharedBackendNetwork {
			b.WriteString("      - baseharbor-shared-backend\n")
		}
		if serviceObjectStorage {
			b.WriteString("      - baseharbor-object-storage\n")
		}
		if topology.telemetryManaged {
			b.WriteString("      - baseharbor-telemetry\n")
		}
		if topology.identityManaged {
			b.WriteString("      - baseharbor-identity\n")
		}
		return
	}
	if topology.applicationBackendNetwork {
		b.WriteString("      baseharbor-backend: {}\n")
	}
	if topology.sharedBackendNetwork {
		b.WriteString("      baseharbor-shared-backend: {}\n")
	}
	if serviceObjectStorage {
		b.WriteString("      baseharbor-object-storage: {}\n")
	}
	if topology.telemetryManaged {
		b.WriteString("      baseharbor-telemetry: {}\n")
	}
	if topology.identityManaged {
		b.WriteString("      baseharbor-identity: {}\n")
	}
	if metricsSource {
		b.WriteString("      baseharbor-metrics:\n")
		b.WriteString("        aliases:\n")
		fmt.Fprintf(b, "          - %s\n", strconv.Quote(MetricsTargetAlias(m, service)))
	}
	if exposed {
		b.WriteString("      baseharbor-exposure:\n")
		b.WriteString("        aliases:\n")
		fmt.Fprintf(b, "          - %s\n", strconv.Quote(service))
	}
	if topology.canonicalDevWorkload {
		b.WriteString("      baseharbor-dev-workload:\n")
		b.WriteString("        aliases:\n")
		fmt.Fprintf(b, "          - %s\n", strconv.Quote(DevelopmentWorkloadAlias(m)))
	}
}

func writeWorkloadOverrideNetworks(b *strings.Builder, m Manifest, runtime RuntimeFiles, topology workloadOverrideTopology) {
	if !topology.backendNetwork && !topology.objectStorage && !topology.hasRuntimeObjectStorage && !topology.telemetryManaged && !topology.identityManaged && len(topology.metricsServices) == 0 && len(topology.exposedServices) == 0 && !topology.canonicalDevWorkload {
		return
	}
	b.WriteString("networks:\n")
	if topology.applicationBackendNetwork {
		b.WriteString("  baseharbor-backend:\n    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", applicationBackendNetworkForRuntime(m, topology.runtimeProject))
	}
	if topology.sharedBackendNetwork {
		b.WriteString("  baseharbor-shared-backend:\n    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(SharedBackendNetworkName(runtime.Namespace, m.Environment)))
	}
	if topology.objectStorage || topology.hasRuntimeObjectStorage {
		b.WriteString("  baseharbor-object-storage:\n    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(topology.objectStorageNetworkName))
	}
	if topology.telemetryManaged {
		b.WriteString("  baseharbor-telemetry:\n    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(topology.telemetryNetworkName))
	}
	if topology.identityManaged {
		b.WriteString("  baseharbor-identity:\n    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(topology.identityNetworkName))
	}
	if len(topology.metricsServices) > 0 {
		b.WriteString("  baseharbor-metrics:\n    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(topology.metricsNetworkName))
	}
	if len(topology.exposedServices) > 0 {
		b.WriteString("  baseharbor-exposure:\n")
		fmt.Fprintf(b, "    name: %s\n", applicationExposureNetworkForRuntime(m, topology.runtimeProject))
	}
	if topology.canonicalDevWorkload {
		b.WriteString("  baseharbor-dev-workload:\n")
		fmt.Fprintf(b, "    name: %s\n", strconv.Quote(topology.canonicalDevNetworkName))
	}
}

func scopedWorkloadNetworkName(base, namespace string) string {
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	if namespace == "" {
		return base
	}
	return base + "-" + namespace
}

func applicationBackendNetworkForRuntime(m Manifest, runtimeProject string) string {
	if name := ApplicationBackendNetworkNameForProject(runtimeProject); name != "" {
		return name
	}
	return ApplicationBackendNetworkName(m)
}

func applicationExposureNetworkForRuntime(m Manifest, runtimeProject string) string {
	if name := ApplicationExposureNetworkNameForProject(runtimeProject); name != "" {
		return name
	}
	return ApplicationExposureNetworkName(m)
}

func ManagedWorkloadEnvironment(m Manifest, files RuntimeFiles) (map[string]string, error) {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return nil, err
	}
	return containerRuntimeEnvironment(m, values)
}

func containerRuntimeEnvironment(m Manifest, values map[string]string) (map[string]string, error) {
	env := map[string]string{}
	postgres := SQLInstanceNames(m)
	preferredPostgres := preferredServiceInstance(postgres)
	for _, instance := range postgres {
		uri, err := postgresContainerConnectionURL(values, instance)
		if err != nil {
			return nil, err
		}
		if instance == preferredPostgres {
			env["DATABASE_URL"] = uri
			env["DATABASE_CA_FILE"] = postgresTLSCAContainerPath(instance)
		}
		if instance != defaultServiceInstance {
			env["DATABASE_"+envInstanceToken(instance)+"_URL"] = uri
		}
	}
	s3Buckets := ObjectStorageBucketNames(m)
	preferredS3 := preferredServiceInstance(s3Buckets)
	for _, bucket := range s3Buckets {
		access, err := requireRuntimeValue(values, s3RuntimeKey(bucket, "ACCESS_KEY_ID"))
		if err != nil {
			return nil, err
		}
		secret, err := requireRuntimeValue(values, s3RuntimeKey(bucket, "SECRET_ACCESS_KEY"))
		if err != nil {
			return nil, err
		}
		physical, err := requireRuntimeValue(values, s3RuntimeKey(bucket, "BUCKET"))
		if err != nil {
			return nil, err
		}
		endpoint, err := requireRuntimeValue(values, s3RuntimeKey(bucket, "CONTAINER_ENDPOINT"))
		if err != nil {
			return nil, err
		}
		token := envInstanceToken(bucket)
		if bucket == preferredS3 {
			env["S3_ENDPOINT"] = endpoint
			env["S3_BUCKET"] = physical
			env["S3_REGION"] = "us-east-1"
			env["AWS_ENDPOINT_URL"] = endpoint
			if strings.TrimSpace(values[S3TLSHostCAEnv]) != "" {
				env["AWS_CA_BUNDLE"] = S3TLSContainerCA
			}
			env["AWS_REGION"] = "us-east-1"
			env["AWS_ACCESS_KEY_ID"] = access
			env["AWS_SECRET_ACCESS_KEY"] = secret
		}
		if bucket != defaultServiceInstance || len(s3Buckets) != 1 {
			env["S3_"+token+"_ENDPOINT"] = endpoint
			env["S3_"+token+"_BUCKET"] = physical
			env["S3_"+token+"_REGION"] = "us-east-1"
			if strings.TrimSpace(values[S3TLSHostCAEnv]) != "" {
				env["S3_"+token+"_CA_FILE"] = S3TLSContainerCA
			}
			env["S3_"+token+"_ACCESS_KEY_ID"] = access
			env["S3_"+token+"_SECRET_ACCESS_KEY"] = secret
		}
	}

	if HasOTLPTelemetry(m) {
		endpoint, err := requireRuntimeValue(values, "OTLP_CONTAINER_ENDPOINT")
		if err != nil {
			return nil, err
		}
		env["OTEL_EXPORTER_OTLP_ENDPOINT"] = endpoint
		env["OTEL_EXPORTER_OTLP_PROTOCOL"] = "http/protobuf"
		env["OTEL_RESOURCE_ATTRIBUTES"] = telemetryResourceAttributes(m, "", values["OTLP_PROVIDER"])
		if strings.TrimSpace(values[OTLPTLSHostCAEnv]) != "" {
			env["OTEL_EXPORTER_OTLP_CERTIFICATE"] = OTLPTLSContainerCA
			if strings.TrimSpace(values[OTLPTLSHostClientCertEnv]) != "" {
				env["OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE"] = OTLPTLSContainerClientCert
				env["OTEL_EXPORTER_OTLP_CLIENT_KEY"] = OTLPTLSContainerClientKey
			}
		}
		if values["OTLP_PROVIDER"] == string(capability.ProviderExternalOTLP) {
			if headers := strings.TrimSpace(os.Getenv("BASEHARBOR_OTLP_HEADERS")); headers != "" {
				env["OTEL_EXPORTER_OTLP_HEADERS"] = headers
			}
		}
	}

	redis := ValkeyInstanceNames(m)
	preferredRedis := preferredServiceInstance(redis)
	for _, instance := range redis {
		uri, err := valkeyContainerConnectionURL(values, instance)
		if err != nil {
			return nil, err
		}
		if instance == preferredRedis {
			env["REDIS_URL"] = uri
			env["VALKEY_URL"] = uri
			env["REDIS_CA_FILE"] = valkeyTLSCAContainerPath(instance)
			env["VALKEY_CA_FILE"] = valkeyTLSCAContainerPath(instance)
		}
		if instance != defaultServiceInstance {
			token := envInstanceToken(instance)
			env["REDIS_"+token+"_URL"] = uri
			env["VALKEY_"+token+"_URL"] = uri
		}
	}
	return env, nil
}

func postgresContainerConnectionURL(values map[string]string, instance string) (string, error) {
	database, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "DB"))
	if err != nil {
		return "", err
	}
	username, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "USER"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("sslmode", "verify-ca")
	query.Set("sslrootcert", postgresTLSCAContainerPath(instance))
	host := strings.TrimSpace(values[postgresContainerHostKey(instance)])
	if host == "" {
		host = postgresAccessService(instance)
	}
	u := &url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(host, "5432"),
		Path:     "/" + database,
		RawQuery: query.Encode(),
	}
	return u.String(), nil
}

func valkeyContainerConnectionURL(values map[string]string, instance string) (string, error) {
	password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(values[valkeyContainerHostKey(instance)])
	if host == "" {
		host = valkeyAccessService(instance)
	}
	u := &url.URL{
		Scheme: "rediss",
		User:   url.UserPassword("default", password),
		Host:   net.JoinHostPort(host, "6379"),
		Path:   "/0",
	}
	return u.String(), nil
}
