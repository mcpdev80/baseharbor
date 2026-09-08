package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrUnsupportedService = errors.New("application contains services that are not yet supported by apply")

type RuntimeFiles struct {
	Dir            string
	Compose        string
	Env            string
	ApplicationEnv string
	Bindings       string
}

func RuntimeProjectName(m Manifest) string {
	return "baseharbor-" + m.Name + "-" + m.Environment
}

func CheckSupportedRuntimeServices(m Manifest) error {
	if len(PostgresInstanceNames(m)) == 0 && len(RedisInstanceNames(m)) == 0 {
		if m.Services.Secrets {
			return fmt.Errorf("%w: managed secrets currently require PostgreSQL or Valkey so the application has a materialized runtime", ErrUnsupportedService)
		}
		return fmt.Errorf("%w: no supported runtime service is enabled", ErrUnsupportedService)
	}
	return nil
}

func EnsureRuntime(store Store, m Manifest) (RuntimeFiles, error) {
	if err := m.Validate(); err != nil {
		return RuntimeFiles{}, err
	}
	if err := CheckSupportedRuntimeServices(m); err != nil {
		return RuntimeFiles{}, err
	}

	dir := filepath.Join(store.Root, m.Name, "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return RuntimeFiles{}, fmt.Errorf("create application runtime directory: %w", err)
	}
	files := RuntimeFiles{
		Dir:            dir,
		Compose:        filepath.Join(dir, "compose.yaml"),
		Env:            filepath.Join(dir, "runtime.env"),
		ApplicationEnv: filepath.Join(dir, "application.env"),
		Bindings:       filepath.Join(dir, "bindings"),
	}
	if err := ensureRuntimeEnv(files.Env, m); err != nil {
		return RuntimeFiles{}, err
	}

	compose, err := RuntimeComposeYAML(m)
	if err != nil {
		return RuntimeFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		return RuntimeFiles{}, fmt.Errorf("write application compose file: %w", err)
	}
	contract, err := EnsureRuntimeContract(m, files)
	if err != nil {
		return RuntimeFiles{}, err
	}
	files.ApplicationEnv = contract.Env
	files.Bindings = contract.BindingsDir
	return files, nil
}

// EnsurePostgresRuntime is kept for callers from the first runtime milestone.
func EnsurePostgresRuntime(store Store, m Manifest) (RuntimeFiles, error) {
	return EnsureRuntime(store, m)
}

func VerifyPostgresRuntime(ctx context.Context, compose bhruntime.Compose, m Manifest, files RuntimeFiles) error {
	for _, instance := range PostgresInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		out, err := compose.ExecProject(ctx, RuntimeProjectName(m), files.Compose, files.Env, service, "psql", "-U", "baseharbor", "-d", postgresDatabaseName(m, instance), "-tAc", "SELECT 1")
		if err != nil {
			return fmt.Errorf("verify postgres instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "1" {
			return fmt.Errorf("verify postgres instance %s: unexpected query result %q", instance, strings.TrimSpace(out))
		}
	}
	return nil
}

