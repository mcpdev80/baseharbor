package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrUnsupportedService = errors.New("application contains services that are not yet supported by apply")

type RuntimeFiles struct {
	Dir     string
	Compose string
	Env     string
}

func RuntimeProjectName(m Manifest) string {
	return "baseharbor-" + m.Name + "-" + m.Environment
}

func CheckSupportedRuntimeServices(m Manifest) error {
	if m.Services.Secrets {
		return fmt.Errorf("%w: managed secrets convergence is not implemented yet", ErrUnsupportedService)
	}
	if !m.Services.Postgres && !m.Services.Redis {
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
	files := RuntimeFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
	if _, err := os.Stat(files.Env); errors.Is(err, os.ErrNotExist) {
		content, err := newRuntimeEnv(m)
		if err != nil {
			return RuntimeFiles{}, err
		}
		if err := os.WriteFile(files.Env, []byte(content), 0o600); err != nil {
			return RuntimeFiles{}, fmt.Errorf("write application runtime environment: %w", err)
		}
	} else if err != nil {
		return RuntimeFiles{}, fmt.Errorf("inspect application runtime environment: %w", err)
	} else if err := validateRuntimeEnv(files.Env, m); err != nil {
		return RuntimeFiles{}, err
	}

	compose, err := RuntimeComposeYAML(m)
	if err != nil {
		return RuntimeFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		return RuntimeFiles{}, fmt.Errorf("write application compose file: %w", err)
	}
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

func newRuntimeEnv(m Manifest) (string, error) {
	var b strings.Builder
	if m.Services.Postgres {
		password, err := randomApplicationSecret(32)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "POSTGRES_DB=%s\nPOSTGRES_USER=baseharbor\nPOSTGRES_PASSWORD=%s\n", postgresDatabaseName(m), password)
	}
	if m.Services.Redis {
		password, err := randomApplicationSecret(32)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "VALKEY_PASSWORD=%s\n", password)
	}
	return b.String(), nil
}

func validateRuntimeEnv(path string, m Manifest) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read application runtime environment: %w", err)
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(value) == "" {
			return errors.New("application runtime environment contains an invalid entry")
		}
		values[key] = value
	}
	if m.Services.Postgres {
		for _, key := range []string{"POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD"} {
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
	}
	if m.Services.Redis && values["VALKEY_PASSWORD"] == "" {
		return errors.New("application runtime environment is missing VALKEY_PASSWORD")
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
