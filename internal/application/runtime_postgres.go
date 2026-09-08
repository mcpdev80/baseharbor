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

func EnsurePostgresRuntime(store Store, m Manifest) (RuntimeFiles, error) {
	if err := m.Validate(); err != nil {
		return RuntimeFiles{}, err
	}
	if !m.Services.Postgres {
		return RuntimeFiles{}, fmt.Errorf("postgres service is required for the current apply milestone")
	}
	if m.Services.Redis || m.Services.Secrets {
		return RuntimeFiles{}, fmt.Errorf("%w: redis/secrets convergence is not implemented yet", ErrUnsupportedService)
	}

	dir := filepath.Join(store.Root, m.Name, "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return RuntimeFiles{}, fmt.Errorf("create application runtime directory: %w", err)
	}
	files := RuntimeFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
	if err := os.WriteFile(files.Compose, []byte(postgresComposeYAML), 0o600); err != nil {
		return RuntimeFiles{}, fmt.Errorf("write application compose file: %w", err)
	}
	if _, err := os.Stat(files.Env); errors.Is(err, os.ErrNotExist) {
		password, err := randomApplicationSecret(32)
		if err != nil {
			return RuntimeFiles{}, err
		}
		content := fmt.Sprintf("POSTGRES_DB=%s\nPOSTGRES_USER=baseharbor\nPOSTGRES_PASSWORD=%s\n", postgresDatabaseName(m), password)
		if err := os.WriteFile(files.Env, []byte(content), 0o600); err != nil {
			return RuntimeFiles{}, fmt.Errorf("write application runtime environment: %w", err)
		}
	} else if err != nil {
		return RuntimeFiles{}, fmt.Errorf("inspect application runtime environment: %w", err)
	}
	return files, nil
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
