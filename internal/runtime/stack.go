package runtime

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	legacyStateDir = ".baseharbor/runtime"
	composeName    = "compose.yaml"
	envName        = "runtime.env"
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
	resolved, err := resolveStateDir(stateDir)
	if err != nil {
		return Files{}, err
	}
	stateDir = resolved
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

	rendered := string(composeYAML)
	composePath := filepath.Join(stateDir, composeName)
	if err := os.WriteFile(composePath, []byte(rendered), 0o600); err != nil {
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

func EnsureServiceAccess(ctx context.Context, issuer serviceaccess.Issuer, files Files) error {
	if issuer == nil {
		return errors.New("control-plane service access requires an issuer")
	}
	stateDir := filepath.Dir(files.Compose)
	openBaoPolicy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	openBaoAccess, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, openBaoPolicy, filepath.Join(stateDir, "providers", "openbao"), serviceaccess.HTTPGatewaySpec{
		ServiceName:      "openbao-access",
		Upstream:         "http://openbao:8200",
		PublishedPortEnv: "BASEHARBOR_OPENBAO_PORT",
		ContainerPort:    8443,
		Networks:         []string{"default"},
		RequireClient:    false,
	})
	if err != nil {
		return fmt.Errorf("prepare OpenBao HTTPS access: %w", err)
	}
	postgresPolicy, err := serviceaccess.Resolve("prod", "control-plane-postgresql", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	postgresAccess, err := serviceaccess.EnsureTCPGateway(ctx, issuer, postgresPolicy, filepath.Join(stateDir, "providers", "postgresql"), serviceaccess.TCPGatewaySpec{
		ServiceName:      "postgres-access",
		UpstreamHost:     "postgres",
		UpstreamPort:     5432,
		PublishedPortEnv: "BASEHARBOR_POSTGRES_PORT",
		ContainerPort:    5432,
	})
	if err != nil {
		return fmt.Errorf("prepare control-plane PostgreSQL TLS access: %w", err)
	}

	rendered := string(composeYAML)
	accessServices := serviceaccess.HTTPGatewayComposeService(openBaoAccess, serviceaccess.HTTPGatewaySpec{
		ServiceName: "openbao-access", Upstream: "http://openbao:8200",
		PublishedPortEnv: "BASEHARBOR_OPENBAO_PORT", ContainerPort: 8443,
		Networks: []string{"default"}, RequireClient: false,
	}) + serviceaccess.TCPGatewayComposeService(postgresAccess, serviceaccess.TCPGatewaySpec{
		ServiceName: "postgres-access", UpstreamHost: "postgres", UpstreamPort: 5432,
		PublishedPortEnv: "BASEHARBOR_POSTGRES_PORT", ContainerPort: 5432,
	})
	if marker := strings.Index(rendered, "\nvolumes:\n"); marker >= 0 {
		rendered = rendered[:marker] + "\n" + accessServices + rendered[marker:]
	} else {
		return errors.New("embedded runtime compose is missing volumes section")
	}
	if err := os.WriteFile(files.Compose, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write control-plane service access compose: %w", err)
	}
	return nil
}

func ExistingFiles(stateDir string) (Files, error) {
	resolved, err := resolveStateDir(stateDir)
	if err != nil {
		return Files{}, err
	}
	stateDir = resolved
	files := Files{Compose: filepath.Join(stateDir, composeName), Env: filepath.Join(stateDir, envName)}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			return Files{}, err
		}
	}
	return files, nil
}

// StateDir resolves the authoritative BaseHarbor runtime state directory without mutating it.
func StateDir(stateDir string) (string, error) {
	return resolveStateDir(stateDir)
}

// DataDir resolves the BaseHarbor data root that owns runtime-global metadata.
// Explicit state-directory overrides remain self-contained. Without an
// override, metadata is stored beside runtime/ so creating it cannot change
// legacy/global runtime selection.
func DataDir(stateDir string) (string, error) {
	if stateDir != "" {
		return filepath.Clean(stateDir), nil
	}
	if override := os.Getenv("BASEHARBOR_STATE_DIR"); override != "" {
		return filepath.Clean(override), nil
	}
	runtimeDir, err := resolveStateDir("")
	if err != nil {
		return "", err
	}
	return filepath.Dir(runtimeDir), nil
}

func resolveStateDir(stateDir string) (string, error) {
	if stateDir != "" {
		return filepath.Clean(stateDir), nil
	}
	if override := os.Getenv("BASEHARBOR_STATE_DIR"); override != "" {
		return filepath.Clean(override), nil
	}

	global, err := globalStateDir()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(global); err == nil {
		return global, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect global runtime state: %w", err)
	}

	// Compatibility for pre-global-state installations and CI fixtures.
	// Existing legacy state is reused only when no global state exists yet.
	if _, err := os.Stat(filepath.Join(legacyStateDir, envName)); err == nil {
		return legacyStateDir, nil
	}
	return global, nil
}

func globalStateDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "baseharbor", "runtime"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot determine BaseHarbor global state directory; set BASEHARBOR_STATE_DIR")
	}
	return filepath.Join(home, ".local", "share", "baseharbor", "runtime"), nil
}

func randomSecret(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate runtime secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
