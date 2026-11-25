package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

var ErrUnsupportedService = errors.New("application contains services that are not yet supported by apply")

type RuntimeFiles struct {
	Dir             string
	Compose         string
	Env             string
	ApplicationEnv  string
	Bindings        string
	Project         string
	ResourceProject string
	Namespace       string
}

func RuntimeProjectName(m Manifest) string {
	return "baseharbor-" + m.Name + "-" + m.Environment
}

// RuntimeProjectNameForApp derives the workload runtime project name from the
// retained application name and environment alone. It is used by the
// damaged-state network check, where the deploy directory is a per-deploy UUID
// carrying no application name, so the manifest/record identity is the only
// source of truth for the workload network name.
func RuntimeProjectNameForApp(applicationName, environment string) string {
	return "baseharbor-" + applicationName + "-" + environment
}

func RuntimeProjectNameForStore(store Store, m Manifest) string {
	namespace := strings.TrimSpace(strings.ReplaceAll(store.Namespace, ".", "-"))
	if namespace == "" {
		return RuntimeProjectName(m)
	}
	return "baseharbor-" + namespace + "-" + m.Name + "-" + m.Environment
}

func RuntimeComposeProjectNameForStore(store Store, m Manifest) string {
	return bhruntime.ApplicationProjectName(store.Namespace, m.Name, m.Environment)
}

func CheckSupportedRuntimeServices(m Manifest) error {
	if err := validateProviderAvailabilityIntent(m); err != nil {
		return err
	}
	if len(SQLInstanceNames(m)) > 0 && !UsesSharedPostgreSQL(m) && ComponentHA(m, "sql") {
		return fmt.Errorf("%w: application-scoped PostgreSQL is single-instance; select native shared placement for HA", ErrUnsupportedService)
	}
	if !HasManagedRuntimeServices(m) {
		if m.Services.Secrets && !HasExplicitWorkload(m) {
			return fmt.Errorf("%w: managed secrets currently require PostgreSQL or Valkey so the application has a materialized runtime", ErrUnsupportedService)
		}
		if HasObjectStorage(m) || HasExplicitWorkload(m) {
			return nil
		}
		return fmt.Errorf("%w: no supported runtime service or explicit Compose workload is enabled", ErrUnsupportedService)
	}
	return nil
}

func RuntimeFilesFor(store Store, m Manifest) RuntimeFiles {
	dir := filepath.Join(store.Root, m.Name, "runtime")
	return RuntimeFiles{
		Dir:             dir,
		Compose:         filepath.Join(dir, "compose.yaml"),
		Env:             filepath.Join(dir, "runtime.env"),
		ApplicationEnv:  filepath.Join(dir, "application.env"),
		Bindings:        filepath.Join(dir, "bindings"),
		Project:         RuntimeComposeProjectNameForStore(store, m),
		ResourceProject: RuntimeProjectNameForStore(store, m),
		Namespace:       strings.TrimSpace(store.Namespace),
	}
}

