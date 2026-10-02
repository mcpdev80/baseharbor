package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/provideroperation"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const sharedBackendStateVersion = 2

type sharedBackendState struct {
	Version                 int                              `json:"version"`
	Environment             string                           `json:"environment"`
	PostgresAdminCredential       string                           `json:"postgres_admin_credential,omitempty"`
	PostgresReplicationCredential string                           `json:"postgres_replication_credential,omitempty"`
	PostgresHostPort              int                              `json:"postgres_host_port,omitempty"`
	PostgresUIHostPort      int                              `json:"postgres_ui_host_port,omitempty"`
	CacheUIHostPort         int                              `json:"cache_ui_host_port,omitempty"`
	ManagementUsername      string                           `json:"management_username,omitempty"`
	ManagementPassword      string                           `json:"management_password,omitempty"`
	Applications            map[string]sharedBackendAppState `json:"applications"`
}

type sharedBackendAppState struct {
	Application       string                            `json:"application"`
	Environment       string                            `json:"environment"`
	SQLManagementUI   bool                              `json:"sql_management_ui,omitempty"`
	CacheManagementUI bool                              `json:"cache_management_ui,omitempty"`
	SQL               map[string]sharedPostgresResource `json:"sql,omitempty"`
	Cache             map[string]sharedValkeyResource   `json:"cache,omitempty"`
}

type sharedPostgresResource struct {
	Database            string `json:"database"`
	Username            string `json:"username"`
	CredentialReference string `json:"credential_reference"`
}

type SharedPostgresResourceObservation struct {
	Instance        string `json:"instance"`
	Database        string `json:"database"`
	Role            string `json:"role"`
	Owner           string `json:"owner"`
	ProviderScope   string `json:"provider_scope"`
	CredentialScope string `json:"credential_scope"`
}

type sharedValkeyResource struct {
	CredentialReference string `json:"credential_reference"`
	HostPort            int    `json:"host_port"`
	Instances           int    `json:"instances,omitempty"`
}

type SharedBackendFiles struct {
	Dir             string
	Compose         string
	Env             string
	State           string
	Project         string
	ResourceProject string
	Network         string
}

func SharedBackendNetworkName(namespace, environment string) string {
	base := bhruntime.SharedResourceProjectName(namespace)
	env := sharedBackendToken(environment)
	if env == "" {
		env = "dev"
	}
	return base + "-" + env + "-backends"
}

func SharedBackendFilesAt(dataDir, namespace, environment string) SharedBackendFiles {
	env := sharedBackendToken(environment)
	if env == "" {
		env = "dev"
	}
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "shared-backends", env)
	return SharedBackendFiles{
		Dir:             dir,
		Compose:         filepath.Join(dir, "compose.yaml"),
		Env:             filepath.Join(dir, "provider.env"),
		State:           filepath.Join(dir, "state.json"),
		Project:         bhruntime.SharedProjectName(namespace),
		ResourceProject: bhruntime.SharedResourceProjectName(namespace),
		Network:         SharedBackendNetworkName(namespace, environment),
	}
}

func UsesSharedPostgreSQL(m Manifest) bool {
	placement, err := ResolveProviderPlacement(m, capability.ProviderPostgreSQL)
	return err == nil && placement.Scope == capability.ScopeShared && len(SQLInstanceNames(m)) > 0
}

func UsesSharedValkey(m Manifest) bool {
	placement, err := ResolveProviderPlacement(m, capability.ProviderValkey)
	return err == nil && placement.Scope == capability.ScopeShared && len(CacheInstanceNames(m)) > 0
}

func HasSharedBackends(m Manifest) bool {
	return UsesSharedPostgreSQL(m) || UsesSharedValkey(m)
}

func HasApplicationScopedRuntimeServices(m Manifest) bool {
	if len(SQLInstanceNames(m)) > 0 && !UsesSharedPostgreSQL(m) {
		return true
	}
	if len(CacheInstanceNames(m)) > 0 && !UsesSharedValkey(m) {
		return true
	}
	if len(RabbitMQInstanceNames(m)) > 0 {
		return true
	}
	if len(DocumentDatabaseInstanceNames(m)) > 0 {
		return true
	}
	return false
}

