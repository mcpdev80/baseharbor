package identityprovider

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

// Retained dedicated SQL is never converted by up/update. An explicit,
// backup-verified migration with rollback must be implemented separately.
func rejectLegacySharedIdentity(dataDir string) error {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "keycloak", "shared")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return errors.New("shared Identity state contains an unsupported non-directory entry")
		}
		path := filepath.Join(root, entry.Name(), "compose.yaml")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return errors.New("shared Identity Compose must be a protected regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var graph struct {
			Services map[string]any `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &graph); err != nil {
			return err
		}
		for name := range graph.Services {
			if name == "keycloak-db" || strings.HasPrefix(name, "keycloak-db-member-") || entry.Name() != "core" && strings.HasPrefix(name, "keycloak-") {
				return errors.New("existing shared Identity uses a separate provider or SQL database; explicit backup-verified migration is required and is currently unsupported; existing volumes, realms and credentials are retained")
			}
		}
	}
	return nil
}

func bindKeycloakCoreSQL(files *KeycloakFiles, values map[string]string, core bhruntime.Files) error {
	if core.Project == "" || core.ResourceProject == "" || core.Compose == "" || core.Env == "" {
		return errors.New("shared Identity Core SQL binding is incomplete")
	}
	bindings := map[string]string{
		"BASEHARBOR_KEYCLOAK_SQL_PLACEMENT":     "core-shared",
		"BASEHARBOR_KEYCLOAK_SQL_CORE_PROJECT":  core.Project,
		"BASEHARBOR_KEYCLOAK_SQL_CORE_RESOURCE": core.ResourceProject,
		"BASEHARBOR_KEYCLOAK_SQL_CORE_COMPOSE":  core.Compose,
		"BASEHARBOR_KEYCLOAK_SQL_CORE_ENV":      core.Env,
	}
	for key, wanted := range bindings {
		if existing := values[key]; existing != "" && existing != wanted {
			return errors.New("shared Identity SQL dependency changed; explicit verified migration is required")
		}
		values[key] = wanted
	}
	values["BASEHARBOR_KEYCLOAK_DB_NAME"] = "baseharbor_identity"
	values["BASEHARBOR_KEYCLOAK_DB_USER"] = "baseharbor_identity"
	delete(values, "BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD")
	delete(values, "BASEHARBOR_KEYCLOAK_DB_REPLICATION_PASSWORD")
	files.SharedSQL = &core
	return nil
}

func existingKeycloakCoreSQL(values map[string]string) (*bhruntime.Files, error) {
	if values["BASEHARBOR_KEYCLOAK_SQL_PLACEMENT"] == "" {
		return nil, nil
	}
	if values["BASEHARBOR_KEYCLOAK_SQL_PLACEMENT"] != "core-shared" {
		return nil, errors.New("unsupported Identity SQL placement")
	}
	core := bhruntime.Files{Project: values["BASEHARBOR_KEYCLOAK_SQL_CORE_PROJECT"], ResourceProject: values["BASEHARBOR_KEYCLOAK_SQL_CORE_RESOURCE"], Compose: values["BASEHARBOR_KEYCLOAK_SQL_CORE_COMPOSE"], Env: values["BASEHARBOR_KEYCLOAK_SQL_CORE_ENV"]}
	if err := bindKeycloakCoreSQL(&KeycloakFiles{}, map[string]string{}, core); err != nil {
		return nil, err
	}
	return &core, nil
}

func projectKeycloakCoreSQLCA(files KeycloakFiles, core bhruntime.Files) error {
	path := bhruntime.CorePostgresCA(core)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("shared Identity requires the existing Core PostgreSQL CA")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("shared Identity Core PostgreSQL CA is invalid")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !certificate.IsCA {
		return errors.New("shared Identity Core PostgreSQL CA is not a CA certificate")
	}
	dir := filepath.Join(files.Dir, "db-ha", "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "ca.pem"), data, 0o644)
}