// EnsureRuntime renders and reconciles the application runtime. Its
// apply-boundary datasource call is the LAST load-time read of the seam
// pointers: the metrics package registers them at its init, and the
// application package's own tests register a fallback resolver in TestMain
// (after all package inits, before any test call), so a single EnsureRuntime
// invocation always observes the same seam state.
func EnsureRuntime(ctx context.Context, issuer serviceaccess.Issuer, store Store, m Manifest) (RuntimeFiles, error) {
	if err := m.Validate(); err != nil {
		return RuntimeFiles{}, err
	}
	if err := CheckSupportedRuntimeServices(m); err != nil {
		return RuntimeFiles{}, err
	}

	files := RuntimeFilesFor(store, m)
	if err := CheckRuntimeTopology(files, m); err != nil {
		return RuntimeFiles{}, err
	}
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		return RuntimeFiles{}, fmt.Errorf("create application runtime directory: %w", err)
	}
	if len(ValkeyInstanceNames(m)) > 0 && !UsesSharedValkey(m) {
		if err := ensureValkeyProviderBuildContext(files.Dir); err != nil {
			return RuntimeFiles{}, fmt.Errorf("prepare Valkey provider image: %w", err)
		}
	}
	if err := ensureRuntimeEnv(files.Env, m); err != nil {
		return RuntimeFiles{}, err
	}
	if err := ensureMongoDBInitFiles(files, m); err != nil {
		return RuntimeFiles{}, err
	}
	if err := ensureRabbitMQHAConfigFiles(files, m); err != nil {
		return RuntimeFiles{}, err
	}
	if err := EnsureBackendServiceAccess(ctx, issuer, files, m); err != nil {
		return RuntimeFiles{}, err
	}
	if err := EnsureApplicationManagementUIs(ctx, issuer, files, m); err != nil {
		return RuntimeFiles{}, fmt.Errorf("reconcile application management UIs: %w", err)
	}

	// The 0428-083 apply-boundary call site: the Prometheus datasource the
	// RUNNING Perses consumes is materialized BEFORE the compose render, into
	// this deployment's datasource projection root
	// <deploymentRoot>/applications/<resourceProject>/providers/management-ui/
	// perses — the providers/management-ui/perses/ layout the Perses service
	// already mounts its TLS material from, one path layout for the whole
	// provider projection. The compose render derives the mount sources from
	// the materializer's own path formula (the seam resolver), so mounts and
	// artifacts can never drift apart and no second constant exists.
	if err := ensurePersesDatasourceRuntime(ctx, issuer, files, m, store); err != nil {
		return RuntimeFiles{}, err
	}

	// The 0428-181 apply-boundary call site: the selected, verified Perses
	// identity dashboard is PROVISIONED through the registered dashboard
	// provisioner BEFORE the compose render, into this deployment's dashboard
	// projection root <deploymentRoot>/applications/<resourceProject>/
	// providers/management-ui/perses/provisioning/dashboards — the same sibling
	// of the runtime dir the datasource materializer writes beside. A selected
	// surface that the REAL per-target probe (metrics.PersesBackendAnswersNow)
	// reports verified exports its eligible dashboard JSON; a deselected, silent,
	// unverified or failed surface exports nothing and cleans any stale
	// dashboards (fail closed, 0428-181). The render mounts the whole
	// provisioning tree, so the loader loads the dashboards recursively with NO
	// config.yaml change.
	if err := ensurePersesDashboardRuntime(ctx, issuer, files, m, store); err != nil {
		return RuntimeFiles{}, err
	}

	compose, err := RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		return RuntimeFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		return RuntimeFiles{}, fmt.Errorf("write application compose file: %w", err)
	}
	if err := reconcileManagedRuntimeObservability(m, files.Project, files.Dir); err != nil {
		return RuntimeFiles{}, fmt.Errorf("reconcile managed runtime observability: %w", err)
	}
	if HasSharedBackends(m) {
		files.ApplicationEnv = filepath.Join(files.Dir, "application.env")
		files.Bindings = filepath.Join(files.Dir, "bindings")
		return files, nil
	}
	contract, err := EnsureRuntimeContract(m, files)
	if err != nil {
		return RuntimeFiles{}, err
	}
	files.ApplicationEnv = contract.Env
	files.Bindings = contract.BindingsDir
	return files, nil
}

// EnsurePostgresRuntime is kept for callers from the first runtime milestone.
func EnsurePostgresRuntime(ctx context.Context, issuer serviceaccess.Issuer, store Store, m Manifest) (RuntimeFiles, error) {
	return EnsureRuntime(ctx, issuer, store, m)
}

type BackendProbeKind string

const (
	BackendProbeSQLSelectOne         BackendProbeKind = "sql.select-one"
	BackendProbeSQLConsumerInventory BackendProbeKind = "sql.consumer-inventory"
	BackendProbeCachePing            BackendProbeKind = "cache.ping"
	BackendProbeDurableKeyValueRW    BackendProbeKind = "database.key-value.write-read"
)

type BackendProbe struct {
	Kind     BackendProbeKind
	Instance string
	Database string
}

type BackendProbeExecutor interface {
	ProbeBackend(context.Context, BackendProbe) (string, error)
}

// The native file/project mechanics remain in the existing local realization
// adapter; semantic probes also accept independently owned remote services.
func NewRuntimeBackendProbeExecutor(runtime RuntimeProjectExecutor, files RuntimeFiles) RuntimeBackendProbeExecutor {
	executor := RuntimeBackendProbeExecutor{files: files}
	if runtime != nil {
		executor.local = func(ctx context.Context, service string, argv ...string) (string, error) {
			return runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, service, argv...)
		}
	}
	return executor
}

// SQLConsumerInventory reports a real credential-bound query, not a proposed
// binding. User-object and large-object counts describe only the selected DB.
type SQLConsumerInventory struct {
	Consumer string
	Instance string
	Empty    bool
	Evidence string
}