func ReconcileSharedBackends(ctx context.Context, compose bhruntime.RuntimeProvider, issuer serviceaccess.Issuer, dataDir, namespace string, m Manifest, files RuntimeFiles) (bool, error) {
	if !HasSharedBackends(m) {
		return false, nil
	}
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	if err := os.MkdirAll(shared.Dir, 0o700); err != nil {
		return false, fmt.Errorf("create shared backend state: %w", err)
	}
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		return false, err
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return false, err
	}
	appKey := sharedBackendApplicationKey(m)
	app := state.Applications[appKey]
	app.Application = m.Name
	app.Environment = m.Environment
	app.SQLManagementUI = m.Services.SQLManagementUI && UsesSharedPostgreSQL(m)
	app.CacheManagementUI = (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && UsesSharedValkey(m)
	if app.SQL == nil {
		app.SQL = map[string]sharedPostgresResource{}
	}
	if app.Cache == nil {
		app.Cache = map[string]sharedValkeyResource{}
	}

	if UsesSharedPostgreSQL(m) {
		if state.PostgresAdminCredential == "" {
			ref, err := ensureSharedPostgresCredential(shared.Dir, "provider-admin", "")
			if err != nil {
				return false, err
			}
			state.PostgresAdminCredential = ref
		}
		if state.PostgresReplicationCredential == "" {
			ref, err := ensureSharedPostgresCredential(shared.Dir, "provider-replication", "")
			if err != nil {
				return false, err
			}
			state.PostgresReplicationCredential = ref
		}
		if state.PostgresHostPort == 0 {
			state.PostgresHostPort, err = allocateLoopbackPort(nil)
			if err != nil {
				return false, err
			}
		}
		for _, instance := range SQLInstanceNames(m) {
			database := sharedPostgresDatabaseName(m, instance)
			username := sharedPostgresRoleName(m, instance)
			ref, err := ensureSharedPostgresCredential(shared.Dir, sharedBackendApplicationKey(m), instance)
			if err != nil {
				return false, err
			}
			password, err := readSharedBackendCredential(shared.Dir, ref)
			if err != nil {
				return false, err
			}
			resource := sharedPostgresResource{
				Database:            database,
				Username:            username,
				CredentialReference: ref,
			}
			app.SQL[instance] = resource
			values[postgresRuntimeKey(instance, "DB")] = database
			values[postgresRuntimeKey(instance, "USER")] = username
			values[postgresRuntimeKey(instance, "PASSWORD")] = password
			values[postgresRuntimeKey(instance, "HOST_PORT")] = strconv.Itoa(state.PostgresHostPort)
			values[postgresContainerHostKey(instance)] = sharedPostgresAlias()
		}
	}

	if UsesSharedValkey(m) {
		for _, instance := range CacheInstanceNames(m) {
			port, convErr := strconv.Atoi(strings.TrimSpace(values[valkeyRuntimeKey(instance, "HOST_PORT")]))
			if convErr != nil || port <= 0 {
				port, err = allocateLoopbackPort(nil)
				if err != nil {
					return false, err
				}
			}
			ref, err := ensureSharedValkeyCredential(shared.Dir, sharedBackendApplicationKey(m), instance)
			if err != nil {
				return false, err
			}
			password, err := readSharedBackendCredential(shared.Dir, ref)
			if err != nil {
				return false, err
			}
			app.Cache[instance] = sharedValkeyResource{
				CredentialReference: ref,
				HostPort:            port,
				Instances:           valkeyMemberCount(m, instance),
			}
			values[valkeyRuntimeKey(instance, "PASSWORD")] = password
			values[valkeyRuntimeKey(instance, "HOST_PORT")] = strconv.Itoa(port)
			values[valkeyContainerHostKey(instance)] = sharedValkeyAccessAlias(m, instance)
		}
	}
	state.Applications[appKey] = app

	if err := ensureSharedManagementUIState(&state, m, values); err != nil {
		return false, err
	}
	if err := ensureSharedBackendTLS(ctx, issuer, shared, m, &state, files, values); err != nil {
		return false, err
	}
	if err := ensureSharedManagementUIs(ctx, issuer, shared, m, state, values); err != nil {
		return false, err
	}
	if err := writeRuntimeEnv(files.Env, m, values); err != nil {
		return false, err
	}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		return false, err
	}
	if err := renderSharedBackendRuntime(shared, state); err != nil {
		return false, err
	}
	if err := compose.ConfigProject(ctx, shared.Project, shared.Compose, shared.Env); err != nil {
		return false, fmt.Errorf("validate shared backend runtime: %w", err)
	}
	if err := compose.UpProject(ctx, shared.Project, shared.Compose, shared.Env); err != nil {
		return false, fmt.Errorf("start shared backend runtime: %w", err)
	}
	if UsesSharedPostgreSQL(m) {
		if err := waitSharedPostgresReady(ctx, compose, shared, m.Environment); err != nil {
			return false, err
		}
		if err := reconcileSharedPostgresApplication(ctx, compose, shared, app); err != nil {
			return false, err
		}
	}
	if UsesSharedValkey(m) {
		if err := waitSharedValkeyReady(ctx, compose, shared, m); err != nil {
			return false, err
		}
	}
	if _, err := EnsureRuntimeContract(m, files); err != nil {
		return false, fmt.Errorf("refresh application contract for shared backends: %w", err)
	}
	return true, nil
}

