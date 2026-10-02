package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"golang.org/x/crypto/bcrypt"
)

const (
	PostgresUIHostPortEnv = "BASEHARBOR_POSTGRES_UI_HOST_PORT"
	PostgresUIEmailEnv    = "BASEHARBOR_POSTGRES_UI_EMAIL"
	PostgresUIPasswordEnv = "BASEHARBOR_POSTGRES_UI_PASSWORD"
	CacheUIHostPortEnv    = "BASEHARBOR_CACHE_UI_HOST_PORT"
	CacheUIUserEnv        = "BASEHARBOR_CACHE_UI_USER"
	CacheUIPasswordEnv    = "BASEHARBOR_CACHE_UI_PASSWORD"
	MongoDBUIUserEnv      = "BASEHARBOR_MONGODB_UI_USER"
	MongoDBUIPasswordEnv  = "BASEHARBOR_MONGODB_UI_PASSWORD"

	PostgresUIImage = "docker.io/dpage/pgadmin4:9.18"
	CacheUIImage    = "ghcr.io/joeferner/redis-commander:0.9.1"
	MongoDBUIImage  = "docker.io/huggingface/mongoku:2.11.3"
	UIProxyImage    = "docker.io/library/caddy:2.11.4-alpine"
)

func rabbitmqUIHostPortKey(instance string) string {
	return rabbitmqRuntimeKey(instance, "UI_HOST_PORT")
}

func rabbitmqUIServiceName(instance string) string {
	return runtimeServiceName("rabbitmq", instance) + "-ui"
}

func rabbitmqUIRouteName(instance string) string {
	if instance == defaultServiceInstance {
		return "rabbitmq"
	}
	return "rabbitmq-" + instance
}

func RabbitMQManagementUIRouteName(instance string) string {
	return rabbitmqUIRouteName(instance)
}

func mongodbUIHostPortKey(instance string) string {
	return mongodbRuntimeKey(instance, "UI_HOST_PORT")
}

func mongodbUIServiceName(instance string) string {
	return runtimeServiceName("mongodb", instance) + "-ui"
}

func mongodbUIAccessServiceName(instance string) string {
	return runtimeServiceName("mongodb", instance) + "-ui-access"
}

func mongodbUIRouteName(instance string) string {
	if instance == defaultServiceInstance {
		return "mongodb"
	}
	return "mongodb-" + instance
}

func MongoDBManagementUIRouteName(instance string) string {
	return mongodbUIRouteName(instance)
}

type ProviderInterfacePurpose string

const (
	ProviderInterfaceApplication    ProviderInterfacePurpose = "application"
	ProviderInterfaceUserFacing     ProviderInterfacePurpose = "user-facing"
	ProviderInterfaceAdministration ProviderInterfacePurpose = "administration"
	ProviderInterfaceManagement     ProviderInterfacePurpose = "management"
	ProviderInterfaceObservability  ProviderInterfacePurpose = "observability"
)

type ManagementUISurface struct {
	Service             string                        `json:"service"`
	Purpose             ProviderInterfacePurpose      `json:"purpose"`
	URL                 string                        `json:"url"`
	Authentication      string                        `json:"authentication"`
	AuthenticationClass ManagementAuthenticationClass `json:"authentication_class"`
	RoleMappings        []ManagementRoleMapping       `json:"role_mappings,omitempty"`
}

func ApplyDevelopmentManagementUICredentials(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return errors.New("development management UI credentials are incomplete")
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	if m.Services.SQLManagementUI {
		values[PostgresUIEmailEnv] = developmentPostgresUIEmail(username)
		values[PostgresUIPasswordEnv] = password
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		values[CacheUIUserEnv] = username
		values[CacheUIPasswordEnv] = password
	}
	if m.Services.DocumentDatabaseManagementUI {
		values[MongoDBUIUserEnv] = username
		values[MongoDBUIPasswordEnv] = password
	}
	if m.Services.MessagingManagementUI {
		for _, instance := range RabbitMQInstanceNames(m) {
			values[rabbitmqRuntimeKey(instance, "ADMIN_USER")] = username
			values[rabbitmqRuntimeKey(instance, "ADMIN_PASSWORD")] = password
		}
	}
	if err := writeRuntimeEnv(files.Env, m, values); err != nil {
		return err
	}
	return EnsureApplicationManagementUIs(ctx, issuer, files, m)
}

func developmentPostgresUIEmail(username string) string {
	username = strings.TrimSpace(username)
	if strings.Contains(username, "@") {
		return username
	}
	return username + "@baseharbor.dev"
}