func ObserveSQLConsumerInventory(ctx context.Context, executor BackendProbeExecutor, files RuntimeFiles, m Manifest, instance string) (SQLConsumerInventory, error) {
	var result SQLConsumerInventory
	if executor == nil || UsesSharedPostgreSQL(m) {
		return result, errors.New("application-scoped SQL probe executor required")
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return result, err
	}
	wantUser, wantDB := values[postgresRuntimeKey(instance, "USER")], values[postgresRuntimeKey(instance, "DB")]
	if wantUser == "" || wantDB == "" || wantDB != postgresDatabaseName(m, instance) {
		return result, errors.New("SQL consumer identity/database inventory unavailable")
	}
	out, err := executor.ProbeBackend(ctx, BackendProbe{Kind: BackendProbeSQLConsumerInventory, Instance: instance, Database: wantDB})
	if err != nil {
		return result, errors.New("protected SQL consumer inventory query failed")
	}
	fields := strings.Split(strings.TrimSpace(out), "|")
	if len(fields) != 4 || fields[0] != wantUser || fields[1] != wantDB {
		return result, errors.New("SQL consumer identity or database differs from protected runtime owner")
	}
	objects, err1 := strconv.ParseUint(fields[2], 10, 64)
	largeObjects, err2 := strconv.ParseUint(fields[3], 10, 64)
	if err1 != nil || err2 != nil {
		return result, errors.New("SQL consumer catalog inventory could not be verified")
	}
	result.Consumer = "application/" + m.ApplicationID + "/" + m.Environment
	result.Instance = instance
	result.Empty = objects == 0 && largeObjects == 0
	result.Evidence = "native TLS-verified SELECT with protected application-scoped SQL user/database, system catalog object and large-object inventory"
	return result, nil
}