func SharedPostgresResourcesAt(dataDir, namespace string, m Manifest) ([]SharedPostgresResourceObservation, error) {
	if !UsesSharedPostgreSQL(m) {
		return nil, nil
	}
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		return nil, err
	}
	appKey := sharedBackendApplicationKey(m)
	app, ok := state.Applications[appKey]
	if !ok {
		return nil, fmt.Errorf("shared PostgreSQL application registration is missing")
	}
	if err := verifySharedPostgresStateOwnership(state, appKey); err != nil {
		return nil, err
	}
	instances := make([]string, 0, len(app.SQL))
	for instance := range app.SQL {
		instances = append(instances, instance)
	}
	sort.Strings(instances)
	out := make([]SharedPostgresResourceObservation, 0, len(instances))
	for _, instance := range instances {
		resource := app.SQL[instance]
		out = append(out, SharedPostgresResourceObservation{
			Instance:        instance,
			Database:        resource.Database,
			Role:            resource.Username,
			Owner:           app.Application + "/" + app.Environment,
			ProviderScope:   string(capability.ScopeShared),
			CredentialScope: "application",
		})
	}
	return out, nil
}

func VerifySharedPostgreSQL(ctx context.Context, compose bhruntime.RuntimeProvider, dataDir, namespace string, m Manifest) error {
	if !UsesSharedPostgreSQL(m) {
		return nil
	}
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		return err
	}
	appKey := sharedBackendApplicationKey(m)
	app, ok := state.Applications[appKey]
	if !ok {
		return fmt.Errorf("shared PostgreSQL application registration is missing")
	}
	if err := verifySharedPostgresStateOwnership(state, appKey); err != nil {
		return err
	}
	for instance, resource := range app.SQL {
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared PostgreSQL credential %s: %w", instance, err)
		}
		script := fmt.Sprintf("PGPASSWORD=%s psql -h 127.0.0.1 -U %s -d %s -tAc 'SELECT 1'", shellQuote(password), shellQuote(resource.Username), shellQuote(resource.Database))
		out, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(m.Environment), "sh", "-ec", script)
		if err != nil {
			return fmt.Errorf("verify shared PostgreSQL %s with application credential: %w", instance, err)
		}
		if strings.TrimSpace(out) != "1" {
			return fmt.Errorf("verify shared PostgreSQL %s: unexpected query result %q", instance, strings.TrimSpace(out))
		}
		if err := verifySharedPostgresDatabaseOwnership(ctx, compose, shared, app.Environment, resource); err != nil {
			return fmt.Errorf("verify shared PostgreSQL %s ownership: %w", instance, err)
		}
		adminDBDeny := fmt.Sprintf("PGPASSWORD=%s psql -h 127.0.0.1 -U %s -d postgres -tAc 'SELECT 1'", shellQuote(password), shellQuote(resource.Username))
		if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(m.Environment), "sh", "-ec", adminDBDeny); err == nil {
			return fmt.Errorf("shared PostgreSQL isolation failed: %s/%s can connect to provider administration database postgres", app.Application, instance)
		}
		for otherKey, otherApp := range state.Applications {
			if otherKey == appKey {
				continue
			}
			for otherInstance, other := range otherApp.SQL {
				deny := fmt.Sprintf("PGPASSWORD=%s psql -h 127.0.0.1 -U %s -d %s -tAc 'SELECT 1'", shellQuote(password), shellQuote(resource.Username), shellQuote(other.Database))
				if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(m.Environment), "sh", "-ec", deny); err == nil {
					return fmt.Errorf("shared PostgreSQL isolation failed: %s/%s can connect to %s/%s database %s", app.Application, instance, otherApp.Application, otherInstance, other.Database)
				}
			}
		}
	}
	return nil
}

