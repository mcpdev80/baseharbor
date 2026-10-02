package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func ensureSharedBackendTLS(ctx context.Context, issuer serviceaccess.Issuer, shared SharedBackendFiles, m Manifest, state *sharedBackendState, files RuntimeFiles, values map[string]string) error {
	if UsesSharedPostgreSQL(m) {
		policy, err := serviceaccess.Resolve(m.Environment, "postgresql", serviceaccess.AuthenticationNative)
		if err != nil {
			return err
		}
		root := filepath.Join(shared.Dir, "postgresql")
		material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(root, "service-access", "pki"), sharedPostgresService(m.Environment), sharedPostgresAlias(), "127.0.0.1")
		if err != nil {
			return fmt.Errorf("prepare shared PostgreSQL TLS: %w", err)
		}
		if err := projectPostgresServerMaterial(root, material); err != nil {
			return err
		}
		for _, instance := range SQLInstanceNames(m) {
			ca, err := projectBackendCA(files, "postgres", instance, material.CA)
			if err != nil {
				return err
			}
			values[postgresTLSCAKey(instance)] = ca
		}
	}

	if UsesSharedValkey(m) {
		app := state.Applications[sharedBackendApplicationKey(m)]
		for _, instance := range ValkeyInstanceNames(m) {
			root := filepath.Join(shared.Dir, "valkey", sharedBackendToken(m.Name), sharedBackendToken(instance))
			policy, err := serviceaccess.Resolve(m.Environment, "valkey", serviceaccess.AuthenticationNative)
			if err != nil {
				return err
			}
			_, err = serviceaccess.EnsureTCPGateway(ctx, issuer, policy, root, sharedValkeyGatewaySpec(app, instance))
			if err != nil {
				return fmt.Errorf("prepare shared Valkey TLS for %s: %w", instance, err)
			}
			material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(root, "service-access", "pki"))
			if err != nil {
				return err
			}
			ca, err := projectBackendCA(files, "valkey", instance, material.CA)
			if err != nil {
				return err
			}
			values[valkeyTLSCAKey(instance)] = ca
		}
	}
	return nil
}

func ensureSharedManagementUIState(state *sharedBackendState, m Manifest, values map[string]string) error {
	if m.Services.SQLManagementUI && UsesSharedPostgreSQL(m) {
		if state.PostgresUIHostPort == 0 {
			port, err := allocateLoopbackPort(nil)
			if err != nil {
				return err
			}
			state.PostgresUIHostPort = port
		}
		values[PostgresUIHostPortEnv] = strconv.Itoa(state.PostgresUIHostPort)
	}
	if (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && UsesSharedValkey(m) {
		if state.CacheUIHostPort == 0 {
			port, err := allocateLoopbackPort(nil)
			if err != nil {
				return err
			}
			state.CacheUIHostPort = port
		}
		values[CacheUIHostPortEnv] = strconv.Itoa(state.CacheUIHostPort)
	}
	if (m.Services.SQLManagementUI && UsesSharedPostgreSQL(m)) || ((m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && UsesSharedValkey(m)) {
		username := strings.TrimSpace(values[CacheUIUserEnv])
		if username == "" {
			email := strings.TrimSpace(values[PostgresUIEmailEnv])
			if at := strings.IndexByte(email, '@'); at > 0 {
				username = email[:at]
			}
		}
		password := strings.TrimSpace(values[CacheUIPasswordEnv])
		if password == "" {
			password = strings.TrimSpace(values[PostgresUIPasswordEnv])
		}
		if username == "" || password == "" {
			return errors.New("shared backend management UI credentials are incomplete")
		}
		if state.ManagementUsername == "" || state.ManagementPassword == "" {
			state.ManagementUsername = username
			state.ManagementPassword = password
		} else if strings.EqualFold(strings.TrimSpace(m.Environment), "dev") ||
			strings.EqualFold(strings.TrimSpace(m.Environment), "development") {
			// Development credentials are Target-scoped and explicitly rotatable.
			state.ManagementUsername = username
			state.ManagementPassword = password
		}
		values[CacheUIUserEnv] = state.ManagementUsername
		values[CacheUIPasswordEnv] = state.ManagementPassword
		values[PostgresUIEmailEnv] = developmentPostgresUIEmail(state.ManagementUsername)
		values[PostgresUIPasswordEnv] = state.ManagementPassword
	}
	return nil
}

func ensureSharedManagementUIs(ctx context.Context, issuer serviceaccess.Issuer, shared SharedBackendFiles, m Manifest, state sharedBackendState, values map[string]string) error {
	if m.Services.SQLManagementUI && UsesSharedPostgreSQL(m) {
		if err := ensureSharedPostgresManagementUI(ctx, issuer, shared, m.Environment, state); err != nil {
			return err
		}
	}
	if (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && UsesSharedValkey(m) {
		if err := ensureSharedCacheManagementUI(ctx, issuer, shared, m.Environment, state); err != nil {
			return err
		}
	}
	return nil
}

func ensureSharedPostgresManagementUI(ctx context.Context, issuer serviceaccess.Issuer, shared SharedBackendFiles, environment string, state sharedBackendState) error {
	dir := filepath.Join(shared.Dir, "management-ui", "postgres")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve(environment, "pgadmin", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", "shared-pgadmin")
	if err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.cert")); err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server.key")); err != nil {
		return err
	}
	return refreshSharedPostgresManagementUIConfig(shared, state)
}

func refreshSharedPostgresManagementUIConfig(shared SharedBackendFiles, state sharedBackendState) error {
	dir := filepath.Join(shared.Dir, "management-ui", "postgres")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "password"), []byte(state.ManagementPassword+"\n"), 0o644); err != nil {
		return err
	}
	postgresPolicy, err := serviceaccess.Resolve(state.Environment, "postgresql", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	postgresMaterial, err := serviceaccess.ExistingTLSMaterial(postgresPolicy, filepath.Join(shared.Dir, "postgresql", "service-access", "pki"))
	if err != nil {
		return err
	}
	if err := projectUIReadableFile(postgresMaterial.CA, filepath.Join(dir, "postgres.ca.pem")); err != nil {
		return err
	}
	var pgpass strings.Builder
	servers := map[string]any{"Servers": map[string]any{}}
	serverMap := servers["Servers"].(map[string]any)
	index := 1
	appKeys := sortedSharedBackendApplicationKeys(state)
	for _, key := range appKeys {
		app := state.Applications[key]
		instances := make([]string, 0, len(app.SQL))
		for instance := range app.SQL {
			instances = append(instances, instance)
		}
		sort.Strings(instances)
		for _, instance := range instances {
			resource := app.SQL[instance]
			password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
			if err != nil {
				return fmt.Errorf("load shared PostgreSQL UI credential for %s/%s/%s: %w", app.Application, app.Environment, instance, err)
			}
			fmt.Fprintf(&pgpass, "%s:5432:*:%s:%s\n", sharedPostgresAlias(), resource.Username, password)
			label := app.Application
			if instance != defaultServiceInstance {
				label += " / " + instance
			}
			serverMap[strconv.Itoa(index)] = map[string]any{
				"Name": label, "Group": "BaseHarbor", "Host": sharedPostgresAlias(), "Port": 5432,
				"MaintenanceDB": resource.Database, "Username": resource.Username, "SSLMode": "verify-ca",
				"PassFile": "/run/baseharbor/pgpass",
				"ConnectionParameters": map[string]any{
					"sslmode": "verify-ca", "sslrootcert": "/run/baseharbor/postgres.ca.pem", "passfile": "/run/baseharbor/pgpass",
				},
			}
			index++
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "pgpass"), []byte(pgpass.String()), 0o644); err != nil {
		return err
	}
	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "servers.json"), append(data, '\n'), 0o644)
}

