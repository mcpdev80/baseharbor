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
	if !m.Services.Postgres && !m.Services.Redis {
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
	out, err := compose.ExecProject(ctx, RuntimeProjectName(m), files.Compose, files.Env, "postgres", "psql", "-U", "baseharbor", "-d", postgresDatabaseName(m), "-tAc", "SELECT 1")
	if err != nil {
		return fmt.Errorf("verify postgres: %w", err)
	}
	if strings.TrimSpace(out) != "1" {
		return fmt.Errorf("verify postgres: unexpected query result %q", strings.TrimSpace(out))
	}
	return nil
}

func VerifyValkeyRuntime(ctx context.Context, compose bhruntime.Compose, m Manifest, files RuntimeFiles) error {
	out, err := compose.ExecProject(ctx, RuntimeProjectName(m), files.Compose, files.Env, "valkey", "sh", "-ec", `VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli ping`)
	if err != nil {
		return fmt.Errorf("verify valkey: %w", err)
	}
	if strings.TrimSpace(out) != "PONG" {
		return fmt.Errorf("verify valkey: unexpected PING result %q", strings.TrimSpace(out))
	}
	return nil
}

func RuntimeComposeYAML(m Manifest) (string, error) {
	if err := CheckSupportedRuntimeServices(m); err != nil {
		return "", err
	}
	if m.Services.Postgres && !m.Services.Redis {
		return postgresComposeYAML, nil
	}
	if !m.Services.Postgres && m.Services.Redis {
		return valkeyComposeYAML, nil
	}
	return postgresValkeyComposeYAML, nil
}

func ensureRuntimeEnv(path string, m Manifest) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		content, err := newRuntimeEnv(m)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write application runtime environment: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect application runtime environment: %w", err)
	}

	values, err := readRuntimeEnv(path)
	if err != nil {
		return err
	}
	changed := false
	excluded := map[int]struct{}{}
	for _, key := range []string{"POSTGRES_HOST_PORT", "VALKEY_HOST_PORT"} {
		if value := values[key]; value != "" {
			if err := validatePortValue(value, key); err != nil {
				return err
			}
			port, _ := strconv.Atoi(value)
			excluded[port] = struct{}{}
		}
	}
	if m.Services.Postgres && values["POSTGRES_HOST_PORT"] == "" {
		port, err := allocateLoopbackPort(excluded)
		if err != nil {
			return err
		}
		values["POSTGRES_HOST_PORT"] = strconv.Itoa(port)
		excluded[port] = struct{}{}
		changed = true
	}
	if m.Services.Redis && values["VALKEY_HOST_PORT"] == "" {
		port, err := allocateLoopbackPort(excluded)
		if err != nil {
			return err
		}
		values["VALKEY_HOST_PORT"] = strconv.Itoa(port)
		excluded[port] = struct{}{}
		changed = true
	}
	if err := validateRuntimeValues(values, m); err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return writeRuntimeEnv(path, m, values)
}

func newRuntimeEnv(m Manifest) (string, error) {
	values := map[string]string{}
	excluded := map[int]struct{}{}
	if m.Services.Postgres {
		password, err := randomApplicationSecret(32)
		if err != nil {
			return "", err
		}
		port, err := allocateLoopbackPort(excluded)
		if err != nil {
			return "", err
		}
		excluded[port] = struct{}{}
		values["POSTGRES_DB"] = postgresDatabaseName(m)
		values["POSTGRES_USER"] = "baseharbor"
		values["POSTGRES_PASSWORD"] = password
		values["POSTGRES_HOST_PORT"] = strconv.Itoa(port)
	}
	if m.Services.Redis {
		password, err := randomApplicationSecret(32)
		if err != nil {
			return "", err
		}
		port, err := allocateLoopbackPort(excluded)
		if err != nil {
			return "", err
		}
		values["VALKEY_PASSWORD"] = password
		values["VALKEY_HOST_PORT"] = strconv.Itoa(port)
	}
	return runtimeEnvContent(m, values), nil
}

func writeRuntimeEnv(path string, m Manifest, values map[string]string) error {
	if err := writeOwnerOnlyFile(path, []byte(runtimeEnvContent(m, values))); err != nil {
		return fmt.Errorf("write application runtime environment: %w", err)
	}
	return nil
}

func runtimeEnvContent(m Manifest, values map[string]string) string {
	var b strings.Builder
	if m.Services.Postgres {
		fmt.Fprintf(&b, "POSTGRES_DB=%s\n", values["POSTGRES_DB"])
		fmt.Fprintf(&b, "POSTGRES_USER=%s\n", values["POSTGRES_USER"])
		fmt.Fprintf(&b, "POSTGRES_PASSWORD=%s\n", values["POSTGRES_PASSWORD"])
		fmt.Fprintf(&b, "POSTGRES_HOST_PORT=%s\n", values["POSTGRES_HOST_PORT"])
	}
	if m.Services.Redis {
		fmt.Fprintf(&b, "VALKEY_PASSWORD=%s\n", values["VALKEY_PASSWORD"])
		fmt.Fprintf(&b, "VALKEY_HOST_PORT=%s\n", values["VALKEY_HOST_PORT"])
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
	if m.Services.Postgres {
		for _, key := range []string{"POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_HOST_PORT"} {
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		if err := validatePortValue(values["POSTGRES_HOST_PORT"], "POSTGRES_HOST_PORT"); err != nil {
			return err
		}
	}
	if m.Services.Redis {
		for _, key := range []string{"VALKEY_PASSWORD", "VALKEY_HOST_PORT"} {
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		if err := validatePortValue(values["VALKEY_HOST_PORT"], "VALKEY_HOST_PORT"); err != nil {
			return err
		}
	}
	return nil
}

func postgresDatabaseName(m Manifest) string {
	name := strings.ReplaceAll(m.Name+"_"+m.Environment, "-", "_")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func randomApplicationSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate application runtime secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

const postgresComposeYAML = `services:
  postgres:
    image: postgres:18-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    ports:
      - "127.0.0.1:${POSTGRES_HOST_PORT}:5432"
    volumes:
      - postgres-data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s

volumes:
  postgres-data:
`

const valkeyServiceYAML = `  valkey:
    image: valkey/valkey:9.1.2-alpine
    restart: unless-stopped
    environment:
      VALKEY_PASSWORD: ${VALKEY_PASSWORD}
    command:
      - sh
      - -ec
      - |
        printf 'requirepass %s\nappendonly yes\ndir /data\n' "$$VALKEY_PASSWORD" > /tmp/valkey.conf
        exec valkey-server /tmp/valkey.conf
    ports:
      - "127.0.0.1:${VALKEY_HOST_PORT}:6379"
    volumes:
      - valkey-data:/data
    healthcheck:
      test: ["CMD-SHELL", "VALKEYCLI_AUTH=\"$${VALKEY_PASSWORD}\" valkey-cli ping | grep -q '^PONG$'"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s
`

const valkeyComposeYAML = `services:
` + valkeyServiceYAML + `
volumes:
  valkey-data:
`

const postgresValkeyComposeYAML = `services:
  postgres:
    image: postgres:18-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    ports:
      - "127.0.0.1:${POSTGRES_HOST_PORT}:5432"
    volumes:
      - postgres-data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s

` + valkeyServiceYAML + `
volumes:
  postgres-data:
  valkey-data:
`
