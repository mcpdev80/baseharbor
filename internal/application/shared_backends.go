package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const sharedBackendStateVersion = 3

type sharedBackendState struct {
	CoreSQL                       *bhruntime.Files                 `json:"core_sql,omitempty"`
	ValkeyAdminCredential         string                           `json:"valkey_admin_credential,omitempty"`
	ValkeyMembers                 int                              `json:"valkey_members,omitempty"`
	ValkeyHostPort                int                              `json:"valkey_host_port,omitempty"`
	Version                       int                              `json:"version"`
	Environment                   string                           `json:"environment"`
	PostgresMembers               int                              `json:"postgres_members,omitempty"`
	PostgresAdminCredential       string                           `json:"postgres_admin_credential,omitempty"`
	PostgresSuperuserCredential   string                           `json:"postgres_superuser_credential,omitempty"`
	PostgresReplicationCredential string                           `json:"postgres_replication_credential,omitempty"`
	PostgresHostPort              int                              `json:"postgres_host_port,omitempty"`
	PostgresUIHostPort            int                              `json:"postgres_ui_host_port,omitempty"`
	CacheUIHostPort               int                              `json:"cache_ui_host_port,omitempty"`
	ManagementUsername            string                           `json:"management_username,omitempty"`
	ManagementPassword            string                           `json:"management_password,omitempty"`
	Applications                  map[string]sharedBackendAppState `json:"applications"`
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
	Members         int    `json:"members"`
	HA              bool   `json:"ha"`
}

type sharedValkeyResource struct {
	Username            string `json:"username,omitempty"`
	KeyPrefix           string `json:"key_prefix,omitempty"`
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
	return bhruntime.ControlPlaneNetworkName(bhruntime.SharedResourceProjectName(namespace))
}

func SharedBackendFilesAt(dataDir, namespace, environment string) SharedBackendFiles {
	env := "core"
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "shared-backends", env)
	network := SharedBackendNetworkName(namespace, environment)
	// Retained bindings remain readable for backup and authorized cleanup.
	// Reconciliation rejects their implicit conversion before any mutation.
	if _, err := os.Lstat(filepath.Join(dir, "state.json")); errors.Is(err, os.ErrNotExist) {
		legacy := sharedBackendToken(environment)
		legacyDir := filepath.Join(filepath.Dir(dir), legacy)
		if legacy != "" && legacy != "core" {
			if _, err := os.Lstat(filepath.Join(legacyDir, "state.json")); err == nil {
				dir = legacyDir
				network = bhruntime.SharedResourceProjectName(namespace) + "-" + legacy + "-backends"
			}
		}
	}
	return SharedBackendFiles{
		Dir:             dir,
		Compose:         filepath.Join(dir, "compose.yaml"),
		Env:             filepath.Join(dir, "provider.env"),
		State:           filepath.Join(dir, "state.json"),
		Project:         bhruntime.SharedProjectName(namespace),
		ResourceProject: bhruntime.SharedResourceProjectName(namespace),
		Network:         network,
	}
}

func SharedCoreSQLFilesAt(dataDir, namespace string, m Manifest) (bhruntime.Files, bool, error) {
	if !UsesSharedPostgreSQL(m) {
		return bhruntime.Files{}, false, nil
	}
	files := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(files.State, m.Environment)
	if err != nil {
		return bhruntime.Files{}, false, err
	}
	if state.CoreSQL == nil {
		return bhruntime.Files{}, false, nil
	}
	return *state.CoreSQL, true, nil
}

func UsesSharedPostgreSQL(m Manifest) bool {
	placement, err := ResolveProviderPlacement(m, capability.ProviderPostgreSQL)
	return err == nil && placement.Scope == capability.ScopeShared && len(SQLInstanceNames(m)) > 0
}