func ensureSharedCacheManagementUI(ctx context.Context, issuer serviceaccess.Issuer, shared SharedBackendFiles, environment string, state sharedBackendState) error {
	dir := filepath.Join(shared.Dir, "management-ui", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve(environment, "redis-commander", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", "shared-cache-ui")
	if err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.pem")); err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server-key.pem")); err != nil {
		return err
	}
	return refreshSharedCacheManagementUIConfig(shared, state)
}

func refreshSharedCacheManagementUIConfig(shared SharedBackendFiles, state sharedBackendState) error {
	dir := filepath.Join(shared.Dir, "management-ui", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "http-password"), []byte(state.ManagementPassword+"\n"), 0o644); err != nil {
		return err
	}
	var connections []map[string]any
	appKeys := sortedSharedBackendApplicationKeys(state)
	for _, key := range appKeys {
		app := state.Applications[key]
		instances := make([]string, 0, len(app.Cache))
		for instance := range app.Cache {
			instances = append(instances, instance)
		}
		sort.Strings(instances)
		for _, instance := range instances {
			resource := app.Cache[instance]
			password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
			if err != nil {
				return err
			}
			root := filepath.Join(shared.Dir, "valkey", sharedBackendToken(app.Application), sharedBackendToken(instance))
			valkeyPolicy, err := serviceaccess.Resolve(state.Environment, "valkey", serviceaccess.AuthenticationNative)
			if err != nil {
				return err
			}
			valkeyMaterial, err := serviceaccess.ExistingTLSMaterial(valkeyPolicy, filepath.Join(root, "service-access", "pki"))
			if err != nil {
				return err
			}
			caData, err := os.ReadFile(valkeyMaterial.CA)
			if err != nil {
				return err
			}
			label := app.Application
			if instance != defaultServiceInstance {
				label += " / " + instance
			}
			connections = append(connections, map[string]any{
				"label": label, "host": sharedValkeyAccessServiceFor(app.Application, app.Environment, instance),
				"port": 6379, "username": "default", "password": password, "dbIndex": 0,
				"tls": map[string]any{
					"ca":         []string{strings.TrimSpace(string(caData))},
					"servername": sharedValkeyAccessServiceFor(app.Application, app.Environment, instance),
				},
			})
		}
	}
	data, err := json.MarshalIndent(map[string]any{"connections": connections}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "local.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "local-production.json"), []byte("{}\n"), 0o644); err != nil {
		return err
	}
	caddy := "{\n  auto_https disable_redirects\n}\n\n:8443 {\n  tls /certs/server.pem /certs/server-key.pem\n  reverse_proxy shared-cache-ui:8081\n}\n"
	return os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddy), 0o644)
}

func sortedSharedBackendApplicationKeys(state sharedBackendState) []string {
	keys := make([]string, 0, len(state.Applications))
	for key := range state.Applications {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