func EnsureApplicationManagementUIs(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest) error {
	if !m.Services.SQLManagementUI && !m.Services.CacheManagementUI && !m.Services.KeyValueManagementUI && !m.Services.MessagingManagementUI && !m.Services.DocumentDatabaseManagementUI {
		return nil
	}
	if issuer == nil {
		return fmt.Errorf("management UI TLS requires a service issuer")
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	if m.Services.SQLManagementUI && !UsesSharedPostgreSQL(m) {
		if err := ensurePostgresManagementUI(ctx, issuer, files, m, values); err != nil {
			return err
		}
	}
	if (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && !UsesSharedValkey(m) {
		if err := ensureCacheManagementUI(ctx, issuer, files, m, values); err != nil {
			return err
		}
	}
	if m.Services.MessagingManagementUI {
		if err := ensureRabbitMQManagementUI(ctx, issuer, files, m); err != nil {
			return err
		}
	}
	if m.Services.DocumentDatabaseManagementUI {
		if err := ensureMongoDBManagementUI(ctx, issuer, files, m, values); err != nil {
			return err
		}
	}
	return nil
}

type ManagementUICheckResult struct {
	Name string
	Err  error
}

func VerifyApplicationManagementUIChecks(ctx context.Context, m Manifest, files RuntimeFiles) []ManagementUICheckResult {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		var results []ManagementUICheckResult
		if m.Services.SQLManagementUI {
			results = append(results, ManagementUICheckResult{Name: "pgadmin", Err: err})
		}
		if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
			results = append(results, ManagementUICheckResult{Name: "redis-commander", Err: err})
		}
		if m.Services.MessagingManagementUI {
			for _, instance := range RabbitMQInstanceNames(m) {
				results = append(results, ManagementUICheckResult{Name: rabbitmqUIRouteName(instance), Err: err})
			}
		}
		if m.Services.DocumentDatabaseManagementUI {
			for _, instance := range DocumentDatabaseInstanceNames(m) {
				results = append(results, ManagementUICheckResult{Name: mongodbUIRouteName(instance), Err: err})
			}
		}
		return results
	}

	checks := []struct {
		enabled   bool
		name      string
		portKey   string
		dir       string
		path      string
		basicAuth bool
	}{
		{
			enabled: m.Services.SQLManagementUI && !UsesSharedPostgreSQL(m),
			name:    "pgadmin",
			portKey: PostgresUIHostPortEnv,
			dir:     filepath.Join(files.Dir, "providers", "management-ui", "postgres", "pki"),
			path:    "/misc/ping",
		},
		{
			enabled: (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && !UsesSharedValkey(m),
			name:    "redis-commander",
			portKey: CacheUIHostPortEnv,
			dir:     filepath.Join(files.Dir, "providers", "management-ui", "cache", "pki"),
			path:    "/",
		},
	}
	if m.Services.MessagingManagementUI {
		for _, instance := range RabbitMQInstanceNames(m) {
			checks = append(checks, struct {
				enabled   bool
				name      string
				portKey   string
				dir       string
				path      string
				basicAuth bool
			}{
				enabled: true,
				name:    rabbitmqUIRouteName(instance),
				portKey: rabbitmqUIHostPortKey(instance),
				dir:     filepath.Join(files.Dir, "providers", "management-ui", "rabbitmq", instance, "pki"),
				path:    "/",
			})
		}
	}

	if m.Services.DocumentDatabaseManagementUI {
		for _, instance := range DocumentDatabaseInstanceNames(m) {
			checks = append(checks, struct {
				enabled   bool
				name      string
				portKey   string
				dir       string
				path      string
				basicAuth bool
			}{
				enabled:   true,
				name:      mongodbUIRouteName(instance),
				portKey:   mongodbUIHostPortKey(instance),
				dir:       filepath.Join(files.Dir, "providers", "management-ui", "mongodb", instance, "pki"),
				path:      "/servers",
				basicAuth: true,
			})
		}
	}

	results := make([]ManagementUICheckResult, 0, len(checks))
	for _, check := range checks {
		if !check.enabled {
			continue
		}

		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		checkErr := func() error {
			portValue, err := requireRuntimeValue(values, check.portKey)
			if err != nil {
				return err
			}
			port, err := strconv.Atoi(portValue)
			if err != nil {
				return fmt.Errorf("%s management UI has invalid host port: %w", check.name, err)
			}
			policy, err := serviceaccess.Resolve(m.Environment, check.name, serviceaccess.AuthenticationNative)
			if err != nil {
				return err
			}
			material, err := serviceaccess.ExistingTLSMaterial(policy, check.dir)
			if err != nil {
				return fmt.Errorf("inspect %s management UI TLS: %w", check.name, err)
			}
			var client *http.Client
			if check.basicAuth {
				client, err = serviceaccess.NewHTTPClientWithBasicAuth(material, values[MongoDBUIUserEnv], values[MongoDBUIPasswordEnv])
			} else {
				client, err = serviceaccess.NewHTTPClient(material, false)
			}
			if err != nil {
				return err
			}
			endpoint, err := serviceaccess.LoopbackHTTPSURL(port)
			if err != nil {
				return err
			}
			if err := serviceaccess.WaitHTTPS(checkCtx, client, endpoint, check.path); err != nil {
				return fmt.Errorf("%s management UI is not ready: %w", check.name, err)
			}
			return nil
		}()
		cancel()

		results = append(results, ManagementUICheckResult{Name: check.name, Err: checkErr})
	}
	return results
}

func VerifyApplicationManagementUIs(ctx context.Context, m Manifest, files RuntimeFiles) error {
	var errs []error
	for _, result := range VerifyApplicationManagementUIChecks(ctx, m, files) {
		if result.Err != nil {
			errs = append(errs, result.Err)
		}
	}
	return errors.Join(errs...)
}

func ApplicationManagementUISurfaces(m Manifest, files RuntimeFiles) ([]ManagementUISurface, error) {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return nil, err
	}
	var result []ManagementUISurface
	if m.Services.SQLManagementUI {
		port, err := requireRuntimeValue(values, PostgresUIHostPortEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, ManagementUISurface{
			Service: "sql", Purpose: ProviderInterfaceManagement,
			URL:                 "https://127.0.0.1:" + port + "/",
			Authentication:      "pgadmin-native",
			AuthenticationClass: ManagementAuthNativeCredential,
			RoleMappings:        NativeCredentialRoleMappings("administrator", "user"),
		})
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		port, err := requireRuntimeValue(values, CacheUIHostPortEnv)
		if err != nil {
			return nil, err
		}
		service := "cache"
		if m.Services.KeyValueManagementUI && !m.Services.CacheManagementUI {
			service = "key-value"
		}
		result = append(result, ManagementUISurface{
			Service: service, Purpose: ProviderInterfaceManagement,
			URL:                 "https://127.0.0.1:" + port + "/",
			Authentication:      "http-basic",
			AuthenticationClass: ManagementAuthStandardsAdapter,
			RoleMappings:        NativeCredentialRoleMappings("developer", "developer"),
		})
	}
	if m.Services.MessagingManagementUI {
		for _, instance := range RabbitMQInstanceNames(m) {
			port, err := requireRuntimeValue(values, rabbitmqUIHostPortKey(instance))
			if err != nil {
				return nil, err
			}
			result = append(result, ManagementUISurface{
				Service: rabbitmqUIRouteName(instance), Purpose: ProviderInterfaceManagement,
				URL:                 "https://127.0.0.1:" + port + "/",
				Authentication:      "rabbitmq-native",
				AuthenticationClass: ManagementAuthNativeCredential,
				RoleMappings:        NativeCredentialRoleMappings("administrator", "management"),
			})
		}
	}
	if m.Services.DocumentDatabaseManagementUI {
		for _, instance := range DocumentDatabaseInstanceNames(m) {
			port, err := requireRuntimeValue(values, mongodbUIHostPortKey(instance))
			if err != nil {
				return nil, err
			}
			result = append(result, ManagementUISurface{
				Service: mongodbUIRouteName(instance), Purpose: ProviderInterfaceManagement,
				URL:                 "https://127.0.0.1:" + port + "/",
				Authentication:      "http-basic",
				AuthenticationClass: ManagementAuthStandardsAdapter,
				RoleMappings:        NativeCredentialRoleMappings("developer", "developer"),
			})
		}
	}
	return result, nil
}

func ensurePostgresManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, values map[string]string) error {
	dir := filepath.Join(files.Dir, "providers", "management-ui", "postgres")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve(m.Environment, "pgadmin", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", "postgres-ui")
	if err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.cert")); err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server.key")); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "password"), []byte(values[PostgresUIPasswordEnv]+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(dir, "password"), 0o644); err != nil {
		return err
	}

	var pgpass strings.Builder
	servers := map[string]any{"Servers": map[string]any{}}
	serverMap := servers["Servers"].(map[string]any)
	for i, instance := range SQLInstanceNames(m) {
		password, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return err
		}
		db, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "DB"))
		if err != nil {
			return err
		}
		user, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "USER"))
		if err != nil {
			return err
		}
		host := postgresAccessService(instance)
		fmt.Fprintf(&pgpass, "%s:5432:*:%s:%s\n", host, user, password)
		caSource := values[postgresTLSCAKey(instance)]
		caTarget := filepath.Join(dir, "postgres-"+envInstanceToken(instance)+".ca.pem")
		if err := projectUIReadableFile(caSource, caTarget); err != nil {
			return err
		}
		serverMap[strconv.Itoa(i+1)] = map[string]any{
			"Name":          "BaseHarbor " + instance,
			"Group":         "BaseHarbor",
			"Host":          host,
			"Port":          5432,
			"MaintenanceDB": db,
			"Username":      user,
			"SSLMode":       "verify-ca",
			"PassFile":      "/run/baseharbor/pgpass",
			"ConnectionParameters": map[string]any{
				"sslmode":     "verify-ca",
				"sslrootcert": "/run/baseharbor/postgres-" + envInstanceToken(instance) + ".ca.pem",
				"passfile":    "/run/baseharbor/pgpass",
			},
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "pgpass"), []byte(pgpass.String()), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(dir, "pgpass"), 0o644); err != nil {
		return err
	}
	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "servers.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	return nil
}

func ensureCacheManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, values map[string]string) error {
	dir := filepath.Join(files.Dir, "providers", "management-ui", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve(m.Environment, "redis-commander", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", "cache-ui")
	if err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.pem")); err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server-key.pem")); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "http-password"), []byte(values[CacheUIPasswordEnv]+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(dir, "http-password"), 0o644); err != nil {
		return err
	}

	connections := make([]map[string]any, 0, len(ValkeyInstanceNames(m)))
	for _, instance := range ValkeyInstanceNames(m) {
		password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return err
		}
		caData, err := os.ReadFile(values[valkeyTLSCAKey(instance)])
		if err != nil {
			return err
		}
		connections = append(connections, map[string]any{
			"label":    "BaseHarbor " + instance,
			"host":     valkeyAccessService(instance),
			"port":     6379,
			"username": "default",
			"password": password,
			"dbIndex":  0,
			"tls": map[string]any{
				"ca":         []string{strings.TrimSpace(string(caData))},
				"servername": valkeyAccessService(instance),
			},
		})
	}
	data, err := json.MarshalIndent(map[string]any{"connections": connections}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "local.json"), append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(dir, "local.json"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "local-production.json"), []byte("{}\n"), 0o644); err != nil {
		return err
	}

	caddy := "{\n  auto_https disable_redirects\n}\n\n:8443 {\n  tls /certs/server.pem /certs/server-key.pem\n  reverse_proxy cache-ui:8081\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddy), 0o644); err != nil {
		return err
	}
	return nil
}

func ensureRabbitMQManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest) error {
	for _, instance := range RabbitMQInstanceNames(m) {
		dir := filepath.Join(files.Dir, "providers", "management-ui", "rabbitmq", instance)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		policy, err := serviceaccess.Resolve(m.Environment, "rabbitmq-management", serviceaccess.AuthenticationNative)
		if err != nil {
			return err
		}
		material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", rabbitmqUIServiceName(instance))
		if err != nil {
			return err
		}
		if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.pem")); err != nil {
			return err
		}
		if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server-key.pem")); err != nil {
			return err
		}
		var upstreams []string
		for ordinal := 0; ordinal < rabbitmqMemberCount(m); ordinal++ {
			upstreams = append(upstreams, rabbitmqMemberServiceName(instance, ordinal)+":15672")
		}
		caddy := fmt.Sprintf("{\n  auto_https disable_redirects\n}\n\n:8443 {\n  tls /certs/server.pem /certs/server-key.pem\n  reverse_proxy %s\n}\n", strings.Join(upstreams, " "))
		if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddy), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ensureMongoDBManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, values map[string]string) error {
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		dir := filepath.Join(files.Dir, "providers", "management-ui", "mongodb", instance)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		policy, err := serviceaccess.Resolve(m.Environment, "mongodb-management", serviceaccess.AuthenticationNative)
		if err != nil {
			return err
		}
		material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", mongodbUIAccessServiceName(instance))
		if err != nil {
			return err
		}
		if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.pem")); err != nil {
			return err
		}
		if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server-key.pem")); err != nil {
			return err
		}
		user, err := requireRuntimeValue(values, MongoDBUIUserEnv)
		if err != nil {
			return err
		}
		password, err := requireRuntimeValue(values, MongoDBUIPasswordEnv)
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash MongoDB management UI password: %w", err)
		}
		caddy := fmt.Sprintf("{\n  auto_https disable_redirects\n}\n\n:8443 {\n  tls /certs/server.pem /certs/server-key.pem\n  handle /healthz {\n    respond \"ok\" 200\n  }\n  handle {\n    basic_auth {\n      %s %s\n    }\n    reverse_proxy %s:3100\n  }\n}\n", user, string(hash), mongodbUIServiceName(instance))
		if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddy), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func projectUIReadableFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(target, 0o644)
}
