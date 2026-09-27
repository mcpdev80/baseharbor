package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	PostgresUIHostPortEnv = "BASEHARBOR_POSTGRES_UI_HOST_PORT"
	PostgresUIEmailEnv    = "BASEHARBOR_POSTGRES_UI_EMAIL"
	PostgresUIPasswordEnv = "BASEHARBOR_POSTGRES_UI_PASSWORD"
	CacheUIHostPortEnv    = "BASEHARBOR_CACHE_UI_HOST_PORT"
	CacheUIUserEnv        = "BASEHARBOR_CACHE_UI_USER"
	CacheUIPasswordEnv    = "BASEHARBOR_CACHE_UI_PASSWORD"

	PostgresUIImage = "dpage/pgadmin4:9.18"
	CacheUIImage    = "ghcr.io/joeferner/redis-commander:0.9.1"
	UIProxyImage    = "docker.io/library/caddy:2.11.4-alpine"
)

type ProviderInterfacePurpose string

const (
	ProviderInterfaceApplication    ProviderInterfacePurpose = "application"
	ProviderInterfaceUserFacing     ProviderInterfacePurpose = "user-facing"
	ProviderInterfaceAdministration ProviderInterfacePurpose = "administration"
	ProviderInterfaceManagement     ProviderInterfacePurpose = "management"
	ProviderInterfaceObservability  ProviderInterfacePurpose = "observability"
)

type ManagementUISurface struct {
	Service        string                   `json:"service"`
	Purpose        ProviderInterfacePurpose `json:"purpose"`
	URL            string                   `json:"url"`
	Authentication string                   `json:"authentication"`
}

func EnsureApplicationManagementUIs(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest) error {
	if !m.Services.SQLManagementUI && !m.Services.CacheManagementUI {
		return nil
	}
	if issuer == nil {
		return fmt.Errorf("management UI TLS requires a service issuer")
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	if m.Services.SQLManagementUI {
		if err := ensurePostgresManagementUI(ctx, issuer, files, m, values); err != nil {
			return err
		}
	}
	if m.Services.CacheManagementUI {
		if err := ensureCacheManagementUI(ctx, issuer, files, m, values); err != nil {
			return err
		}
	}
	return nil
}

func ApplicationManagementUISurfaces(m Manifest, files RuntimeFiles) ([]ManagementUISurface, error) {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return nil, err
	}
	var result []ManagementUISurface
	if m.Services.SQLManagementUI {
		port, err := requireRuntimeValue(values, PostgresUIHostPortEnv)
		if err != nil { return nil, err }
		result = append(result, ManagementUISurface{
			Service: "sql", Purpose: ProviderInterfaceManagement,
			URL: "https://127.0.0.1:" + port + "/",
			Authentication: "pgadmin-native",
		})
	}
	if m.Services.CacheManagementUI {
		port, err := requireRuntimeValue(values, CacheUIHostPortEnv)
		if err != nil { return nil, err }
		result = append(result, ManagementUISurface{
			Service: "cache", Purpose: ProviderInterfaceManagement,
			URL: "https://127.0.0.1:" + port + "/",
			Authentication: "http-basic",
		})
	}
	return result, nil
}

func ensurePostgresManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, values map[string]string) error {
	dir := filepath.Join(files.Dir, "providers", "management-ui", "postgres")
	if err := os.MkdirAll(dir, 0o700); err != nil { return err }
	policy, err := serviceaccess.Resolve(m.Environment, "pgadmin", serviceaccess.AuthenticationNative)
	if err != nil { return err }
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", "postgres-ui")
	if err != nil { return err }
	if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.cert")); err != nil { return err }
	if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server.key")); err != nil { return err }

	if err := os.WriteFile(filepath.Join(dir, "password"), []byte(values[PostgresUIPasswordEnv]+"\n"), 0o600); err != nil { return err }
	if err := os.Chmod(filepath.Join(dir, "password"), 0o644); err != nil { return err }

	var pgpass strings.Builder
	servers := map[string]any{"Servers": map[string]any{}}
	serverMap := servers["Servers"].(map[string]any)
	for i, instance := range SQLInstanceNames(m) {
		password, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "PASSWORD"))
		if err != nil { return err }
		db, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "DB"))
		if err != nil { return err }
		user, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "USER"))
		if err != nil { return err }
		host := postgresAccessService(instance)
		fmt.Fprintf(&pgpass, "%s:5432:*:%s:%s\n", host, user, password)
		caSource := values[postgresTLSCAKey(instance)]
		caTarget := filepath.Join(dir, "postgres-"+envInstanceToken(instance)+".ca.pem")
		if err := projectUIReadableFile(caSource, caTarget); err != nil { return err }
		serverMap[strconv.Itoa(i+1)] = map[string]any{
			"Name": "BaseHarbor "+instance,
			"Group": "BaseHarbor",
			"Host": host,
			"Port": 5432,
			"MaintenanceDB": db,
			"Username": user,
			"SSLMode": "verify-ca",
			"PassFile": "/run/baseharbor/pgpass",
			"ConnectionParameters": map[string]any{
				"sslmode": "verify-ca",
				"sslrootcert": "/run/baseharbor/postgres-"+envInstanceToken(instance)+".ca.pem",
				"passfile": "/run/baseharbor/pgpass",
			},
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "pgpass"), []byte(pgpass.String()), 0o600); err != nil { return err }
	if err := os.Chmod(filepath.Join(dir, "pgpass"), 0o644); err != nil { return err }
	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil { return err }
	if err := os.WriteFile(filepath.Join(dir, "servers.json"), append(data, '\n'), 0o644); err != nil { return err }
	return nil
}

func ensureCacheManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, values map[string]string) error {
	dir := filepath.Join(files.Dir, "providers", "management-ui", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil { return err }
	policy, err := serviceaccess.Resolve(m.Environment, "redis-commander", serviceaccess.AuthenticationNative)
	if err != nil { return err }
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", "cache-ui")
	if err != nil { return err }
	if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.pem")); err != nil { return err }
	if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server-key.pem")); err != nil { return err }

	if err := os.WriteFile(filepath.Join(dir, "http-password"), []byte(values[CacheUIPasswordEnv]+"\n"), 0o600); err != nil { return err }
	if err := os.Chmod(filepath.Join(dir, "http-password"), 0o644); err != nil { return err }

	connections := make([]map[string]any, 0, len(CacheInstanceNames(m)))
	for _, instance := range CacheInstanceNames(m) {
		password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
		if err != nil { return err }
		caData, err := os.ReadFile(values[valkeyTLSCAKey(instance)])
		if err != nil { return err }
		connections = append(connections, map[string]any{
			"label": "BaseHarbor "+instance,
			"host": valkeyAccessService(instance),
			"port": 6379,
			"username": "default",
			"password": password,
			"dbIndex": 0,
			"tls": map[string]any{
				"ca": []string{strings.TrimSpace(string(caData))},
				"servername": valkeyAccessService(instance),
			},
		})
	}
	data, err := json.MarshalIndent(map[string]any{"connections": connections}, "", "  ")
	if err != nil { return err }
	if err := os.WriteFile(filepath.Join(dir, "local.json"), append(data, '\n'), 0o600); err != nil { return err }
	if err := os.Chmod(filepath.Join(dir, "local.json"), 0o644); err != nil { return err }

	caddy := ":8443 {\n  tls /certs/server.pem /certs/server-key.pem\n  reverse_proxy cache-ui:8081\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddy), 0o644); err != nil { return err }
	return nil
}

func projectUIReadableFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil { return err }
	if err := os.WriteFile(target, data, 0o600); err != nil { return err }
	return os.Chmod(target, 0o644)
}