func VerifyPostgresProvider(ctx context.Context, executor BackendProbeExecutor, m Manifest) error {
	if UsesSharedPostgreSQL(m) {
		return nil
	}
	if executor == nil {
		return errors.New("postgres verification executor is required")
	}
	for _, instance := range SQLInstanceNames(m) {
		out, err := executor.ProbeBackend(ctx, BackendProbe{
			Kind: BackendProbeSQLSelectOne, Instance: instance, Database: postgresDatabaseName(m, instance),
		})
		if err != nil {
			return fmt.Errorf("verify postgres instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "1" {
			return fmt.Errorf("verify postgres instance %s: unexpected query result %q", instance, strings.TrimSpace(out))
		}
	}
	return nil
}

func VerifyPostgresRuntime(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles) error {
	return VerifyPostgresProvider(ctx, NewRuntimeBackendProbeExecutor(runtime, files), m)
}

func VerifyValkeyProvider(ctx context.Context, executor BackendProbeExecutor, m Manifest) error {
	if UsesSharedValkey(m) {
		return nil
	}
	if executor == nil {
		return errors.New("valkey verification executor is required")
	}
	for _, instance := range CacheInstanceNames(m) {
		out, err := executor.ProbeBackend(ctx, BackendProbe{Kind: BackendProbeCachePing, Instance: instance})
		if err != nil {
			return fmt.Errorf("verify valkey cache instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "PONG" {
			return fmt.Errorf("verify valkey cache instance %s: unexpected PING result %q", instance, strings.TrimSpace(out))
		}
	}
	for _, instance := range KeyValueInstanceNames(m) {
		out, err := executor.ProbeBackend(ctx, BackendProbe{Kind: BackendProbeDurableKeyValueRW, Instance: instance})
		if err != nil {
			return fmt.Errorf("verify durable valkey instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "durable" {
			return fmt.Errorf("verify durable valkey instance %s: unexpected write/read result %q", instance, strings.TrimSpace(out))
		}
	}
	return nil
}
func VerifyValkeyRuntime(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles) error {
	if UsesSharedValkey(m) {
		return VerifyValkeyProvider(ctx, NewRuntimeBackendProbeExecutor(runtime, files), m)
	}
	executor := NewRuntimeBackendProbeExecutor(runtime, files)
	for _, instance := range CacheInstanceNames(m) {
		if valkeyMemberCount(m, instance) > 1 {
			if err := verifyValkeyStableEndpoint(ctx, m, files, instance, false); err != nil {
				return fmt.Errorf("verify Valkey HA cache instance %s: %w", instance, err)
			}
			continue
		}
		out, err := executor.ProbeBackend(ctx, BackendProbe{Kind: BackendProbeCachePing, Instance: instance})
		if err != nil {
			return fmt.Errorf("verify valkey cache instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "PONG" {
			return fmt.Errorf("verify valkey cache instance %s: unexpected PING result %q", instance, strings.TrimSpace(out))
		}
	}
	for _, instance := range KeyValueInstanceNames(m) {
		if valkeyMemberCount(m, instance) > 1 {
			if err := verifyValkeyStableEndpoint(ctx, m, files, instance, true); err != nil {
				return fmt.Errorf("verify durable Valkey HA instance %s: %w", instance, err)
			}
			continue
		}
		out, err := executor.ProbeBackend(ctx, BackendProbe{Kind: BackendProbeDurableKeyValueRW, Instance: instance})
		if err != nil {
			return fmt.Errorf("verify durable valkey instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "durable" {
			return fmt.Errorf("verify durable valkey instance %s: unexpected write/read result %q", instance, strings.TrimSpace(out))
		}
	}
	return nil
}

func RuntimeComposeYAML(m Manifest) (string, error) {
	return RuntimeComposeYAMLForProject(m, RuntimeFiles{ResourceProject: RuntimeProjectName(m)})
}

// RegisterPersesDatasourceMaterializer registers the datasource materializer
// entry point of the metrics package with the application lifecycle seam (card
// 0428-083). It is called once from the metrics package init; the seam exists
// because internal/metrics imports internal/application, so the lifecycle
// cannot carry a static edge in the other direction without an import cycle.
func RegisterPersesDatasourceMaterializer(fn func(Manifest, string, string) error) {
	materialize := persesDatasourceMaterializeFunc(fn)
	persesDatasourceMaterializer.Store(&materialize)
}

// persesDatasourceMaterializer is the lifecycle seam to the datasource
// materializer of the metrics package. The materializer root is the datasource
// projection of an application deployment: the providers/management-ui/perses
// sibling of its runtime dir. The lifecycle passes the APPLICATION RUNTIME
// DIR (RuntimeFiles.Dir, computed by RuntimeFilesFor — the one layout formula
// of this Core), so the projection lands beside the runtime dir the compose
// render (whose context dir is that same dir) mounts from, and no second
// layout derivation exists on this route (0428-083 F1).
var persesDatasourceMaterializer atomic.Pointer[persesDatasourceMaterializeFunc]

// persesDatasourceMaterializeFunc is the lifecycle-visible signature of the
// datasource materializer: (manifest, application runtime dir, namespace)
// -> error.
type persesDatasourceMaterializeFunc func(m Manifest, runtimeDir, namespace string) error

// RegisterPersesDashboardProvisioner registers the metrics-side dashboard
// provisioner with the application lifecycle seam (card 0428-181). It is called
// once from the metrics package init; the seam exists because internal/metrics
// imports internal/application, so the lifecycle cannot carry a static edge in
// the other direction without an import cycle.
func RegisterPersesDashboardProvisioner(fn func(m Manifest, runtimeDir, namespace string) error) {
	provisioner := persesDashboardProvisionerFunc(fn)
	persesDashboardProvisioner.Store(&provisioner)
}

// persesDashboardProvisioner is the lifecycle seam to the dashboard
// provisioner of the metrics package. The provisioner root is the dashboard
// projection of an application deployment: the providers/management-ui/perses
// sibling of its runtime dir. The lifecycle passes the APPLICATION RUNTIME DIR
// (RuntimeFiles.Dir, computed by RuntimeFilesFor — the one layout formula of
// this Core), so the dashboards land beside the runtime dir the compose render
// mounts from, and no second layout derivation exists on this route (0428-181
// F1).
var persesDashboardProvisioner atomic.Pointer[persesDashboardProvisionerFunc]

// persesDashboardProvisionerFunc is the lifecycle-visible signature of the
// dashboard provisioner: (manifest, application runtime dir, namespace) ->
// error.
type persesDashboardProvisionerFunc func(m Manifest, runtimeDir, namespace string) error

// PersesDashboardRuntimeRoot resolves the dashboard projection root of a
// deployment addressed by its runtime dir: the
// providers/management-ui/perses sibling of the runtime dir, the SAME root the
// datasource materializer writes beside (0428-083 F1). It is the application-
// side single source of truth for the dashboard projection; the metrics
// provisioner (internal/metrics) mirrors the derivation so the render mounts
// the paths the provisioner writes.
func PersesDashboardRuntimeRoot(runtimeDir string) string {
	return PersesDatasourceRuntimeRoot(runtimeDir)
}

// ensurePersesDatasourceRuntime is the apply-boundary call site of card
// 0428-083: the validated Prometheus datasource reaches the RUNNING Perses
// through the registered materializer BEFORE the compose render starts it,
// and the render mounts exactly the paths the materializer wrote. Fail-closed:
// without a registered materializer a selected surface never starts Perses
// unprovisioned, and a deselected surface tears its projection down so a stale
// datasource can never be served.
func ensurePersesDatasourceRuntime(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, store Store) error {
	materialize := persesDatasourceMaterializer.Load()
	if materialize == nil {
		// No seam registered in this binary (a build without the metrics
		// package, e.g. the application package's own test binary). The
		// lifecycle keeps its fail-closed contract through the LOCAL layout
		// convention: a deselected surface always removes its datasource
		// projection; a selected surface gets the projection DIRECTORY
		// layout (config file + provisioning folder + the trust-bundle
		// placeholder the metrics entry point fills) written under the
		// datasource root the render mounts, so the Perses container starts
		// with a loadable (empty until the metrics entry point provisions
		// it) provisioning set and never with mounts of missing files. In
		// the CLI build the metrics seam is always registered and performs
		// the full validated materialization here.
		if !PersesUITLSRequired(m) {
			return RemovePersesDatasourceProjection(files, m)
		}
		return ensurePersesDatasourceFallbackLayout(PersesDatasourceRuntimeRoot(files.Dir))
	}
	// The root argument IS the application runtime dir (files.Dir, computed
	// by RuntimeFilesFor — the one layout formula of this Core): the
	// materializer writes, the render mounts and this package's fallback all
	// address the same sibling-of-runtime datasource root, so the lifecycle
	// never carries a second layout derivation (0428-083 F1).
	return (*materialize)(m, files.Dir, strings.TrimSpace(store.Namespace))
}

// ensurePersesDashboardRuntime is the apply-boundary call site of card
// 0428-181: the selected, verified Perses dashboard reaches the RUNNING Perses
// through the registered dashboard provisioner BEFORE the compose render starts
// it, and the render mounts exactly the paths the provisioner wrote. Fail
// closed: without a registered provisioner a selected surface starts with an
// empty dashboards folder (the metrics entry point fills it); a deselected,
// silent, unverified or failed surface exports nothing and cleans any stale
// dashboards. The metrics-side provisioner holds the REAL per-target probe
// (perses_backend_status.go) and the capability filter gate, so a static edge
// from internal/application would be an import cycle — this seam is the only
// application-side edge (0428-181).
func ensurePersesDashboardRuntime(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest, store Store) error {
	provisioner := persesDashboardProvisioner.Load()
	if provisioner == nil {
		// No seam registered in this binary (a build without the metrics
		// package, e.g. the application package's own test binary). The
		// lifecycle keeps its fail-closed contract through the local layout
		// convention: a deselected surface always removes its dashboards
		// folder; a selected surface gets the empty dashboards folder the
		// render mounts, so a Perses start never fails on a missing source.
		if !PersesUITLSRequired(m) {
			return RemovePersesDashboardProjection(files, m)
		}
		return ensurePersesDashboardFallbackLayout(PersesDashboardRuntimeRoot(files.Dir))
	}
	return (*provisioner)(m, files.Dir, strings.TrimSpace(store.Namespace))
}

// RemovePersesDashboardProjection tears down the dashboards folder of a
// deselected Perses surface (the no-seam fallback for the application package's
// own test binary). Fail closed: a deselected surface must not leave a stale
// dashboard a re-selected Perses would load.
func RemovePersesDashboardProjection(files RuntimeFiles, m Manifest) error {
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return nil
	}
	root := PersesDashboardRuntimeRoot(files.Dir)
	dir := filepath.Join(root, "provisioning", "dashboards")
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove perses dashboard projection: %w", err)
	}
	return nil
}

// ensurePersesDashboardFallbackLayout writes the dashboards DIRECTORY layout of
// a deployment that runs without the metrics-side dashboard provisioner seam:
// the empty provisioning/dashboards folder the render mounts, so a Perses start
// never fails on a missing source and never loads an unvalidated dashboard. The
// full provisioned content is the metrics-side contract of the CLI build.
func ensurePersesDashboardFallbackLayout(root string) error {
	root = filepath.Clean(root)
	if err := os.MkdirAll(filepath.Join(root, "provisioning", "dashboards"), 0o755); err != nil {
		return fmt.Errorf("perses dashboard fallback provisioning folder: %w", err)
	}
	return nil
}

// ensurePersesDatasourceFallbackLayout writes the datasource projection
// DIRECTORY layout (config file + provisioning folder) of a deployment that
// runs without the metrics-side materializer seam. The config file content is
// the deterministic provisioning contract (file database + the mounted
// provisioning folder + the reconcile interval); the provisioning folder
// starts EMPTY, so a Perses start never fails on a missing provisioning
// source and never loads an unvalidated datasource either. The full
// materialization (with validation and the GlobalDatasource resource) is the
// metrics-side contract of the CLI build.
func ensurePersesDatasourceFallbackLayout(datasourceRoot string) error {
	root := filepath.Clean(datasourceRoot)
	if err := os.MkdirAll(filepath.Join(root, "provisioning"), 0o755); err != nil {
		return fmt.Errorf("perses datasource fallback provisioning folder: %w", err)
	}
	configFile := filepath.Join(root, "config.yaml")
	if _, err := os.Stat(configFile); err == nil {
		return nil // a previous (metrics-side) materialization owns the content
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect perses datasource config: %w", err)
	}
	config := "{\n  \"database\": {\n    \"file\": {\n      \"folder\": \"/persistence/database\",\n      \"extension\": \"json\"\n    }\n  },\n  \"provisioning\": {\n    \"folders\": [\n      \"" + PersesDatasourceContainerProvisioningPath + "\"\n    ],\n    \"interval\": \"300s\"\n  }\n}\n"
	return writeUIRuntimeProjection(configFile, []byte(config))
}

// RuntimeComposeYAMLForProject renders the managed compose definition. Pure// callers (inventory, projection, retirement checks) pass no runtime files:
// the rendering then uses the deterministic relative bind sources rooted at
// the compose file's own directory, exactly as every other managed service
// renders them. The deployment path (EnsureRuntime) passes the materialized
// RuntimeFiles: only there the optional Perses gateway rendering (0428-089)
// can consume the provenance-checked projection state and fail closed on
// drift or absence. Without files, a selected Perses renders no host bind at
// all and the gateway record check below refuses the rendering, so a pure
// caller can never mistake this form for the applied definition.
func RuntimeComposeYAMLForProject(m Manifest, files RuntimeFiles) (string, error) {
	resourceProject := strings.TrimSpace(files.ResourceProject)
	if resourceProject == "" {
		resourceProject = RuntimeProjectName(m)
	}
	return runtimeComposeYAMLForProject(m, resourceProject, files)
}

func runtimeComposeYAMLForProject(m Manifest, resourceProject string, files RuntimeFiles) (string, error) {
	if err := CheckSupportedRuntimeServices(m); err != nil {
		return "", err
	}
	if !HasManagedRuntimeServices(m) {
		return "services: {}\n", nil
	}
	sqlInstances := SQLInstanceNames(m)
	cacheInstances := ValkeyInstanceNames(m)
	rabbitInstances := RabbitMQInstanceNames(m)
	mongoInstances := DocumentDatabaseInstanceNames(m)
	if UsesSharedPostgreSQL(m) {
		sqlInstances = nil
	}
	if UsesSharedValkey(m) {
		cacheInstances = nil
	}
	if len(sqlInstances) == 0 && len(cacheInstances) == 0 && len(rabbitInstances) == 0 && len(mongoInstances) == 0 {
		return "services: {}\n", nil
	}
	var b strings.Builder
	b.WriteString("services:\n")
	for _, instance := range sqlInstances {
		writePostgresComposeService(&b, instance)
	}
	for _, instance := range cacheInstances {
		writeValkeyHAComposeServices(&b, m, instance)
		b.WriteString(valkeyGatewayCompose(m, instance))
	}
	for _, instance := range rabbitInstances {
		writeRabbitMQComposeService(&b, m, instance)
		b.WriteString(rabbitmqGatewayCompose(m, instance))
		if m.Services.MessagingManagementUI {
			writeRabbitMQUIComposeService(&b, m, instance)
		}
	}
	for _, instance := range mongoInstances {
		writeMongoDBComposeService(&b, m, instance)
		if m.Services.DocumentDatabaseManagementUI {
			writeMongoDBUIComposeServices(&b, m, instance)
		}
	}
	if m.Services.SQLManagementUI && !UsesSharedPostgreSQL(m) {
		writePostgresUIComposeService(&b, m)
	}
	if (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && !UsesSharedValkey(m) {
		writeCacheUIComposeServices(&b, m)
	}
	if m.Services.Perses {
		// The gateway topology (0428-089): the Perses process never owns a
		// host port or a routable bind; its management gateway is the only
		// host-visible address, and the gateway reaches Perses through the
		// backend-network alias while Perses binds the shared-namespace
		// loopback. The rendering consumes only files with a proven producer
		// (EnsurePersesManagementUI) and fails closed on drift or absence.
		if strings.TrimSpace(files.Dir) == "" {
			return "", errors.New("the Perses gateway composition requires the materialized runtime files: a pure rendering cannot prove the mounted files' provenance")
		}
		if err := writePersesGatewayComposeServices(&b, m, files); err != nil {
			return "", err
		}
	}
	b.WriteString("\nvolumes:\n")
	for _, instance := range sqlInstances {
		service := runtimeServiceName("postgres", instance)
		fmt.Fprintf(&b, "  %s-data:\n    name: %s_%s-data\n", service, resourceProject, service)
	}
	for _, instance := range cacheInstances {
		for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
			volume := valkeyMemberVolumeName(instance, ordinal)
			fmt.Fprintf(&b, "  %s:\n    name: %s_%s\n", volume, resourceProject, volume)
		}
	}
	for _, instance := range rabbitInstances {
		for ordinal := 0; ordinal < rabbitmqMemberCount(m); ordinal++ {
			volume := rabbitmqMemberVolumeName(instance, ordinal)
			fmt.Fprintf(&b, "  %s:\n    name: %s_%s\n", volume, resourceProject, volume)
		}
	}
	for _, instance := range mongoInstances {
		for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
			volume := mongodbMemberVolumeName(instance, ordinal)
			fmt.Fprintf(&b, "  %s:\n    name: %s_%s\n", volume, resourceProject, volume)
		}
	}
	if m.Services.Perses {
		service := runtimeServiceName("perses", defaultServiceInstance)
		fmt.Fprintf(&b, "  %s-data:\n    name: %s\n", service, PersesRuntimeVolumeName(resourceProject, service))
		// The Perses database folder /etc/perses/database is writable via a
		// tmpfs of its own (emitted in writePersesGatewayComposeServices), so
		// it needs no named volume here. The shipped plugin archives extract
		// into /etc/perses/plugins at startup, which the pinned distroless
		// image has no such directory and whose rootfs is read-only, so a
		// deployment-scoped named volume provides the writable, host-isolated
		// extraction target the runner must support.
		fmt.Fprintf(&b, "  %s-plugins:\n    name: %s_database\n", service, resourceProject)
	}
	b.WriteString("\nnetworks:\n  default:\n")
	fmt.Fprintf(&b, "    name: %s\n", ApplicationBackendNetworkNameForProject(resourceProject))
	for _, instance := range cacheInstances {
		if valkeyMemberCount(m, instance) > 1 {
			fmt.Fprintf(&b, "  %s-ha:\n    internal: true\n", valkeyMemberServiceName(instance, 0))
		}
	}
	return b.String(), nil
}

func writePostgresComposeService(b *strings.Builder, instance string) {
	service := runtimeServiceName("postgres", instance)
	dbKey := postgresRuntimeKey(instance, "DB")
	userKey := postgresRuntimeKey(instance, "USER")
	passwordKey := postgresRuntimeKey(instance, "PASSWORD")
	portKey := postgresRuntimeKey(instance, "HOST_PORT")
	tlsRoot := "./" + filepath.ToSlash(filepath.Join("providers", "postgresql", instance, "runtime"))
	fmt.Fprintf(b, `  %s:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    read_only: true
    cap_drop: ["ALL"]
    cap_add: ["CHOWN", "DAC_READ_SEARCH", "FOWNER", "SETGID", "SETUID"]
    security_opt: ["no-new-privileges:true"]
    entrypoint:
      - /bin/sh
      - -ec
    command:
      - |
        cp /run/baseharbor/tls-source/server-key.pem /tmp/server-key.pem
        chmod 0600 /tmp/server-key.pem
        if [ "$$(id -u)" = 0 ]; then chown postgres:postgres /tmp/server-key.pem; fi
        exec /usr/local/bin/docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/run/baseharbor/tls-source/server-cert.pem -c ssl_key_file=/tmp/server-key.pem -c hba_file=/run/baseharbor/tls-source/pg_hba.conf
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /var/run/postgresql:rw,noexec,nosuid,nodev
    environment:
      POSTGRES_DB: ${%s}
      POSTGRES_USER: ${%s}
      POSTGRES_PASSWORD: ${%s}
    ports:
      - "127.0.0.1:${%s}:5432"
    volumes:
      - %s-data:/var/lib/postgresql
      - ./bindings/postgres/%s/ca.pem:/run/baseharbor/tls/ca.pem:ro
      - "%s/server-cert.pem:/run/baseharbor/tls-source/server-cert.pem:ro"
      - "%s/server-key.pem:/run/baseharbor/tls-source/server-key.pem:ro"
      - "%s/pg_hba.conf:/run/baseharbor/tls-source/pg_hba.conf:ro"
    healthcheck:
      test: ["CMD-SHELL", "PGPASSWORD=\"$${POSTGRES_PASSWORD}\" psql \"host=127.0.0.1 port=5432 user=$${POSTGRES_USER} dbname=$${POSTGRES_DB} sslmode=verify-ca sslrootcert=/run/baseharbor/tls/ca.pem\" -tAc 'SELECT 1' | grep -q '^1$'"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s

`, service, dbKey, userKey, passwordKey, portKey, service, instance, tlsRoot, tlsRoot, tlsRoot)
}

func writePostgresUIComposeService(b *strings.Builder, m Manifest) {
	b.WriteString(`  postgres-ui:
    image: ` + PostgresUIImage + `
    restart: unless-stopped
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      PGADMIN_DEFAULT_EMAIL: ${` + PostgresUIEmailEnv + `}
      PGADMIN_DEFAULT_PASSWORD_FILE: /run/baseharbor/password
      PGADMIN_ENABLE_TLS: "True"
      PGADMIN_LISTEN_PORT: "8443"
      PGADMIN_SERVER_JSON_FILE: /run/baseharbor/servers.json
      PGADMIN_REPLACE_SERVERS_ON_STARTUP: "True"
      PGADMIN_DISABLE_POSTFIX: "True"
      PGADMIN_CUSTOM_CONFIG_DISTRO_FILE: /var/lib/pgadmin/config_distro.py
      PGADMIN_CONFIG_MASTER_PASSWORD_REQUIRED: "False"
      PGPASS_FILE: /run/baseharbor/pgpass
      PGADMIN_CONFIG_SHARED_STORAGE: "[{'name': 'BaseHarbor', 'path': '/run/baseharbor', 'restricted_access': True}]"
    entrypoint: ["/bin/sh", "/run/baseharbor/pgadmin-entrypoint.sh"]
    ports:
      - "127.0.0.1:${` + PostgresUIHostPortEnv + `}:8443"
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /var/lib/pgadmin:rw,noexec,nosuid,nodev,mode=1777
    volumes:
      - ./providers/management-ui/postgres/password:/run/baseharbor/password:ro
      - ./providers/management-ui/postgres/servers.json:/run/baseharbor/servers.json:ro
      - ./providers/management-ui/postgres/pgpass:/run/baseharbor/pgpass:ro
      - ./providers/management-ui/postgres/pgadmin-entrypoint.sh:/run/baseharbor/pgadmin-entrypoint.sh:ro
      - ./providers/management-ui/postgres/server.cert:/certs/server.cert:ro
      - ./providers/management-ui/postgres/server.key:/certs/server.key:ro
`)
	for _, instance := range SQLInstanceNames(m) {
		token := envInstanceToken(instance)
		fmt.Fprintf(b, "      - ./providers/management-ui/postgres/postgres-%s.ca.pem:/run/baseharbor/postgres-%s.ca.pem:ro\n", token, token)
	}
	fmt.Fprintf(b, "    networks:\n      default:\n        aliases:\n          - %q\n\n", devaccess.ApplicationAlias(m.Name, "pgadmin"))
}

func writeCacheUIComposeServices(b *strings.Builder, m Manifest) {
	b.WriteString(`  cache-ui:
    image: ` + CacheUIImage + `
    restart: unless-stopped
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      HTTP_USER: ${` + CacheUIUserEnv + `}
      HTTP_PASSWORD_FILE: /run/baseharbor/http-password
      NOSAVE: "true"
      NO_LOG_DATA: "true"
      NODE_ENV: production
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    volumes:
      - ./providers/management-ui/cache/http-password:/run/baseharbor/http-password:ro
      - ./providers/management-ui/cache/local.json:/redis-commander/config/local.json:ro
      - ./providers/management-ui/cache/local-production.json:/redis-commander/config/local-production.json:ro

  cache-ui-access:
    image: ` + UIProxyImage + `
    restart: unless-stopped
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
      - "127.0.0.1:${` + CacheUIHostPortEnv + `}:8443"
    volumes:
      - ./providers/management-ui/cache/Caddyfile:/etc/caddy/Caddyfile:ro
      - ./providers/management-ui/cache/server.pem:/certs/server.pem:ro
      - ./providers/management-ui/cache/server-key.pem:/certs/server-key.pem:ro

`)
	fmt.Fprintf(b, "    networks:\n      default:\n        aliases:\n          - %q\n\n", devaccess.ApplicationAlias(m.Name, "cache"))
}
