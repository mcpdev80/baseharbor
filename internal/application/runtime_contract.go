package application

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const loopbackHost = "127.0.0.1"

type RuntimeContract struct {
	Env         string
	BindingsDir string
	Metadata    string
}

type runtimeMetadata struct {
	Version     int                          `json:"version"`
	Application string                       `json:"application"`
	Environment string                       `json:"environment"`
	Services    map[string]runtimeServiceRef `json:"services"`
}

type runtimeServiceRef struct {
	Binding string `json:"binding"`
}

func EnsureRuntimeContract(m Manifest, files RuntimeFiles) (RuntimeContract, error) {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return RuntimeContract{}, err
	}

	bindingsDir := filepath.Join(files.Dir, "bindings")
	bindingsAbs, err := filepath.Abs(bindingsDir)
	if err != nil {
		return RuntimeContract{}, fmt.Errorf("resolve application bindings directory: %w", err)
	}
	if err := os.MkdirAll(bindingsDir, 0o700); err != nil {
		return RuntimeContract{}, fmt.Errorf("create application bindings directory: %w", err)
	}
	if err := os.Chmod(bindingsDir, 0o700); err != nil {
		return RuntimeContract{}, fmt.Errorf("secure application bindings directory: %w", err)
	}

	serviceRefs := map[string]runtimeServiceRef{}
	var env strings.Builder
	fmt.Fprintf(&env, "BASEHARBOR_APP_NAME=%s\n", m.Name)
	fmt.Fprintf(&env, "BASEHARBOR_ENVIRONMENT=%s\n", m.Environment)
	fmt.Fprintf(&env, "BASEHARBOR_BINDINGS=%s\n", bindingsAbs)

	postgresInstances := PostgresInstanceNames(m)
	preferredPostgres := preferredServiceInstance(postgresInstances)
	for _, instance := range postgresInstances {
		binding, bindingRef, err := ensureInstanceBindingDirs(bindingsDir, bindingsAbs, "postgres", instance, len(postgresInstances))
		if err != nil {
			return RuntimeContract{}, err
		}
		uri, err := postgresConnectionURL(values, instance)
		if err != nil {
			return RuntimeContract{}, err
		}
		entries := map[string]string{
			"host":     loopbackHost,
			"port":     values[postgresRuntimeKey(instance, "HOST_PORT")],
			"database": values[postgresRuntimeKey(instance, "DB")],
			"username": values[postgresRuntimeKey(instance, "USER")],
			"password": values[postgresRuntimeKey(instance, "PASSWORD")],
			"uri":      uri,
		}
		if err := writeBinding(binding, entries); err != nil {
			return RuntimeContract{}, err
		}
		if instance == preferredPostgres {
			fmt.Fprintf(&env, "DATABASE_URL=%s\n", uri)
		}
		if instance != defaultServiceInstance {
			fmt.Fprintf(&env, "DATABASE_%s_URL=%s\n", envInstanceToken(instance), uri)
		}
		serviceRefs[serviceReferenceKey("postgres", instance, len(postgresInstances))] = runtimeServiceRef{Binding: bindingRef}
	}

	redisInstances := RedisInstanceNames(m)
	preferredRedis := preferredServiceInstance(redisInstances)
	for _, instance := range redisInstances {
		binding, bindingRef, err := ensureInstanceBindingDirs(bindingsDir, bindingsAbs, "valkey", instance, len(redisInstances))
		if err != nil {
			return RuntimeContract{}, err
		}
		uri, err := valkeyConnectionURL(values, instance)
		if err != nil {
			return RuntimeContract{}, err
		}
		entries := map[string]string{
			"host":     loopbackHost,
			"port":     values[valkeyRuntimeKey(instance, "HOST_PORT")],
			"password": values[valkeyRuntimeKey(instance, "PASSWORD")],
			"uri":      uri,
		}
		if err := writeBinding(binding, entries); err != nil {
			return RuntimeContract{}, err
		}
		if instance == preferredRedis {
			fmt.Fprintf(&env, "REDIS_URL=%s\n", uri)
			fmt.Fprintf(&env, "VALKEY_URL=%s\n", uri)
		}
		if instance != defaultServiceInstance {
			token := envInstanceToken(instance)
			fmt.Fprintf(&env, "REDIS_%s_URL=%s\n", token, uri)
			fmt.Fprintf(&env, "VALKEY_%s_URL=%s\n", token, uri)
		}
		serviceRefs[serviceReferenceKey("valkey", instance, len(redisInstances))] = runtimeServiceRef{Binding: bindingRef}
	}

	applicationEnv := filepath.Join(files.Dir, "application.env")
	if err := writeOwnerOnlyFile(applicationEnv, []byte(env.String())); err != nil {
		return RuntimeContract{}, fmt.Errorf("write application environment contract: %w", err)
	}

	metadataPath := filepath.Join(bindingsDir, "metadata.json")
	metadata, err := json.MarshalIndent(runtimeMetadata{
		Version:     1,
		Application: m.Name,
		Environment: m.Environment,
		Services:    serviceRefs,
	}, "", "  ")
	if err != nil {
		return RuntimeContract{}, fmt.Errorf("encode application binding metadata: %w", err)
	}
	metadata = append(metadata, '\n')
	if err := writeOwnerOnlyFile(metadataPath, metadata); err != nil {
		return RuntimeContract{}, fmt.Errorf("write application binding metadata: %w", err)
	}

	return RuntimeContract{Env: applicationEnv, BindingsDir: bindingsDir, Metadata: metadataPath}, nil
}