func VerifyValkeyRuntime(ctx context.Context, compose bhruntime.Compose, m Manifest, files RuntimeFiles) error {
	for _, instance := range RedisInstanceNames(m) {
		service := runtimeServiceName("valkey", instance)
		out, err := compose.ExecProject(ctx, RuntimeProjectName(m), files.Compose, files.Env, service, "sh", "-ec", `VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli ping`)
		if err != nil {
			return fmt.Errorf("verify valkey instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "PONG" {
			return fmt.Errorf("verify valkey instance %s: unexpected PING result %q", instance, strings.TrimSpace(out))
		}
	}
	return nil
}

func RuntimeComposeYAML(m Manifest) (string, error) {
	if err := CheckSupportedRuntimeServices(m); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("services:\n")
	for _, instance := range PostgresInstanceNames(m) {
		writePostgresComposeService(&b, instance)
	}
	for _, instance := range RedisInstanceNames(m) {
		writeValkeyComposeService(&b, instance)
	}
	b.WriteString("\nvolumes:\n")
	for _, instance := range PostgresInstanceNames(m) {
		fmt.Fprintf(&b, "  %s-data:\n", runtimeServiceName("postgres", instance))
	}
	for _, instance := range RedisInstanceNames(m) {
		fmt.Fprintf(&b, "  %s-data:\n", runtimeServiceName("valkey", instance))
	}
	return b.String(), nil
}

func writePostgresComposeService(b *strings.Builder, instance string) {
	service := runtimeServiceName("postgres", instance)
	dbKey := postgresRuntimeKey(instance, "DB")
	userKey := postgresRuntimeKey(instance, "USER")
	passwordKey := postgresRuntimeKey(instance, "PASSWORD")
	portKey := postgresRuntimeKey(instance, "HOST_PORT")
	fmt.Fprintf(b, `  %s:
    image: postgres:18-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${%s}
      POSTGRES_USER: ${%s}
      POSTGRES_PASSWORD: ${%s}
    ports:
      - "127.0.0.1:${%s}:5432"
    volumes:
      - %s-data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${%s} -d ${%s}"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s

`, service, dbKey, userKey, passwordKey, portKey, service, userKey, dbKey)
}

func writeValkeyComposeService(b *strings.Builder, instance string) {
	service := runtimeServiceName("valkey", instance)
	passwordKey := valkeyRuntimeKey(instance, "PASSWORD")
	portKey := valkeyRuntimeKey(instance, "HOST_PORT")
	fmt.Fprintf(b, `  %s:
    image: valkey/valkey:9.1.2-alpine
    restart: unless-stopped
    environment:
      VALKEY_PASSWORD: ${%s}
    command:
      - sh
      - -ec
      - |
        printf 'requirepass %%s\nappendonly yes\ndir /data\n' "$$VALKEY_PASSWORD" > /tmp/valkey.conf
        exec valkey-server /tmp/valkey.conf
    ports:
      - "127.0.0.1:${%s}:6379"
    volumes:
      - %s-data:/data
    healthcheck:
      test: ["CMD-SHELL", "VALKEYCLI_AUTH=\"$${VALKEY_PASSWORD}\" valkey-cli ping | grep -q '^PONG$'"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s

`, service, passwordKey, portKey, service)
}

func ensureRuntimeEnv(path string, m Manifest) error {
	values := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		var readErr error
		values, readErr = readRuntimeEnvBytes(data)
		if readErr != nil {
			return readErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect application runtime environment: %w", err)
	}

	if err := ensureDesiredRuntimeValues(values, m); err != nil {
		return err
	}
	if err := validateRuntimeValues(values, m); err != nil {
		return err
	}
	return writeRuntimeEnv(path, m, values)
}

func newRuntimeEnv(m Manifest) (string, error) {
	values := map[string]string{}
	if err := ensureDesiredRuntimeValues(values, m); err != nil {
		return "", err
	}
	return runtimeEnvContent(m, values), nil
}

func ensureDesiredRuntimeValues(values map[string]string, m Manifest) error {
	excluded := map[int]struct{}{}
	for key, value := range values {
		if !strings.HasSuffix(key, "_HOST_PORT") || strings.TrimSpace(value) == "" {
			continue
		}
		if err := validatePortValue(value, key); err != nil {
			return err
		}
		port, _ := strconv.Atoi(value)
		excluded[port] = struct{}{}
	}

	for _, instance := range PostgresInstanceNames(m) {
		dbKey := postgresRuntimeKey(instance, "DB")
		userKey := postgresRuntimeKey(instance, "USER")
		passwordKey := postgresRuntimeKey(instance, "PASSWORD")
		portKey := postgresRuntimeKey(instance, "HOST_PORT")
		if values[dbKey] == "" {
			values[dbKey] = postgresDatabaseName(m, instance)
		}
		if values[userKey] == "" {
			values[userKey] = "baseharbor"
		}
		if values[passwordKey] == "" {
			password, err := randomApplicationSecret(32)
			if err != nil {
				return err
			}
			values[passwordKey] = password
		}
		if values[portKey] == "" {
			port, err := allocateLoopbackPort(excluded)
			if err != nil {
				return err
			}
			values[portKey] = strconv.Itoa(port)
			excluded[port] = struct{}{}
		}
	}
	for _, instance := range RedisInstanceNames(m) {
		passwordKey := valkeyRuntimeKey(instance, "PASSWORD")
		portKey := valkeyRuntimeKey(instance, "HOST_PORT")
		if values[passwordKey] == "" {
			password, err := randomApplicationSecret(32)
			if err != nil {
				return err
			}
			values[passwordKey] = password
		}
		if values[portKey] == "" {
			port, err := allocateLoopbackPort(excluded)
			if err != nil {
				return err
			}
			values[portKey] = strconv.Itoa(port)
			excluded[port] = struct{}{}
		}
	}
	return nil
}

func writeRuntimeEnv(path string, m Manifest, values map[string]string) error {
	if err := writeOwnerOnlyFile(path, []byte(runtimeEnvContent(m, values))); err != nil {
		return fmt.Errorf("write application runtime environment: %w", err)
	}
	return nil
}

func runtimeEnvContent(m Manifest, values map[string]string) string {
	var b strings.Builder
	for _, instance := range PostgresInstanceNames(m) {
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "HOST_PORT"} {
			key := postgresRuntimeKey(instance, suffix)
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
	}
	for _, instance := range RedisInstanceNames(m) {
		for _, suffix := range []string{"PASSWORD", "HOST_PORT"} {
			key := valkeyRuntimeKey(instance, suffix)
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
	}
	return b.String()
}

func validateRuntimeEnv(path string, m Manifest) error {
	values, err := readRuntimeEnv(path)
	if err != nil {
		return err
	}
	return validateRuntimeValues(values, m)
}

func validateRuntimeValues(values map[string]string, m Manifest) error {
	for _, instance := range PostgresInstanceNames(m) {
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "HOST_PORT"} {
			key := postgresRuntimeKey(instance, suffix)
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		portKey := postgresRuntimeKey(instance, "HOST_PORT")
		if err := validatePortValue(values[portKey], portKey); err != nil {
			return err
		}
	}
	for _, instance := range RedisInstanceNames(m) {
		for _, suffix := range []string{"PASSWORD", "HOST_PORT"} {
			key := valkeyRuntimeKey(instance, suffix)
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		portKey := valkeyRuntimeKey(instance, "HOST_PORT")
		if err := validatePortValue(values[portKey], portKey); err != nil {
			return err
		}
	}
	return nil
}

func postgresDatabaseName(m Manifest, instance string) string {
	base := strings.ReplaceAll(m.Name+"_"+m.Environment, "-", "_")
	if instance == defaultServiceInstance {
		if len(base) > 63 {
			base = base[:63]
		}
		return base
	}
	suffix := "_" + strings.ReplaceAll(instance, "-", "_")
	maxBase := 63 - len(suffix)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

func runtimeServiceName(kind, instance string) string {
	if instance == defaultServiceInstance {
		return kind
	}
	return kind + "-" + instance
}

func postgresRuntimeKey(instance, suffix string) string {
	return runtimeInstanceKey("POSTGRES", instance, suffix)
}

func valkeyRuntimeKey(instance, suffix string) string {
	return runtimeInstanceKey("VALKEY", instance, suffix)
}

func runtimeInstanceKey(prefix, instance, suffix string) string {
	if instance == defaultServiceInstance {
		return prefix + "_" + suffix
	}
	name := strings.ToUpper(strings.ReplaceAll(instance, "-", "_"))
	return prefix + "_" + name + "_" + suffix
}

func randomApplicationSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate application runtime secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