func verifySharedPostgresStateOwnership(state sharedBackendState, ownerKey string) error {
	app, ok := state.Applications[ownerKey]
	if !ok {
		return fmt.Errorf("shared PostgreSQL owner %q is not registered", ownerKey)
	}
	adminCredential := strings.TrimSpace(state.PostgresAdminCredential)
	if adminCredential == "" {
		return errors.New("shared PostgreSQL provider administrator credential reference is missing")
	}
	seenDB := map[string]string{}
	seenRole := map[string]string{}
	for key, registered := range state.Applications {
		for instance, resource := range registered.SQL {
			resourceKey := key + "/" + instance
			database := strings.TrimSpace(resource.Database)
			username := strings.TrimSpace(resource.Username)
			credential := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(resource.CredentialReference))))
			if database == "" || username == "" || credential == "" || credential == "." {
				return fmt.Errorf("shared PostgreSQL resource %s has incomplete ownership metadata", resourceKey)
			}
			if username == "baseharbor_admin" {
				return fmt.Errorf("shared PostgreSQL resource %s illegally references provider administrator role", resourceKey)
			}
			if database == "postgres" || database == "template0" || database == "template1" {
				return fmt.Errorf("shared PostgreSQL resource %s illegally references provider database %q", resourceKey, database)
			}
			if credential == filepath.ToSlash(filepath.Clean(filepath.FromSlash(adminCredential))) {
				return fmt.Errorf("shared PostgreSQL resource %s illegally references provider administrator credential", resourceKey)
			}
			if !strings.HasPrefix(credential, "credentials/postgres/") || strings.HasPrefix(credential, "../") || filepath.IsAbs(filepath.FromSlash(credential)) {
				return fmt.Errorf("shared PostgreSQL resource %s has invalid application credential reference %q", resourceKey, resource.CredentialReference)
			}
			if len(database) > 63 || len(username) > 63 {
				return fmt.Errorf("shared PostgreSQL resource %s exceeds PostgreSQL identifier limit", resourceKey)
			}
			if previous, exists := seenDB[database]; exists && previous != resourceKey {
				return fmt.Errorf("shared PostgreSQL database %q has ambiguous owners %s and %s", database, previous, resourceKey)
			}
			seenDB[database] = resourceKey
			if previous, exists := seenRole[username]; exists && previous != resourceKey {
				return fmt.Errorf("shared PostgreSQL role %q has ambiguous owners %s and %s", username, previous, resourceKey)
			}
			seenRole[username] = resourceKey
		}
	}
	if len(app.SQL) == 0 {
		return errors.New("shared PostgreSQL owner registration contains no SQL resources")
	}
	return nil
}

type sharedPostgresExecRuntime interface {
	ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error)
}

