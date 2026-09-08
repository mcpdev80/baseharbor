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
	if len(PostgresInstanceNames(m)) > 0 {
		shadowed["postgres"] = struct{}{}
	}
	if len(RedisInstanceNames(m)) > 0 {
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
	var b strings.Builder
	b.WriteString("services:\n")
	for _, service := range services {
		fmt.Fprintf(&b, "  %s:\n", service)
		if len(env) > 0 {
			b.WriteString("    environment:\n")
			keys := make([]string, 0, len(env))
			for key := range env {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(&b, "      %s: %s\n", key, strconv.Quote(env[key]))
			}
		}
		b.WriteString("    networks:\n      baseharbor-backend: {}\n")
	}
	b.WriteString("networks:\n  baseharbor-backend:\n    external: true\n")
	fmt.Fprintf(&b, "    name: %s\n", ApplicationBackendNetworkName(m))
	return b.String(), nil
}

func containerRuntimeEnvironment(m Manifest, values map[string]string) (map[string]string, error) {
	env := map[string]string{}
	postgres := PostgresInstanceNames(m)
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
	redis := RedisInstanceNames(m)
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
		Scheme:   "postgresql",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(runtimeServiceName("postgres", instance), "5432"),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
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
