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
	Compose         string
	Env             string
	Project         string
	ResourceProject string
}

type Ports struct {
	Postgres int
	OpenBao  int
}

func EnsureFiles(stateDir string) (Files, error) {
	return EnsureFilesForProject(stateDir, "baseharbor", Ports{Postgres: DefaultPostgresPort, OpenBao: DefaultOpenBaoPort})
}

func EnsureFilesWithPorts(stateDir string, ports Ports) (Files, error) {
	return EnsureFilesForProject(stateDir, "baseharbor", ports)
}

func EnsureFilesForProject(stateDir, project string, ports Ports) (Files, error) {
	return EnsureFilesForProjectAndResources(stateDir, project, project, ports)
}

func EnsureFilesForProjectAndResources(stateDir, project, resourceProject string, ports Ports) (Files, error) {
	project = strings.TrimSpace(project)
	resourceProject = strings.TrimSpace(resourceProject)
	if project == "" {
		return Files{}, errors.New("runtime project is required")
	}
	if resourceProject == "" {
		return Files{}, errors.New("runtime resource project is required")
	}
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

	rendered := renderComposeForProject(resourceProject)
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

	if err := prepareOpenBaoStorage(stateDir, envPath); err != nil {
		return Files{}, err
	}
	if err := normalizeProjectedFiles(
		filepath.Join(stateDir, "providers", "postgresql", "runtime"),
		filepath.Join(stateDir, "providers", "openbao", "runtime"),
	); err != nil {
		return Files{}, err
	}
	return Files{Compose: composePath, Env: envPath, Project: project, ResourceProject: resourceProject}, nil
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
	openBaoPolicy.ServerName = "openbao"
	openBaoRoot := filepath.Join(stateDir, "providers", "openbao")
	openBaoMaterial, err := serviceaccess.EnsureTLSMaterial(
		ctx,
		issuer,
		openBaoPolicy,
		filepath.Join(openBaoRoot, "service-access", "pki"),
		"openbao",
		"127.0.0.1",
	)
	if err != nil {
		return fmt.Errorf("prepare OpenBao native TLS: %w", err)
	}
	if err := projectControlPlaneOpenBaoTLS(openBaoRoot, openBaoMaterial); err != nil {
		return fmt.Errorf("project OpenBao native TLS: %w", err)
	}

	postgresPolicy, err := serviceaccess.Resolve("prod", "control-plane-postgresql", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	postgresRoot := filepath.Join(stateDir, "providers", "postgresql")
	postgresMaterial, err := serviceaccess.EnsureTLSMaterial(
		ctx,
		issuer,
		postgresPolicy,
		filepath.Join(postgresRoot, "service-access", "pki"),
		"postgres",
		"127.0.0.1",
	)
	if err != nil {
		return fmt.Errorf("prepare control-plane PostgreSQL native TLS: %w", err)
	}
	if err := projectControlPlanePostgresTLS(postgresRoot, postgresMaterial); err != nil {
		return fmt.Errorf("project control-plane PostgreSQL TLS: %w", err)
	}

	resourceProject := strings.TrimSpace(files.ResourceProject)
	if resourceProject == "" {
		resourceProject = sharedResourceProjectNameForOperatorProject(files.Project)
	}
	rendered := renderComposeForProject(resourceProject)
	rendered, err = renderSecureControlPlanePostgres(rendered)
	if err != nil {
		return err
	}
	rendered, err = renderSecureControlPlaneOpenBao(rendered)
	if err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write control-plane service access compose: %w", err)
	}
	return nil
}

func ControlPlaneNetworkName(resourceProject string) string {
	resourceProject = strings.TrimSpace(resourceProject)
	if resourceProject == "" {
		resourceProject = "baseharbor"
	}
	return resourceProject + "-default"
}

func renderComposeForProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		project = "baseharbor"
	}
	rendered := strings.ReplaceAll(string(composeYAML), "name: baseharbor-secrets", "name: "+project+"-secrets")
	rendered = strings.ReplaceAll(rendered, "  default: {}\n", "  default:\n    name: "+ControlPlaneNetworkName(project)+"\n")
	return rendered
}

func projectControlPlanePostgresTLS(root string, material serviceaccess.TLSMaterial) error {
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		return err
	}
	for source, name := range map[string]string{
		material.CA:                "ca.pem",
		material.ServerCertificate: "server-cert.pem",
		material.ServerKey:         "server-key.pem",
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("control-plane PostgreSQL TLS material %s is empty", name)
		}
		if err := os.WriteFile(filepath.Join(runtimeDir, name), data, 0o644); err != nil {
			return err
		}
	}
	const hba = `local all all trust
hostssl all all 0.0.0.0/0 scram-sha-256
hostssl all all ::/0 scram-sha-256
hostnossl all all 0.0.0.0/0 reject
hostnossl all all ::/0 reject
`
	return os.WriteFile(filepath.Join(runtimeDir, "pg_hba.conf"), []byte(hba), 0o644)
}

func projectControlPlaneOpenBaoTLS(root string, material serviceaccess.TLSMaterial) error {
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		return err
	}
	for source, name := range map[string]string{
		material.CA:                "ca.pem",
		material.ServerCertificate: "server-cert.pem",
		material.ServerKey:         "server-key.pem",
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("OpenBao TLS material %s is empty", name)
		}
		// The provider directory remains owner-only (0700), while the bind-mounted
		// leaf material must be readable by OpenBao's non-root runtime UID.
		if err := os.WriteFile(filepath.Join(runtimeDir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func renderSecureControlPlaneOpenBao(rendered string) (string, error) {
	for _, required := range []string{
		"  openbao:\n",
		"docker.io/openbao/openbao:2.7.0",
		"command: [\"server\", \"-config=/run/baseharbor/openbao/openbao.hcl\"]",
		"./providers/openbao/runtime/openbao.hcl:/run/baseharbor/openbao/openbao.hcl:ro",
		"./providers/postgresql/runtime/ca.pem:/run/baseharbor/postgres-ca/ca.pem:ro",
	} {
		if !strings.Contains(rendered, required) {
			return "", fmt.Errorf("embedded runtime compose is missing secure OpenBao runtime %q", required)
		}
	}
	for _, forbidden := range []string{`"storage":{"file"`, `"storage":{"raft"`, "/openbao/file", "/openbao/raft", "openbao-data:"} {
		if strings.Contains(rendered, forbidden) {
			return "", fmt.Errorf("embedded runtime compose contains obsolete OpenBao storage %q", forbidden)
		}
	}
	return rendered, nil
}

func renderSecureControlPlanePostgres(rendered string) (string, error) {
	for _, required := range []string{
		"  postgres:\n",
		"-c ssl=on",
		"hba_file=/run/baseharbor/tls-source/pg_hba.conf",
		"BASEHARBOR_OPENBAO_DB_PASSWORD",
		"./providers/postgresql/runtime/openbao-init.sh:/docker-entrypoint-initdb.d/20-baseharbor-openbao.sh:ro",
	} {
		if !strings.Contains(rendered, required) {
			return "", fmt.Errorf("embedded runtime compose is missing secure PostgreSQL runtime %q", required)
		}
	}
	return rendered, nil
}

func ExistingFiles(stateDir string) (Files, error) {
	return ExistingFilesForProject(stateDir, "baseharbor")
}

func ExistingFilesForProject(stateDir, project string) (Files, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return Files{}, errors.New("runtime project is required")
	}
	resolved, err := resolveStateDir(stateDir)
	if err != nil {
		return Files{}, err
	}
	stateDir = resolved
	files := Files{Compose: filepath.Join(stateDir, composeName), Env: filepath.Join(stateDir, envName), Project: project, ResourceProject: sharedResourceProjectNameForOperatorProject(project)}
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