func verifySharedPostgresDatabaseOwnership(ctx context.Context, compose sharedPostgresExecRuntime, shared SharedBackendFiles, environment string, resource sharedPostgresResource) error {
	query := fmt.Sprintf("SELECT r.rolname FROM pg_database d JOIN pg_roles r ON r.oid=d.datdba WHERE d.datname=%s", quotePostgresLiteral(resource.Database))
	out, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(environment), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-tAc", query)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != resource.Username {
		return fmt.Errorf("database %q owner is %q, expected %q", resource.Database, strings.TrimSpace(out), resource.Username)
	}
	roleQuery := fmt.Sprintf("SELECT r.rolname FROM pg_roles r WHERE r.rolname=%s AND r.rolsuper=false AND r.rolcreatedb=false AND r.rolcreaterole=false AND r.rolreplication=false AND r.rolbypassrls=false AND r.rolinherit=false AND NOT EXISTS (SELECT 1 FROM pg_auth_members am WHERE am.member=r.oid OR am.roleid=r.oid)", quotePostgresLiteral(resource.Username))
	role, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(environment), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-tAc", roleQuery)
	if err != nil {
		return err
	}
	if strings.TrimSpace(role) != resource.Username {
		return fmt.Errorf("application role %q is missing or has elevated privileges", resource.Username)
	}
	return nil
}

func VerifySharedValkey(ctx context.Context, compose bhruntime.RuntimeProvider, dataDir, namespace string, m Manifest) error {
	if !UsesSharedValkey(m) {
		return nil
	}
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		return err
	}
	app, ok := state.Applications[sharedBackendApplicationKey(m)]
	if !ok {
		return fmt.Errorf("shared Valkey application registration is missing")
	}
	op := provideroperation.New(compose, shared.Project, shared.Compose, shared.Env)
	appKey := sharedBackendApplicationKey(m)
	for instance, resource := range app.Cache {
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared Valkey credential %s: %w", instance, err)
		}
		primary := sharedValkeyMemberServiceName(app, instance, 0)
		if sharedValkeyMemberCount(resource) > 1 {
			masterCount := 0
			for ordinal := 0; ordinal < sharedValkeyMemberCount(resource); ordinal++ {
				service := sharedValkeyMemberServiceName(app, instance, ordinal)
				script := "IFS= read -r password; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli -h 127.0.0.1 -p 6379 info replication"
				out, probeErr := op.RunSensitive(ctx, service, []byte(password+"\n"), "sh", "-ec", script)
				if probeErr != nil {
					return fmt.Errorf("inspect shared Valkey HA member %s: %w", service, probeErr)
				}
				role := parseValkeyReplicationRole(out)
				if role == "master" {
					masterCount++
					primary = service
				} else if role != "slave" && role != "replica" {
					return fmt.Errorf("shared Valkey member %s reported unsupported role %q", service, role)
				}
			}
			if masterCount != 1 {
				return fmt.Errorf("shared Valkey %s has %d masters, require exactly one", instance, masterCount)
			}
			for ordinal := 0; ordinal < 3; ordinal++ {
				sentinel := sharedValkeySentinelServiceName(app, instance, ordinal)
				out, sentinelErr := op.Run(ctx, sentinel, "valkey-cli", "-p", "26379", "SENTINEL", "get-master-addr-by-name", valkeySentinelMasterName)
				if sentinelErr != nil {
					return fmt.Errorf("inspect shared Valkey Sentinel %s: %w", sentinel, sentinelErr)
				}
				lines := nonEmptyLines(out)
				if len(lines) < 2 || lines[0] != primary || lines[1] != "6379" {
					return fmt.Errorf("shared Valkey Sentinel %s does not agree on primary %s", sentinel, primary)
				}
			}
		}
		pingScript := "IFS= read -r password; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli -h 127.0.0.1 -p 6379 ping"
		out, err := op.RunSensitive(ctx, primary, []byte(password+"\n"), "sh", "-ec", pingScript)
		if err != nil {
			return fmt.Errorf("verify shared Valkey %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "PONG" {
			return fmt.Errorf("verify shared Valkey %s: unexpected PING result %q", instance, strings.TrimSpace(out))
		}
		for _, durableInstance := range KeyValueInstanceNames(m) {
			if durableInstance != instance {
				continue
			}
			writeRead := "IFS= read -r password; export VALKEYCLI_AUTH=\"$password\"; valkey-cli -h 127.0.0.1 -p 6379 set __baseharbor_verify__ durable >/dev/null && valkey-cli -h 127.0.0.1 -p 6379 get __baseharbor_verify__ && valkey-cli -h 127.0.0.1 -p 6379 del __baseharbor_verify__ >/dev/null"
			value, verifyErr := op.RunSensitive(ctx, primary, []byte(password+"\n"), "sh", "-ec", writeRead)
			if verifyErr != nil {
				return fmt.Errorf("verify durable shared Valkey %s: %w", instance, verifyErr)
			}
			if strings.TrimSpace(value) != "durable" {
				return fmt.Errorf("verify durable shared Valkey %s: unexpected write/read result %q", instance, strings.TrimSpace(value))
			}
		}
		for otherKey, otherApp := range state.Applications {
			if otherKey == appKey {
				continue
			}
			for otherInstance := range otherApp.Cache {
				otherService := sharedValkeyMemberServiceName(otherApp, otherInstance, 0)
				denyScript := "IFS= read -r password; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli -h " + shellQuote(otherService) + " -p 6379 ping"
				if denyOut, denyErr := op.RunSensitive(ctx, primary, []byte(password+"\n"), "sh", "-ec", denyScript); denyErr == nil && strings.TrimSpace(denyOut) == "PONG" {
					return fmt.Errorf("shared Valkey isolation failed: %s/%s can authenticate to %s/%s", app.Application, instance, otherApp.Application, otherInstance)
				}
			}
		}
	}
	return nil
}

