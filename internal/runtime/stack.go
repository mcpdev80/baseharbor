package runtime

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultStateDir = ".baseharbor/runtime"
	composeName     = "compose.yaml"
	envName         = "runtime.env"
)

const (
	DefaultPostgresPort = 5432
	DefaultOpenBaoPort  = 8200
)

//go:embed assets/compose.yaml
var composeYAML []byte

type Files struct {
	Compose string
	Env     string
}

type Ports struct {
	Postgres int
	OpenBao  int
}

func EnsureFiles(stateDir string) (Files, error) {
	return EnsureFilesWithPorts(stateDir, Ports{Postgres: DefaultPostgresPort, OpenBao: DefaultOpenBaoPort})
}

func EnsureFilesWithPorts(stateDir string, ports Ports) (Files, error) {
	if stateDir == "" {
		stateDir = DefaultStateDir
	}
	if ports.Postgres <= 0 || ports.Postgres > 65535 {
		return Files{}, fmt.Errorf("invalid postgres port %d", ports.Postgres)
	}
	if ports.OpenBao <= 0 || ports.OpenBao > 65535 {
		return Files{}, fmt.Errorf("invalid OpenBao port %d", ports.OpenBao)
	}
	if ports.Postgres == ports.OpenBao {
		return Files{}, fmt.Errorf("postgres and OpenBao cannot share host port %d", ports.Postgres)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return Files{}, fmt.Errorf("create runtime state directory: %w", err)
	}

	composePath := filepath.Join(stateDir, composeName)
	if err := os.WriteFile(composePath, composeYAML, 0o600); err != nil {
		return Files{}, fmt.Errorf("write compose file: %w", err)
	}

	envPath := filepath.Join(stateDir, envName)
	if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		password, err := randomSecret(32)
		if err != nil {
			return Files{}, err
		}
		content := fmt.Sprintf("BASEHARBOR_POSTGRES_DB=baseharbor\nBASEHARBOR_POSTGRES_USER=baseharbor\nBASEHARBOR_POSTGRES_PASSWORD=%s\nBASEHARBOR_POSTGRES_PORT=%d\nBASEHARBOR_OPENBAO_PORT=%d\n", password, ports.Postgres, ports.OpenBao)
		if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
			return Files{}, fmt.Errorf("write runtime environment: %w", err)
		}
	} else if err != nil {
		return Files{}, fmt.Errorf("inspect runtime environment: %w", err)
	}

	return Files{Compose: composePath, Env: envPath}, nil
}

func ExistingFiles(stateDir string) (Files, error) {
	if stateDir == "" {
		stateDir = DefaultStateDir
	}
	files := Files{Compose: filepath.Join(stateDir, composeName), Env: filepath.Join(stateDir, envName)}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			return Files{}, err
		}
	}
	return files, nil
}

func randomSecret(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate runtime secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