func ensureInstanceBindingDirs(bindingsDir, bindingsAbs, kind, instance string, count int) (string, string, error) {
	kindDir := filepath.Join(bindingsDir, kind)
	kindRef := filepath.Join(bindingsAbs, kind)
	binding := kindDir
	bindingRef := kindRef
	if count != 1 || instance != defaultServiceInstance {
		binding = filepath.Join(kindDir, instance)
		bindingRef = filepath.Join(kindRef, instance)
	}
	if err := os.MkdirAll(binding, 0o700); err != nil {
		return "", "", fmt.Errorf("create %s binding for %s: %w", kind, instance, err)
	}
	for _, path := range []string{kindDir, binding} {
		if err := os.Chmod(path, 0o700); err != nil {
			return "", "", fmt.Errorf("secure %s binding for %s: %w", kind, instance, err)
		}
	}
	return binding, bindingRef, nil
}

func serviceReferenceKey(kind, instance string, count int) string {
	if count == 1 && instance == defaultServiceInstance {
		return kind
	}
	return kind + "." + instance
}

func preferredServiceInstance(instances []string) string {
	if len(instances) == 1 {
		return instances[0]
	}
	for _, preferred := range []string{defaultServiceInstance, "primary"} {
		for _, instance := range instances {
			if instance == preferred {
				return instance
			}
		}
	}
	return ""
}

func envInstanceToken(instance string) string {
	return strings.ToUpper(strings.ReplaceAll(instance, "-", "_"))
}

func postgresConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
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
		Host:     net.JoinHostPort(loopbackHost, port),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}
	return u.String(), nil
}

func valkeyConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	u := &url.URL{
		Scheme: "redis",
		User:   url.UserPassword("", password),
		Host:   net.JoinHostPort(loopbackHost, port),
		Path:   "/0",
	}
	return u.String(), nil
}

func writeBinding(dir string, values map[string]string) error {
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("application binding %s has an empty value", name)
		}
		if err := writeOwnerOnlyFile(filepath.Join(dir, name), []byte(value+"\n")); err != nil {
			return fmt.Errorf("write application binding %s: %w", name, err)
		}
	}
	return nil
}

func writeOwnerOnlyFile(path string, content []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func readRuntimeEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application runtime environment: %w", err)
	}
	return readRuntimeEnvBytes(data)
}

func readRuntimeEnvBytes(data []byte) (map[string]string, error) {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("application runtime environment contains an invalid entry")
		}
		values[key] = value
	}
	return values, nil
}

func requireRuntimeValue(values map[string]string, key string) (string, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return "", fmt.Errorf("application runtime environment is missing %s", key)
	}
	return value, nil
}

func allocateLoopbackPort(exclude map[int]struct{}) (int, error) {
	for attempt := 0; attempt < 16; attempt++ {
		listener, err := net.Listen("tcp", loopbackHost+":0")
		if err != nil {
			return 0, fmt.Errorf("allocate local application service port: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			return 0, fmt.Errorf("release local application service port probe: %w", err)
		}
		if _, exists := exclude[port]; exists {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("allocate distinct local application service port")
}

func validatePortValue(value, key string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("application runtime environment contains invalid %s", key)
	}
	return nil
}