func VerifySharedManagementUIChecks(ctx context.Context, dataDir, namespace string, m Manifest) []ManagementUICheckResult {
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		var results []ManagementUICheckResult
		if m.Services.SQLManagementUI && UsesSharedPostgreSQL(m) {
			results = append(results, ManagementUICheckResult{Name: "pgadmin", Err: err})
		}
		if m.Services.CacheManagementUI && UsesSharedValkey(m) {
			results = append(results, ManagementUICheckResult{Name: "redis-commander", Err: err})
		}
		return results
	}
	checks := []struct {
		enabled bool
		name    string
		port    int
		dir     string
		path    string
	}{
		{m.Services.SQLManagementUI && UsesSharedPostgreSQL(m), "pgadmin", state.PostgresUIHostPort, filepath.Join(shared.Dir, "management-ui", "postgres", "pki"), "/misc/ping"},
		{m.Services.CacheManagementUI && UsesSharedValkey(m), "redis-commander", state.CacheUIHostPort, filepath.Join(shared.Dir, "management-ui", "cache", "pki"), "/"},
	}
	var results []ManagementUICheckResult
	for _, check := range checks {
		if !check.enabled {
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		checkErr := func() error {
			if check.port < 1 || check.port > 65535 {
				return fmt.Errorf("%s shared management UI has invalid host port", check.name)
			}
			policy, err := serviceaccess.Resolve(m.Environment, check.name, serviceaccess.AuthenticationNative)
			if err != nil {
				return err
			}
			material, err := serviceaccess.ExistingTLSMaterial(policy, check.dir)
			if err != nil {
				return fmt.Errorf("inspect shared %s management UI TLS: %w", check.name, err)
			}
			client, err := serviceaccess.NewHTTPClient(material, false)
			if err != nil {
				return err
			}
			endpoint, err := serviceaccess.LoopbackHTTPSURL(check.port)
			if err != nil {
				return err
			}
			if err := serviceaccess.WaitHTTPS(checkCtx, client, endpoint, check.path); err != nil {
				return fmt.Errorf("shared %s management UI is not ready: %w", check.name, err)
			}
			return nil
		}()
		cancel()
		results = append(results, ManagementUICheckResult{Name: check.name, Err: checkErr})
	}
	return results
}