func UsesSharedValkey(m Manifest) bool {
	placement, err := ResolveProviderPlacement(m, capability.ProviderValkey)
	return err == nil && placement.Scope == capability.ScopeShared && len(ValkeyInstanceNames(m)) > 0
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
	if err := rejectLegacySharedBackends(dataDir); err != nil {
		return false, err
	}
	if err := os.MkdirAll(shared.Dir, 0700); err != nil {
		return false, err
	}
	unlock, err := coreinstallation.AcquireLifecycleLock(shared.Dir)
	if err != nil {
		return false, fmt.Errorf("shared provider lifecycle is already active: %w", err)
	}
	defer unlock()
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		return false, err
	}
	dependency, ok := issuer.(bhruntime.CoreDependencyProvider)
	if !ok {
		return false, errors.New("shared backends require the existing selected Core provider")
	}
	core, err := dependency.CoreRuntimeFiles()
	if err != nil {
		return false, err
	}
	if core.Project != bhruntime.SharedProjectName(namespace) || core.ResourceProject != bhruntime.SharedResourceProjectName(namespace) {
		return false, errors.New("shared backend dependency does not belong to the selected Core/Target")
	}
	if state.CoreSQL == nil && (state.PostgresAdminCredential != "" || len(state.Applications) > 0) {
		return false, errors.New("retained shared backends use a separate provider; explicit backup-verified migration is required and currently unsupported; existing resources are retained")
	}
	if state.CoreSQL != nil && (state.CoreSQL.Project != core.Project || state.CoreSQL.Compose != core.Compose || state.CoreSQL.Env != core.Env) {
		return false, errors.New("shared backend Core binding changed; explicit migration is required")
	}
	state.CoreSQL = &core
	state.Environment = "core"
	if err := checkSharedValkeyTopology(state, m); err != nil {
		return false, err
	}
	if UsesSharedPostgreSQL(m) {
		requirement := AvailabilityIntent(m).Resolve("sql")
		members := 1
		if core.HA {
			members = 3
		}
		if requirement.HA && !core.HA || requirement.Instances > 0 && requirement.Instances != members {
			return false, errors.New("shared PostgreSQL intent differs from the existing Core topology; explicitly select a matching Core or application placement")
		}
		state.PostgresMembers = members
	}
	if err := os.MkdirAll(shared.Dir, 0o700); err != nil {
		return false, fmt.Errorf("create shared backend state: %w", err)
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
		if state.CoreSQL == nil {
			if state.PostgresAdminCredential == "" {
				ref, err := ensureSharedPostgresCredential(shared.Dir, "provider-admin", "")
				if err != nil {
					return false, err
				}
				state.PostgresAdminCredential = ref
			}
			if state.PostgresSuperuserCredential == "" {
				ref, err := ensureSharedPostgresCredential(shared.Dir, "provider-superuser", "")
				if err != nil {
					return false, err
				}
				state.PostgresSuperuserCredential = ref
			}
			if state.PostgresMembers > 1 && state.PostgresReplicationCredential == "" {
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
		} else {
			coreValues, err := bhruntime.RuntimeEnvironment(core)
			if err != nil {
				return false, err
			}
			state.PostgresHostPort, err = strconv.Atoi(coreValues["BASEHARBOR_POSTGRES_PORT"])
			if err != nil {
				return false, errors.New("shared Core SQL endpoint is invalid")
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
			values[postgresContainerHostKey(instance)] = "postgres"
		}
	}

	if err := bindSharedValkeyApplication(&state, shared, m, &app, values); err != nil {
		return false, err
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
	if sharedBackendHasRuntimeServices(state) {
		if err := compose.ConfigProject(ctx, shared.Project, shared.Compose, shared.Env); err != nil {
			return false, fmt.Errorf("validate shared backend runtime: %w", err)
		}
		if err := compose.UpProject(ctx, shared.Project, shared.Compose, shared.Env); err != nil {
			return false, fmt.Errorf("start shared backend runtime: %w", err)
		}
	}
	if UsesSharedPostgreSQL(m) {
		for _, resource := range app.SQL {
			password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
			if err != nil {
				return false, err
			}
			if err := bhruntime.EnsureCoreSQLConsumer(ctx, compose, core, resource.Database, resource.Username, password, "baseharbor:shared:application:"+appKey+":"+resource.Database); err != nil {
				return false, err
			}
		}
	}
	if UsesSharedValkey(m) {
		if err := reconcileCoreSharedValkeyACL(ctx, compose, shared, state); err != nil {
			return false, err
		}
		if err := waitSharedValkeyReady(ctx, compose, shared, m); err != nil {
			return false, err
		}
	}
	if _, err := EnsureRuntimeContract(m, files); err != nil {
		return false, fmt.Errorf("refresh application contract for shared backends: %w", err)
	}
	return true, nil
}

func bindSharedValkeyApplication(state *sharedBackendState, shared SharedBackendFiles, m Manifest, app *sharedBackendAppState, values map[string]string) error {
	if UsesSharedValkey(m) {
		if err := selectCoreSharedValkey(state, shared, m); err != nil {
			return err
		}
		for _, instance := range ValkeyInstanceNames(m) {
			port := state.ValkeyHostPort
			ref, err := ensureSharedValkeyCredential(shared.Dir, sharedBackendApplicationKey(m), instance)
			if err != nil {
				return err
			}
			password, err := readSharedBackendCredential(shared.Dir, ref)
			if err != nil {
				return err
			}
			app.Cache[instance] = sharedValkeyResource{
				Username:            sharedValkeyConsumerName(m, instance),
				KeyPrefix:           sharedValkeyConsumerPrefix(m, instance),
				CredentialReference: ref,
				HostPort:            port,
				Instances:           valkeyMemberCount(m, instance),
			}
			values[valkeyRuntimeKey(instance, "PASSWORD")] = password
			values[valkeyRuntimeKey(instance, "USER")] = app.Cache[instance].Username
			values[valkeyRuntimeKey(instance, "KEY_PREFIX")] = app.Cache[instance].KeyPrefix
			values[valkeyRuntimeKey(instance, "HOST_PORT")] = strconv.Itoa(port)
			values[valkeyContainerHostKey(instance)] = coreSharedValkeyAccessAlias()
		}
	}
	return nil
}
