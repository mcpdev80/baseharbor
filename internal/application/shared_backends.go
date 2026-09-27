package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const sharedBackendStateVersion = 2

type sharedBackendState struct {
	Version                 int                              `json:"version"`
	Environment             string                           `json:"environment"`
	PostgresAdminCredential string                           `json:"postgres_admin_credential,omitempty"`
	PostgresHostPort        int                              `json:"postgres_host_port,omitempty"`
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
	return false
}

func ReconcileSharedBackends(ctx context.Context, compose bhruntime.Compose, issuer serviceaccess.Issuer, dataDir, namespace string, m Manifest, files RuntimeFiles) (bool, error) {
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
	app.CacheManagementUI = m.Services.CacheManagementUI && UsesSharedValkey(m)
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
			app.Cache[instance] = sharedValkeyResource{CredentialReference: ref, HostPort: port}
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

func VerifySharedPostgreSQL(ctx context.Context, compose bhruntime.Compose, dataDir, namespace string, m Manifest) error {
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

func VerifySharedValkey(ctx context.Context, compose bhruntime.Compose, dataDir, namespace string, m Manifest) error {
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
	appKey := sharedBackendApplicationKey(m)
	for instance, resource := range app.Cache {
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared Valkey credential %s: %w", instance, err)
		}
		service := sharedValkeyService(m, instance)
		script := fmt.Sprintf("VALKEYCLI_AUTH=%s valkey-cli -h 127.0.0.1 -p 6379 ping", shellQuote(password))
		out, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, service, "sh", "-ec", script)
		if err != nil {
			return fmt.Errorf("verify shared Valkey %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "PONG" {
			return fmt.Errorf("verify shared Valkey %s: unexpected PING result %q", instance, strings.TrimSpace(out))
		}
		for otherKey, otherApp := range state.Applications {
			if otherKey == appKey {
				continue
			}
			for otherInstance := range otherApp.Cache {
				otherService := sharedValkeyServiceFor(otherApp.Application, otherApp.Environment, otherInstance)
				deny := fmt.Sprintf("VALKEYCLI_AUTH=%s valkey-cli -h %s -p 6379 ping", shellQuote(password), shellQuote(otherService))
				if denyOut, denyErr := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, service, "sh", "-ec", deny); denyErr == nil && strings.TrimSpace(denyOut) == "PONG" {
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

func VerifySharedBackends(ctx context.Context, compose bhruntime.Compose, dataDir, namespace string, m Manifest) error {
	if err := VerifySharedPostgreSQL(ctx, compose, dataDir, namespace, m); err != nil {
		return err
	}
	return VerifySharedValkey(ctx, compose, dataDir, namespace, m)
}

func DestroyAllSharedBackendsAt(ctx context.Context, compose bhruntime.Compose, dataDir, namespace string) error {
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
			if err := compose.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
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

func ReleaseSharedBackendApplication(ctx context.Context, compose bhruntime.Compose, dataDir, namespace string, m Manifest) error {
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
	if len(state.Applications) == 0 {
		if err := compose.DestroyProject(ctx, shared.Project, shared.Compose, shared.Env); err != nil {
			return err
		}
		return os.RemoveAll(shared.Dir)
	}
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
	return compose.UpProject(ctx, shared.Project, shared.Compose, shared.Env)
}

func destroySharedValkeyApplicationRuntime(ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, app sharedBackendAppState) error {
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
		for _, instance := range CacheInstanceNames(m) {
			root := filepath.Join(shared.Dir, "valkey", sharedBackendToken(m.Name), sharedBackendToken(instance))
			policy, err := serviceaccess.Resolve(m.Environment, "valkey", serviceaccess.AuthenticationNative)
			if err != nil {
				return err
			}
			_, err = serviceaccess.EnsureTCPGateway(ctx, issuer, policy, root, serviceaccess.TCPGatewaySpec{
				ServiceName:      sharedValkeyAccessService(m, instance),
				UpstreamHost:     sharedValkeyService(m, instance),
				UpstreamPort:     6379,
				PublishedPortEnv: sharedValkeyPortEnv(m, instance),
				ContainerPort:    6379,
				Network:          "shared-backend",
			})
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
	if m.Services.CacheManagementUI && UsesSharedValkey(m) {
		if state.CacheUIHostPort == 0 {
			port, err := allocateLoopbackPort(nil)
			if err != nil {
				return err
			}
			state.CacheUIHostPort = port
		}
		values[CacheUIHostPortEnv] = strconv.Itoa(state.CacheUIHostPort)
	}
	if (m.Services.SQLManagementUI && UsesSharedPostgreSQL(m)) || (m.Services.CacheManagementUI && UsesSharedValkey(m)) {
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
	if m.Services.CacheManagementUI && UsesSharedValkey(m) {
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

func waitSharedValkeyReady(ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, m Manifest) error {
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for _, instance := range CacheInstanceNames(m) {
		app := sharedBackendApplicationKey(m)
		state, err := loadSharedBackendState(shared.State, m.Environment)
		if err != nil {
			return err
		}
		resource := state.Applications[app].Cache[instance]
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared Valkey credential %s: %w", instance, err)
		}
		service := sharedValkeyService(m, instance)
		ticker := time.NewTicker(500 * time.Millisecond)
		var lastErr error
		ready := false
		for !ready {
			script := fmt.Sprintf("VALKEYCLI_AUTH=%s valkey-cli -h 127.0.0.1 -p 6379 ping", shellQuote(password))
			out, execErr := compose.ExecProject(waitCtx, shared.Project, shared.Compose, shared.Env, service, "sh", "-ec", script)
			if execErr == nil && strings.TrimSpace(out) == "PONG" {
				ready = true
				break
			}
			if execErr != nil {
				lastErr = execErr
			} else {
				lastErr = fmt.Errorf("unexpected readiness result %q", strings.TrimSpace(out))
			}
			select {
			case <-waitCtx.Done():
				ticker.Stop()
				if lastErr != nil {
					return fmt.Errorf("wait for shared Valkey %s readiness: %w", instance, lastErr)
				}
				return fmt.Errorf("wait for shared Valkey %s readiness: %w", instance, waitCtx.Err())
			case <-ticker.C:
			}
		}
		ticker.Stop()
	}
	return nil
}

func waitSharedPostgresReady(ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, environment string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		out, err := compose.ExecProject(
			waitCtx,
			shared.Project,
			shared.Compose,
			shared.Env,
			sharedPostgresService(environment),
			"pg_isready",
			"-h", "127.0.0.1",
			"-p", "5432",
			"-U", "baseharbor_admin",
			"-d", "postgres",
		)
		if err == nil && strings.Contains(strings.ToLower(strings.TrimSpace(out)), "accepting connections") {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("unexpected readiness result %q", strings.TrimSpace(out))
		}

		select {
		case <-waitCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("wait for shared PostgreSQL readiness: %w", lastErr)
			}
			return fmt.Errorf("wait for shared PostgreSQL readiness: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func reconcileSharedPostgresApplication(ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, app sharedBackendAppState) error {
	instances := make([]string, 0, len(app.SQL))
	for instance := range app.SQL {
		instances = append(instances, instance)
	}
	sort.Strings(instances)
	for _, instance := range instances {
		resource := app.SQL[instance]
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared PostgreSQL credential %s: %w", instance, err)
		}
		sql := fmt.Sprintf("REVOKE ALL ON DATABASE postgres FROM PUBLIC;\nREVOKE ALL ON DATABASE template1 FROM PUBLIC;\nSELECT format('CREATE ROLE %%I LOGIN PASSWORD %%L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS', %s, %s) WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = %s)\\gexec\nALTER ROLE %s WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;\nSELECT format('CREATE DATABASE %%I OWNER %%I', %s, %s) WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = %s)\\gexec\nALTER DATABASE %s OWNER TO %s;\nREVOKE ALL ON DATABASE %s FROM PUBLIC;\nGRANT CONNECT, TEMPORARY ON DATABASE %s TO %s;\n",
			quotePostgresLiteral(resource.Username), quotePostgresLiteral(password), quotePostgresLiteral(resource.Username),
			quotePostgresIdent(resource.Username), quotePostgresLiteral(password),
			quotePostgresLiteral(resource.Database), quotePostgresLiteral(resource.Username), quotePostgresLiteral(resource.Database),
			quotePostgresIdent(resource.Database), quotePostgresIdent(resource.Username),
			quotePostgresIdent(resource.Database),
			quotePostgresIdent(resource.Database), quotePostgresIdent(resource.Username),
		)
		if _, err := compose.ExecProjectInput(ctx, shared.Project, shared.Compose, shared.Env, []byte(sql), sharedPostgresService(app.Environment), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-v", "ON_ERROR_STOP=1"); err != nil {
			return fmt.Errorf("reconcile shared PostgreSQL resource %s: %w", instance, err)
		}
		harden := fmt.Sprintf("REVOKE ALL ON SCHEMA public FROM PUBLIC; GRANT USAGE, CREATE ON SCHEMA public TO %s; REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC; REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC; REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON TABLES FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON SEQUENCES FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON FUNCTIONS FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON TYPES FROM PUBLIC;",
			quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username))
		if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(app.Environment), "psql", "-U", "baseharbor_admin", "-d", resource.Database, "-v", "ON_ERROR_STOP=1", "-c", harden); err != nil {
			return fmt.Errorf("harden shared PostgreSQL resource %s: %w", instance, err)
		}
	}
	return nil
}

func renderSharedBackendRuntime(files SharedBackendFiles, state sharedBackendState) error {
	var env strings.Builder
	if state.PostgresAdminCredential != "" {
		password, err := readSharedBackendCredential(files.Dir, state.PostgresAdminCredential)
		if err != nil {
			return err
		}
		fmt.Fprintf(&env, "SHARED_POSTGRES_ADMIN_PASSWORD=%s\n", password)
	}
	if state.PostgresHostPort > 0 {
		fmt.Fprintf(&env, "SHARED_POSTGRES_HOST_PORT=%d\n", state.PostgresHostPort)
	}
	if state.PostgresUIHostPort > 0 {
		fmt.Fprintf(&env, "SHARED_POSTGRES_UI_HOST_PORT=%d\n", state.PostgresUIHostPort)
	}
	if state.CacheUIHostPort > 0 {
		fmt.Fprintf(&env, "SHARED_CACHE_UI_HOST_PORT=%d\n", state.CacheUIHostPort)
	}
	if state.ManagementUsername != "" {
		fmt.Fprintf(&env, "SHARED_MANAGEMENT_USER=%s\n", state.ManagementUsername)
		fmt.Fprintf(&env, "SHARED_PGADMIN_EMAIL=%s\n", developmentPostgresUIEmail(state.ManagementUsername))
	}
	if state.ManagementPassword != "" {
		fmt.Fprintf(&env, "SHARED_MANAGEMENT_PASSWORD=%s\n", state.ManagementPassword)
	}
	appKeys := make([]string, 0, len(state.Applications))
	for key := range state.Applications {
		appKeys = append(appKeys, key)
	}
	sort.Strings(appKeys)
	for _, key := range appKeys {
		app := state.Applications[key]
		for instance, resource := range app.Cache {
			password, err := readSharedBackendCredential(files.Dir, resource.CredentialReference)
			if err != nil {
				return err
			}
			fmt.Fprintf(&env, "%s=%d\n", sharedValkeyPortEnvFor(app.Application, app.Environment, instance), resource.HostPort)
			fmt.Fprintf(&env, "%s=%s\n", sharedValkeyPasswordEnvFor(app.Application, app.Environment, instance), password)
		}
	}
	if err := writeOwnerOnlyFile(files.Env, []byte(env.String())); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("services:\n")
	hasPostgres := false
	for _, app := range state.Applications {
		if len(app.SQL) > 0 {
			hasPostgres = true
			break
		}
	}
	if hasPostgres {
		writeSharedPostgresCompose(&b, state)
	}
	if sharedBackendPostgresUIRequested(state) {
		writeSharedPostgresUICompose(&b)
	}
	if sharedBackendCacheUIRequested(state) {
		writeSharedCacheUICompose(&b)
	}
	for _, key := range appKeys {
		app := state.Applications[key]
		instances := make([]string, 0, len(app.Cache))
		for instance := range app.Cache {
			instances = append(instances, instance)
		}
		sort.Strings(instances)
		for _, instance := range instances {
			writeSharedValkeyCompose(&b, app, instance)
		}
	}
	b.WriteString("volumes:\n")
	if hasPostgres {
		fmt.Fprintf(&b, "  shared-postgres-data:\n    name: %s-%s-postgres-data\n", files.ResourceProject, sharedBackendToken(state.Environment))
	}
	for _, key := range appKeys {
		app := state.Applications[key]
		for instance := range app.Cache {
			service := sharedValkeyServiceFor(app.Application, app.Environment, instance)
			fmt.Fprintf(&b, "  %s-data:\n    name: %s-%s-%s-data\n", service, files.ResourceProject, sharedBackendToken(state.Environment), service)
		}
	}
	b.WriteString("networks:\n  shared-backend:\n")
	fmt.Fprintf(&b, "    name: %s\n", files.Network)
	return os.WriteFile(files.Compose, []byte(b.String()), 0o600)
}

func writeSharedPostgresUICompose(b *strings.Builder) {
	b.WriteString(`  shared-pgadmin:
    image: ` + PostgresUIImage + `
    restart: unless-stopped
    user: "5050:5050"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      PGADMIN_DEFAULT_EMAIL: ${SHARED_PGADMIN_EMAIL}
      PGADMIN_DEFAULT_PASSWORD_FILE: /run/baseharbor/password
      PGADMIN_ENABLE_TLS: "True"
      PGADMIN_LISTEN_PORT: "8443"
      PGADMIN_SERVER_JSON_FILE: /run/baseharbor/servers.json
      PGADMIN_REPLACE_SERVERS_ON_STARTUP: "True"
      PGADMIN_DISABLE_POSTFIX: "True"
      PGADMIN_CUSTOM_CONFIG_DISTRO_FILE: /var/lib/pgadmin/config_distro.py
      PGADMIN_CONFIG_MASTER_PASSWORD_REQUIRED: "False"
      PGPASS_FILE: /run/baseharbor/pgpass
    ports:
      - "127.0.0.1:${SHARED_POSTGRES_UI_HOST_PORT}:8443"
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /var/lib/pgadmin:rw,noexec,nosuid,nodev,mode=1777
    volumes:
      - ./management-ui/postgres/password:/run/baseharbor/password:ro
      - ./management-ui/postgres/servers.json:/run/baseharbor/servers.json:ro
      - ./management-ui/postgres/pgpass:/run/baseharbor/pgpass:ro
      - ./management-ui/postgres/postgres.ca.pem:/run/baseharbor/postgres.ca.pem:ro
      - ./management-ui/postgres/server.cert:/certs/server.cert:ro
      - ./management-ui/postgres/server.key:/certs/server.key:ro
    networks:
      shared-backend:
        aliases:
          - shared-pgadmin

`)
}

func writeSharedCacheUICompose(b *strings.Builder) {
	b.WriteString(`  shared-cache-ui:
    image: ` + CacheUIImage + `
    restart: unless-stopped
    user: "redis"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      HTTP_USER: ${SHARED_MANAGEMENT_USER}
      HTTP_PASSWORD_FILE: /run/baseharbor/http-password
      NOSAVE: "true"
      NO_LOG_DATA: "true"
      NODE_ENV: production
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    volumes:
      - ./management-ui/cache/http-password:/run/baseharbor/http-password:ro
      - ./management-ui/cache/local.json:/redis-commander/config/local.json:ro
      - ./management-ui/cache/local-production.json:/redis-commander/config/local-production.json:ro
    networks:
      shared-backend: {}

  shared-cache-ui-access:
    image: ` + UIProxyImage + `
    restart: unless-stopped
    user: "65532:65532"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    entrypoint: ["/bin/sh", "-ec"]
    command:
      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /run/baseharbor:rw,exec,nosuid,nodev,mode=1777
      - /data:rw,noexec,nosuid,nodev,mode=1777
      - /config:rw,noexec,nosuid,nodev,mode=1777
    ports:
      - "127.0.0.1:${SHARED_CACHE_UI_HOST_PORT}:8443"
    volumes:
      - ./management-ui/cache/Caddyfile:/etc/caddy/Caddyfile:ro
      - ./management-ui/cache/server.pem:/certs/server.pem:ro
      - ./management-ui/cache/server-key.pem:/certs/server-key.pem:ro
    networks:
      shared-backend:
        aliases:
          - shared-cache-ui-access

`)
}

func writeSharedPostgresCompose(b *strings.Builder, state sharedBackendState) {
	service := sharedPostgresService(state.Environment)
	root := "./postgresql/runtime"
	fmt.Fprintf(b, `  %s:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    user: "postgres"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    entrypoint: ["/bin/sh", "-ec"]
    command:
      - |
        cp /run/baseharbor/tls-source/server-key.pem /tmp/server-key.pem
        chmod 0600 /tmp/server-key.pem
        exec /usr/local/bin/docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/run/baseharbor/tls-source/server-cert.pem -c ssl_key_file=/tmp/server-key.pem -c hba_file=/run/baseharbor/tls-source/pg_hba.conf
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /var/run/postgresql:rw,noexec,nosuid,nodev
    environment:
      POSTGRES_DB: postgres
      POSTGRES_USER: baseharbor_admin
      POSTGRES_PASSWORD: ${SHARED_POSTGRES_ADMIN_PASSWORD}
    ports:
      - "127.0.0.1:${SHARED_POSTGRES_HOST_PORT}:5432"
    volumes:
      - shared-postgres-data:/var/lib/postgresql
      - %s/server-cert.pem:/run/baseharbor/tls-source/server-cert.pem:ro
      - %s/server-key.pem:/run/baseharbor/tls-source/server-key.pem:ro
      - %s/pg_hba.conf:/run/baseharbor/tls-source/pg_hba.conf:ro
    networks:
      shared-backend:
        aliases:
          - %s
    healthcheck:
      test: ["CMD", "pg_isready", "-h", "127.0.0.1", "-p", "5432", "-U", "baseharbor_admin", "-d", "postgres"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s

`, service, root, root, root, sharedPostgresAlias())
}

func writeSharedValkeyCompose(b *strings.Builder, app sharedBackendAppState, instance string) {
	service := sharedValkeyServiceFor(app.Application, app.Environment, instance)
	access := sharedValkeyAccessServiceFor(app.Application, app.Environment, instance)
	passwordEnv := sharedValkeyPasswordEnvFor(app.Application, app.Environment, instance)
	portEnv := sharedValkeyPortEnvFor(app.Application, app.Environment, instance)
	root := "./" + filepath.ToSlash(filepath.Join("valkey", sharedBackendToken(app.Application), sharedBackendToken(instance)))
	fmt.Fprintf(b, `  %s:
    image: docker.io/valkey/valkey:9.1.2-alpine
    restart: unless-stopped
    user: "valkey"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    environment:
      VALKEY_PASSWORD: ${%s}
    command:
      - sh
      - -ec
      - |
        printf 'requirepass %%s\nappendonly yes\ndir /data\n' "$$VALKEY_PASSWORD" > /tmp/valkey.conf
        exec valkey-server /tmp/valkey.conf
    volumes:
      - %s-data:/data
    networks:
      shared-backend: {}

`, service, passwordEnv, service)
	gatewayFiles := serviceaccess.TCPGatewayFiles{
		Config:   root + "/service-access/haproxy.cfg",
		PEM:      root + "/service-access/runtime/server.pem",
		Material: serviceaccess.TLSMaterial{},
	}
	b.WriteString(serviceaccess.TCPGatewayComposeService(gatewayFiles, serviceaccess.TCPGatewaySpec{
		ServiceName:      access,
		UpstreamHost:     service,
		UpstreamPort:     6379,
		PublishedPortEnv: portEnv,
		ContainerPort:    6379,
		Network:          "shared-backend",
	}))
}

func loadSharedBackendState(path, environment string) (sharedBackendState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return sharedBackendState{Version: sharedBackendStateVersion, Environment: environment, Applications: map[string]sharedBackendAppState{}}, nil
	}
	if err != nil {
		return sharedBackendState{}, err
	}
	var state sharedBackendState
	if err := json.Unmarshal(data, &state); err != nil {
		return sharedBackendState{}, err
	}
	if state.Version != sharedBackendStateVersion {
		return sharedBackendState{}, fmt.Errorf("unsupported shared backend state version %d", state.Version)
	}
	if state.Applications == nil {
		state.Applications = map[string]sharedBackendAppState{}
	}
	return state, nil
}

func writeSharedBackendState(path string, state sharedBackendState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeOwnerOnlyFile(path, data)
}

func sharedBackendApplicationKey(m Manifest) string {
	return sharedBackendToken(m.Name) + "/" + sharedBackendToken(m.Environment)
}

func sharedPostgresService(environment string) string {
	return "shared-postgres-" + sharedBackendToken(environment)
}

func sharedPostgresAlias() string { return "postgres-access" }

func sharedValkeyService(m Manifest, instance string) string {
	return sharedValkeyServiceFor(m.Name, m.Environment, instance)
}

func sharedValkeyServiceFor(application, environment, instance string) string {
	return "shared-valkey-" + sharedBackendToken(application+"-"+environment+"-"+instance)
}

func sharedValkeyAccessService(m Manifest, instance string) string {
	return sharedValkeyAccessServiceFor(m.Name, m.Environment, instance)
}

func sharedValkeyAccessServiceFor(application, environment, instance string) string {
	return sharedValkeyServiceFor(application, environment, instance) + "-access"
}

func sharedValkeyAccessAlias(m Manifest, instance string) string {
	return sharedValkeyAccessService(m, instance)
}

func sharedValkeyPortEnv(m Manifest, instance string) string {
	return sharedValkeyPortEnvFor(m.Name, m.Environment, instance)
}

func sharedValkeyPortEnvFor(application, environment, instance string) string {
	return "SHARED_VALKEY_" + envInstanceToken(sharedBackendToken(application+"-"+environment+"-"+instance)) + "_HOST_PORT"
}

func sharedValkeyPasswordEnvFor(application, environment, instance string) string {
	return "SHARED_VALKEY_" + envInstanceToken(sharedBackendToken(application+"-"+environment+"-"+instance)) + "_PASSWORD"
}

func postgresContainerHostKey(instance string) string {
	return postgresRuntimeKey(instance, "CONTAINER_HOST")
}
func valkeyContainerHostKey(instance string) string {
	return valkeyRuntimeKey(instance, "CONTAINER_HOST")
}

func sharedBackendToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func sharedPostgresDatabaseName(m Manifest, instance string) string {
	base := "baha_" + sharedBackendToken(m.Name) + "_" + sharedBackendToken(m.Environment)
	if instance != defaultServiceInstance {
		base += "_" + sharedBackendToken(instance)
	}
	return postgresIdentifierWithHash(base, m.Name+"|"+m.Environment+"|"+instance+"|db")
}

func sharedPostgresRoleName(m Manifest, instance string) string {
	base := "baha_" + sharedBackendToken(m.Name) + "_" + sharedBackendToken(m.Environment)
	if instance != defaultServiceInstance {
		base += "_" + sharedBackendToken(instance)
	}
	return postgresIdentifierWithHash(base, m.Name+"|"+m.Environment+"|"+instance+"|role")
}

func postgresIdentifierWithHash(base, identity string) string {
	sum := sha256.Sum256([]byte(identity))
	suffix := fmt.Sprintf("_%x", sum[:4])
	base = strings.ReplaceAll(base, "-", "_")
	maxBase := 63 - len(suffix)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return strings.Trim(base, "_") + suffix
}

func ensureSharedValkeyCredential(root, owner, instance string) (string, error) {
	token := sharedBackendToken(owner)
	if token == "" {
		token = "application"
	}
	name := token
	if strings.TrimSpace(instance) != "" {
		name += "-" + sharedBackendToken(instance)
	}
	ref := filepath.ToSlash(filepath.Join("credentials", "valkey", name+".password"))
	path := filepath.Join(root, filepath.FromSlash(ref))
	if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
		return ref, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	password, err := sharedBackendSecret(32)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := writeOwnerOnlyFile(path, []byte(password+"\n")); err != nil {
		return "", err
	}
	return ref, nil
}

func ensureSharedPostgresCredential(root, owner, instance string) (string, error) {
	token := sharedBackendToken(owner)
	if token == "" {
		token = "provider"
	}
	name := token
	if strings.TrimSpace(instance) != "" {
		name += "-" + sharedBackendToken(instance)
	}
	ref := filepath.ToSlash(filepath.Join("credentials", "postgres", name+".password"))
	path := filepath.Join(root, filepath.FromSlash(ref))
	if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
		return ref, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	password, err := sharedBackendSecret(32)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := writeOwnerOnlyFile(path, []byte(password+"\n")); err != nil {
		return "", err
	}
	return ref, nil
}

func readSharedBackendCredential(root, reference string) (string, error) {
	reference = filepath.Clean(filepath.FromSlash(strings.TrimSpace(reference)))
	if reference == "." || filepath.IsAbs(reference) || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return "", errors.New("shared backend credential reference is invalid")
	}
	data, err := os.ReadFile(filepath.Join(root, reference))
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("shared backend credential is empty")
	}
	return value, nil
}

func removeSharedBackendCredential(root, reference string) error {
	reference = filepath.Clean(filepath.FromSlash(strings.TrimSpace(reference)))
	if reference == "." || filepath.IsAbs(reference) || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return errors.New("shared backend credential reference is invalid")
	}
	if err := os.Remove(filepath.Join(root, reference)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func sharedBackendSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func quotePostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quotePostgresLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
