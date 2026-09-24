package application

import (
	"bufio"
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
	return "baseharbor-workload-" + m.Name + "-" + m.Environment
}

func ResolveWorkloadCompose(repositoryRoot string, m Manifest) (string, bool, error) {
	if strings.TrimSpace(repositoryRoot) == "" {
		return "", false, nil
	}
	if m.Workload.Compose != "" {
		path := filepath.Join(repositoryRoot, m.Workload.Compose)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", false, fmt.Errorf("%w: %s", ErrWorkloadComposeNotFound, m.Workload.Compose)
			}
			return "", false, err
		}
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("workload compose path %s is not a regular file", m.Workload.Compose)
		}
		return path, true, nil
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
		return "", false, fmt.Errorf("%w: %s; set workload.compose in baseharbor.yaml", ErrWorkloadComposeAmbiguous, strings.Join(rel, ", "))
	}
	return found[0], true, nil
}

func MaterializeWorkload(repositoryRoot string, m Manifest, runtime RuntimeFiles) (WorkloadFiles, bool, error) {
	composePath, found, err := ResolveWorkloadCompose(repositoryRoot, m)
	if err != nil || !found {
		return WorkloadFiles{}, found, err
	}
	services, err := composeServiceNames(composePath)
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	selected, err := selectWorkloadServices(m, services, m.Workload.Services)
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	values, err := readRuntimeEnv(runtime.Env)
	if err != nil {
		return WorkloadFiles{}, false, err
	}
	override, err := workloadOverrideYAML(m, selected, values)
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
		Project:        WorkloadProjectName(m),
		Partial:        len(selected) != len(services),
	}, true, nil
}

func SelectedWorkloadServices(repositoryRoot string, m Manifest) ([]string, string, bool, error) {
	composePath, found, err := ResolveWorkloadCompose(repositoryRoot, m)
	if err != nil || !found {
		return nil, composePath, found, err
	}
	services, err := composeServiceNames(composePath)
	if err != nil {
		return nil, composePath, true, err
	}
	selected, err := selectWorkloadServices(m, services, m.Workload.Services)
	if err != nil {
		return nil, composePath, true, err
	}
	return selected, composePath, true, nil
}

func composeServiceNames(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open application Compose file: %w", err)
	}
	defer file.Close()

	inServices := false
	var services []string
	s := bufio.NewScanner(file)
	for s.Scan() {
		raw := strings.TrimRight(s.Text(), " \t\r")
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			if trim == "services:" {
				inServices = true
				continue
			}
			if inServices {
				break
			}
			continue
		}
		if inServices && indent == 2 && strings.HasSuffix(trim, ":") {
			name := strings.TrimSpace(strings.TrimSuffix(trim, ":"))
			if err := validateComposeServiceName(name); err != nil {
				return nil, fmt.Errorf("invalid application Compose service: %w", err)
			}
			services = append(services, name)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("application Compose file %s contains no services", path)
	}
	return services, nil
}