func VerifySharedBackends(ctx context.Context, compose bhruntime.RuntimeProvider, dataDir, namespace string, m Manifest) error {
	if err := VerifySharedPostgreSQL(ctx, compose, dataDir, namespace, m); err != nil {
		return err
	}
	return VerifySharedValkey(ctx, compose, dataDir, namespace, m)
}

func DestroyAllSharedBackendsAt(ctx context.Context, compose bhruntime.RuntimeProvider, dataDir, namespace string) error {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "shared-backends")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var result error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		environment := entry.Name()
		files := SharedBackendFilesAt(dataDir, namespace, environment)
		if _, statErr := os.Stat(files.Compose); statErr == nil {
			if err := compose.DestroyProjectRemoveOrphans(ctx, files.Project, files.Compose, files.Env); err != nil {
				result = errors.Join(result, fmt.Errorf("destroy shared backend provider %s: %w", environment, err))
				continue
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			result = errors.Join(result, statErr)
			continue
		}
		if err := os.RemoveAll(files.Dir); err != nil {
			result = errors.Join(result, err)
		}
	}
	if result != nil {
		return result
	}
	return os.RemoveAll(root)
}

func ReleaseSharedBackendApplication(ctx context.Context, compose bhruntime.RuntimeProvider, dataDir, namespace string, m Manifest) error {
	postgresPlacement, postgresFound, err := RegisteredProviderPlacementAt(dataDir, m, capability.ProviderPostgreSQL)
	if err != nil {
		return fmt.Errorf("inspect registered PostgreSQL placement before shared release: %w", err)
	}
	valkeyPlacement, valkeyFound, err := RegisteredProviderPlacementAt(dataDir, m, capability.ProviderValkey)
	if err != nil {
		return fmt.Errorf("inspect registered Valkey placement before shared release: %w", err)
	}
	registeredShared := (postgresFound && postgresPlacement.Scope == capability.ScopeShared) ||
		(valkeyFound && valkeyPlacement.Scope == capability.ScopeShared)
	if !registeredShared {
		return nil
	}

	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("refuse shared backend destroy: protected shared provider state is missing")
	}
	if err != nil {
		return err
	}
	key := sharedBackendApplicationKey(m)
	app, ok := state.Applications[key]
	if !ok {
		return fmt.Errorf("refuse shared backend destroy: registered shared provider application %q is missing from protected provider state", key)
	}
	if len(app.SQL) > 0 {
		placement, found, err := RegisteredProviderPlacementAt(dataDir, m, capability.ProviderPostgreSQL)
		if err != nil {
			return fmt.Errorf("verify shared PostgreSQL provider ownership: %w", err)
		}
		if !found || placement.Scope != capability.ScopeShared || placement.Ownership != capability.OwnershipBaseHarbor {
			return fmt.Errorf("refuse shared PostgreSQL destroy: registered provider ownership is missing or not BaseHarbor-shared")
		}
	}
	if len(app.Cache) > 0 {
		placement, found, err := RegisteredProviderPlacementAt(dataDir, m, capability.ProviderValkey)
		if err != nil {
			return fmt.Errorf("verify shared Valkey provider ownership: %w", err)
		}
		if !found || placement.Scope != capability.ScopeShared || placement.Ownership != capability.OwnershipBaseHarbor {
			return fmt.Errorf("refuse shared Valkey destroy: registered provider ownership is missing or not BaseHarbor-shared")
		}
	}
	if len(app.SQL) > 0 {
		if err := verifySharedPostgresStateOwnership(state, key); err != nil {
			return fmt.Errorf("refuse shared PostgreSQL destroy: %w", err)
		}
		instances := make([]string, 0, len(app.SQL))
		for instance := range app.SQL {
			instances = append(instances, instance)
		}
		sort.Strings(instances)
		for _, instance := range instances {
			resource := app.SQL[instance]
			if err := verifySharedPostgresDatabaseOwnership(ctx, compose, shared, m.Environment, resource); err != nil {
				return fmt.Errorf("refuse shared PostgreSQL destroy for %s: %w", instance, err)
			}
		}
		for _, instance := range instances {
			resource := app.SQL[instance]
			terminate := fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=%s AND pid <> pg_backend_pid()", quotePostgresLiteral(resource.Database))
			if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(m.Environment), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", terminate); err != nil {
				return fmt.Errorf("terminate shared PostgreSQL connections for %s: %w", instance, err)
			}
			dropDB := fmt.Sprintf("DROP DATABASE %s", quotePostgresIdent(resource.Database))
			if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(m.Environment), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", dropDB); err != nil {
				return fmt.Errorf("drop shared PostgreSQL database for %s: %w", instance, err)
			}
			dropRole := fmt.Sprintf("DROP ROLE %s", quotePostgresIdent(resource.Username))
			if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(m.Environment), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", dropRole); err != nil {
				return fmt.Errorf("drop shared PostgreSQL role for %s: %w", instance, err)
			}
			if err := removeSharedBackendCredential(shared.Dir, resource.CredentialReference); err != nil {
				return fmt.Errorf("remove shared PostgreSQL credential for %s: %w", instance, err)
			}
		}
	}
	if err := destroySharedValkeyApplicationRuntime(ctx, compose, shared, app); err != nil {
		return err
	}
	for instance, resource := range app.Cache {
		if err := removeSharedBackendCredential(shared.Dir, resource.CredentialReference); err != nil {
			return fmt.Errorf("remove shared Valkey credential for %s: %w", instance, err)
		}
	}
	delete(state.Applications, key)
	if sharedBackendPostgresUIRequested(state) {
		if err := refreshSharedPostgresManagementUIConfig(shared, state); err != nil {
			return err
		}
	}
	if sharedBackendCacheUIRequested(state) {
		if err := refreshSharedCacheManagementUIConfig(shared, state); err != nil {
			return err
		}
	}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		return err
	}
	if err := renderSharedBackendRuntime(shared, state); err != nil {
		return err
	}
	if len(state.Applications) == 0 && state.PostgresAdminCredential == "" {
		return nil
	}
	return compose.UpProject(ctx, shared.Project, shared.Compose, shared.Env)
}

