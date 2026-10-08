package identityprovider

import (
	"context"
	"errors"
	"os"
	"sort"

	"go.yaml.in/yaml/v3"
)

type keycloakSelectedRuntime interface {
	UpProjectFilesSelected(context.Context, string, string, map[string]string, []string, ...string) error
}

// A TCP listener does not prove that Keycloak's initial Liquibase migration
// and master-realm bootstrap have completed. Start one member and authenticate
// over its protected HTTPS gateway before allowing the other members to join.
func applyKeycloakBootstrap(ctx context.Context, runtime KeycloakRuntime, files KeycloakFiles, verify func(context.Context, KeycloakFiles) error) error {
	selected, ok := runtime.(keycloakSelectedRuntime)
	if !ok {
		return runtime.UpProject(ctx, files.Project, files.Compose, files.Env)
	}
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return err
	}
	services, err := keycloakBootstrapServices(files.Compose)
	if err != nil {
		return err
	}
	if err := selected.UpProjectFilesSelected(ctx, files.Project, files.Dir, values, services, files.Compose); err != nil {
		return err
	}
	if err := verify(ctx, files); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return runtime.UpProject(ctx, files.Project, files.Compose, files.Env)
}

func keycloakBootstrapServices(compose string) ([]string, error) {
	data, err := os.ReadFile(compose)
	if err != nil {
		return nil, err
	}
	var document struct {
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	for _, required := range []string{"keycloak-1", "keycloak-access"} {
		if _, ok := document.Services[required]; !ok {
			return nil, errors.New("Keycloak bootstrap graph is incomplete")
		}
	}
	if _, second := document.Services["keycloak-2"]; second {
		if _, third := document.Services["keycloak-3"]; !third {
			return nil, errors.New("Keycloak HA bootstrap graph has fewer than three members")
		}
	} else if _, third := document.Services["keycloak-3"]; third {
		return nil, errors.New("Keycloak HA bootstrap graph is incomplete")
	}
	var selected []string
	for name := range document.Services {
		if name != "keycloak-2" && name != "keycloak-3" {
			selected = append(selected, name)
		}
	}
	sort.Strings(selected)
	return selected, nil
}