func selectWorkloadServices(m Manifest, available, requested []string) ([]string, error) {
	availableSet := make(map[string]struct{}, len(available))
	for _, service := range available {
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
	if len(SQLInstanceNames(m)) > 0 {
		shadowed["postgres"] = struct{}{}
	}
	if len(CacheInstanceNames(m)) > 0 {
		shadowed["redis"] = struct{}{}
		shadowed["valkey"] = struct{}{}
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
	env, err := containerRuntimeEnvironment(m, values)
	if err != nil {
		return "", err
	}
	managedRuntime := HasManagedRuntimeServices(m)
	runtimeBroker := RequiresRuntimeBroker(m)
	backendNetwork := managedRuntime || runtimeBroker
	objectStorage := HasObjectStorage(m)
	runtimeObjectStorageServices := map[string]struct{}{}
	for _, permission := range m.Runtime.Permissions {
		if permission.Capability != string(capability.ObjectStorageS3V1.ID) {
			continue
		}
		for _, service := range permission.Services {
			runtimeObjectStorageServices[service] = struct{}{}
		}
	}
	hasRuntimeObjectStorage := len(runtimeObjectStorageServices) > 0
	telemetryManaged := HasOTLPTelemetry(m) && values["OTLP_PROVIDER"] == string(capability.ProviderOTelCollector)
	metricsServices := map[string]struct{}{}
	metricsNetworkName := ""
	hasMetricsIntent := len(m.Metrics.Sources) > 0 || HasRuntimeMetricsPermissions(m)
	if hasMetricsIntent {
		metricsPolicy, err := MetricsPolicy(m)
		if err != nil {
			return "", err
		}
		if metricsPolicy.Enabled && metricsPolicy.Collect[MetricsSourceApplication] {
			metricsPlacement, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
			if err != nil {
				return "", err
			}
			if metricsPlacement.Scope != capability.ScopeExternal {
				for _, source := range m.Metrics.Sources {
					metricsServices[source.Service] = struct{}{}
				}
				for _, permission := range m.Runtime.Permissions {
					if permission.Capability != string(capability.MetricsV1.ID) {
						continue
					}
					for _, service := range permission.Services {
						metricsServices[service] = struct{}{}
					}
				}
				metricsNetworkName = MetricsProviderNetworkName(m)
			}
		}
	}
	exposedServices := make(map[string]struct{}, len(m.Exposures))
	for _, exposure := range m.Exposures {
		exposedServices[exposure.Service] = struct{}{}
	}
	var b strings.Builder
	b.WriteString("services:\n")
	for _, service := range services {
		_, exposed := exposedServices[service]
		_, metricsSource := metricsServices[service]
		_, runtimeObjectStorage := runtimeObjectStorageServices[service]
		serviceObjectStorage := objectStorage || runtimeObjectStorage
		hasEnvironment := len(env) > 0 || HasOTLPTelemetry(m)
		hasNetworks := backendNetwork || serviceObjectStorage || telemetryManaged || metricsSource || exposed

		if !hasEnvironment && !hasNetworks {
			fmt.Fprintf(&b, "  %s: {}\n", service)
			continue
		}

		fmt.Fprintf(&b, "  %s:\n", service)
		if hasEnvironment {
			b.WriteString("    environment:\n")
			serviceEnv := make(map[string]string, len(env)+2)
			for key, value := range env {
				serviceEnv[key] = value
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
				fmt.Fprintf(&b, "      %s: %s\n", key, strconv.Quote(serviceEnv[key]))
			}
		}
		if hasNetworks {
			b.WriteString("    networks:\n")
			if !metricsSource && !exposed {
				if backendNetwork {
					b.WriteString("      - baseharbor-backend\n")
				}
				if serviceObjectStorage {
					b.WriteString("      - baseharbor-object-storage\n")
				}
				if telemetryManaged {
					b.WriteString("      - baseharbor-telemetry\n")
				}
				continue
			}
			if backendNetwork {
				b.WriteString("      baseharbor-backend: {}\n")
			}
			if serviceObjectStorage {
				b.WriteString("      baseharbor-object-storage: {}\n")
			}
			if telemetryManaged {
				b.WriteString("      baseharbor-telemetry: {}\n")
			}
			if metricsSource {
				b.WriteString("      baseharbor-metrics:\n")
				b.WriteString("        aliases:\n")
				fmt.Fprintf(&b, "          - %s\n", strconv.Quote(MetricsTargetAlias(m, service)))
			}
			if exposed {
				b.WriteString("      baseharbor-exposure:\n")
				b.WriteString("        aliases:\n")
				fmt.Fprintf(&b, "          - %s\n", strconv.Quote(service))
			}
		}
	}
	if backendNetwork || objectStorage || hasRuntimeObjectStorage || telemetryManaged || len(metricsServices) > 0 || len(exposedServices) > 0 {
		b.WriteString("networks:\n")
		if backendNetwork {
			b.WriteString("  baseharbor-backend:\n    external: true\n")
			fmt.Fprintf(&b, "    name: %s\n", ApplicationBackendNetworkName(m))
		}
		if objectStorage || hasRuntimeObjectStorage {
			b.WriteString("  baseharbor-object-storage:\n    external: true\n")
			b.WriteString("    name: baseharbor-object-storage\n")
		}
		if telemetryManaged {
			b.WriteString("  baseharbor-telemetry:\n    external: true\n")
			b.WriteString("    name: baseharbor-telemetry\n")
		}
		if len(metricsServices) > 0 {
			b.WriteString("  baseharbor-metrics:\n    external: true\n")
			fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(metricsNetworkName))
		}
		if len(exposedServices) > 0 {
			b.WriteString("  baseharbor-exposure:\n")
			fmt.Fprintf(&b, "    name: %s\n", ApplicationExposureNetworkName(m))
		}
	}
	return b.String(), nil
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
			env["AWS_REGION"] = "us-east-1"
			env["AWS_ACCESS_KEY_ID"] = access
			env["AWS_SECRET_ACCESS_KEY"] = secret
		}
		if bucket != defaultServiceInstance || len(s3Buckets) != 1 {
			env["S3_"+token+"_ENDPOINT"] = endpoint
			env["S3_"+token+"_BUCKET"] = physical
			env["S3_"+token+"_REGION"] = "us-east-1"
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
		if values["OTLP_PROVIDER"] == string(capability.ProviderExternalOTLP) {
			if headers := strings.TrimSpace(os.Getenv("BASEHARBOR_OTLP_HEADERS")); headers != "" {
				env["OTEL_EXPORTER_OTLP_HEADERS"] = headers
			}
		}
	}

	redis := CacheInstanceNames(m)
	preferredRedis := preferredServiceInstance(redis)
	for _, instance := range redis {
		uri, err := valkeyContainerConnectionURL(values, instance)
		if err != nil {
			return nil, err
		}
		if instance == preferredRedis {
			env["REDIS_URL"] = uri
			env["VALKEY_URL"] = uri
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
	u := &url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(runtimeServiceName("postgres", instance), "5432"),
		Path:   "/" + database,
	}
	return u.String(), nil
}

func valkeyContainerConnectionURL(values map[string]string, instance string) (string, error) {
	password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	u := &url.URL{
		Scheme: "redis",
		User:   url.UserPassword("default", password),
		Host:   net.JoinHostPort(runtimeServiceName("valkey", instance), "6379"),
		Path:   "/0",
	}
	return u.String(), nil
}