func destroySharedValkeyApplicationRuntime(ctx context.Context, compose bhruntime.RuntimeProvider, shared SharedBackendFiles, app sharedBackendAppState) error {
	if len(app.Cache) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("services:\n")
	instances := make([]string, 0, len(app.Cache))
	for instance := range app.Cache {
		instances = append(instances, instance)
	}
	sort.Strings(instances)
	for _, instance := range instances {
		writeSharedValkeyCompose(&b, app, instance)
	}
	b.WriteString("volumes:\n")
	for _, instance := range instances {
		service := sharedValkeyServiceFor(app.Application, app.Environment, instance)
		fmt.Fprintf(&b, "  %s-data:\n    name: %s-%s-%s-data\n", service, shared.ResourceProject, sharedBackendToken(app.Environment), service)
	}
	b.WriteString("networks:\n  shared-backend:\n    external: true\n")
	fmt.Fprintf(&b, "    name: %s\n", shared.Network)
	path := filepath.Join(shared.Dir, ".release-"+sharedBackendToken(app.Application)+"-"+sharedBackendToken(app.Environment)+".yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	defer os.Remove(path)
	if err := compose.DestroyProject(ctx, shared.Project, path, shared.Env); err != nil {
		return fmt.Errorf("destroy shared Valkey resources for %s/%s: %w", app.Application, app.Environment, err)
	}
	return nil
}

func sharedBackendPostgresUIRequested(state sharedBackendState) bool {
	for _, app := range state.Applications {
		if app.SQLManagementUI {
			return true
		}
	}
	return false
}

func sharedBackendCacheUIRequested(state sharedBackendState) bool {
	for _, app := range state.Applications {
		if app.CacheManagementUI {
			return true
		}
	}
	return false
}
