package identityprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// KeycloakInstance is the runtime-neutral control surface consumed by the
// identity Core. Runtime-native deployment identity remains inside the
// realization that produced this instance.
type KeycloakInstance struct {
	StateDir         string
	EndpointBaseURL  string
	PublicBaseURL    string
	WorkloadBaseURL  string
	TrustBundle      []byte
	PublicHTTPClient *http.Client
	AdminHTTPClient  *http.Client
	AdminUsername    string
	AdminPassword    string
}

// KeycloakRealization is the deployment/runtime boundary for the managed
// Keycloak reference provider. Adding Kubernetes/OpenShift/cloud realizations
// must not require changes to Keycloak identity semantics.
type KeycloakRealization interface {
	Apply(context.Context) (KeycloakInstance, error)
	Existing(context.Context) (KeycloakInstance, error)
	Destroy(context.Context) error
}

type identityProjectDiagnostics interface {
	DiagnosticsProject(context.Context, string, string, string) string
}

type localKeycloakRealization struct {
	runtime   KeycloakRuntime
	lifecycle KeycloakLifecycle
	app       application.Manifest
	issuer    serviceaccess.Issuer
	dataDir   string
	namespace string
	files     KeycloakFiles
}

func newLocalKeycloakRealization(runtime KeycloakRuntime, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string) KeycloakRealization {
	return &localKeycloakRealization{
		runtime: runtime, lifecycle: NewKeycloakLifecycle(runtime), app: app,
		issuer: issuer, dataDir: dataDir, namespace: namespace,
	}
}

func (r *localKeycloakRealization) Apply(ctx context.Context) (KeycloakInstance, error) {
	files, err := EnsureKeycloakFilesAt(ctx, r.app, r.issuer, r.dataDir, r.namespace)
	if err != nil {
		return KeycloakInstance{}, err
	}
	r.files = files

	publicBase, err := localKeycloakPublicBaseURL(r.app, r.namespace, r.runtime, files)
	if err != nil {
		return KeycloakInstance{}, err
	}
	if err := prepareApplicationKeycloakCanonicalURL(r.runtime, r.namespace, files, publicBase); err != nil {
		return KeycloakInstance{}, err
	}
	if cleaner, ok := r.runtime.(legacyServiceCleaner); ok {
		if err := cleaner.RemoveProjectServices(ctx, files.Project, "keycloak-public", "keycloak-admin"); err != nil {
			return KeycloakInstance{}, err
		}
	}
	if err := r.lifecycle.Validate(ctx, files); err != nil {
		return KeycloakInstance{}, err
	}
	if err := r.lifecycle.Apply(ctx, files); err != nil {
		diagnosticCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if diagnostics := r.Diagnostics(diagnosticCtx); diagnostics != "" {
			return KeycloakInstance{}, fmt.Errorf("%w\n%s", err, diagnostics)
		}
		return KeycloakInstance{}, err
	}
	if isDevelopmentIdentityEnvironment(r.app.Environment) {
		if err := ensureLocalKeycloakDevelopmentRoute(ctx, r.app, r.namespace, r.runtime, r.issuer, files); err != nil {
			return KeycloakInstance{}, err
		}
		effectiveBase, err := localKeycloakPublicBaseURL(r.app, r.namespace, r.runtime, files)
		if err != nil {
			return KeycloakInstance{}, err
		}
		if effectiveBase != publicBase {
			publicBase = effectiveBase
			if err := prepareApplicationKeycloakCanonicalURL(r.runtime, r.namespace, files, publicBase); err != nil {
				return KeycloakInstance{}, err
			}
			if err := r.lifecycle.Apply(ctx, files); err != nil {
				return KeycloakInstance{}, err
			}
		}
	}
	if err := r.registerKeycloakSignals(); err != nil {
		return KeycloakInstance{}, err
	}
	return r.instance(ctx, files, publicBase)
}

// registerKeycloakSignals wires the Keycloak provider's native openmetrics
// signal into the observability signal registry the runtime fan-in reads. The
// signal is declared in the Keycloak integration descriptor
// (capability/reference.go) but the compose realizes it; this mirrors the Loki,
// Tempo and OTel collector signal registration (logs/realization.go,
// traces/tempo.go, telemetry/realization.go): the reference descriptor is the
// path, the metrics policy is the gate. Without this registration the signal is
// unreachable — perses_identity_witness can never match a real Keycloak metrics
// source and identity_dashboard_eligible can never become true. Registration is
// a no-op unless the deployment policy enables metrics collection, so a Target
// without metrics collection keeps every gate fail-closed (no regression of the
// 080 non-forgeability invariant).
func (r *localKeycloakRealization) registerKeycloakSignals() error {
	// Keep each Keycloak project's source distinct in the shared registry.
	// Other providers include their compose project in the registration ID too.
	registrationID := "keycloak:" + r.files.Project
	placement, err := application.ResolveProviderPlacement(r.app, capability.ProviderKeycloak)
	if err != nil {
		return err
	}
	metricsPolicy, err := application.MetricsPolicy(r.app)
	if err != nil {
		return err
	}
	class := observability.SourcePlatformProvider
	if placement.Scope == capability.ScopeApplication {
		class = observability.SourceApplicationProvider
	}
	metricsEnabled := (application.HasMetricsSources(r.app) || application.HasRuntimeMetricsPermissions(r.app)) && metricsPolicy.Enabled
	if class == observability.SourceApplicationProvider {
		metricsEnabled = metricsEnabled && metricsPolicy.Collect[application.MetricsSourceApplicationProvider]
	} else {
		metricsEnabled = metricsEnabled && metricsPolicy.Collect[application.MetricsSourcePlatformProvider]
	}
	signals := map[string]observability.ProviderSignalRuntime{}
	if metricsEnabled {
		signals["keycloak-metrics"] = observability.ProviderSignalRuntime{
			Network: r.files.ConsumerNetwork,
			Target:  "keycloak:9000",
		}
	}

	// 0428-168 migration: the pre-scoping runtime wrote its Keycloak metrics
	// source of record under the literal id "keycloak-metrics". The new scoping
	// ID would otherwise replace that entry and drop it from the registry
	// (ListMetrics/RegistryHasSignal see nothing of record, and its own Remove
	// cannot find it). The migration runs FIRST, inside the same change-aware
	// path under the existing registry state lock: it re-books every legacy
	// source of record onto a DERIVED identity - the owning application's
	// canonical project with the legacy signal name - never onto this caller's
	// registration ID, so the registration below can neither overwrite nor
	// delete it. The record's owner and every other field stay verbatim. With
	// nothing to migrate the step writes no byte at all (idempotent,
	// no-mutation). The owner->project maps the migration consults come from
	// the Target's own Keycloak state: the caller's canonical project first
	// (so the caller's own record converges onto its canonical registration
	// identity), then every recorded installation, so a foreign owner's record
	// migrates onto ITS grammar, never the caller's. A PARTIAL state scan
	// aborts the registration BEFORE the registry mutation (r2 review
	// blocker): guessing a foreign owner's project would re-book onto a
	// project whose grammar lacks the owner anchor, leaving it unobservable.
	ownerMaps, err := r.ownerProjectMaps(placement)
	if err != nil {
		return err
	}
	registration := observability.ProviderSignalRegistration{
		ID:               registrationID,
		Descriptor:       capability.KeycloakIntegration,
		Class:            class,
		Scope:            placement.Scope,
		SharingBoundary:  placement.SharingBoundary,
		OwnerApplication: r.app.Name,
		Enabled:          map[observability.SignalKind]bool{observability.SignalMetrics: metricsEnabled},
		Signals:          signals,
	}
	registration.Legacy = &observability.LegacyReconciliation{
		Source: observability.LegacySource{
			ID:        observability.LegacyKeycloakMetricsID,
			Providers: []capability.ProviderKind{capability.ProviderKeycloak},
		},
		Project:           r.files.Project,
		CallerApplication: r.app.Name,
		OwnerProjects:     ownerMaps,
	}
	return observability.RegisterProviderSignals(registration)
}

// RegisterKeycloakMetricsSignalsAt registers the native Keycloak metrics
// signal for a realized Target. The local realization calls this same
// production path after Apply; keeping it package-visible also lets integration
// tests verify that the metrics consumer observes the producer's real registry
// record rather than a hand-built SignalSource.
func RegisterKeycloakMetricsSignalsAt(app application.Manifest, files KeycloakFiles, dataDir, namespace string) error {
	return (&localKeycloakRealization{
		app: app, files: files, dataDir: dataDir, namespace: namespace,
	}).registerKeycloakSignals()
}

// ownerProjectMaps lists the owner->project maps the migration consults. The
// caller's own canonical project comes first; the state scan (providers/
// keycloak/applications/<name>/<environment>) enumerates the Target's other
// installations. When the scan finds NO application state at all - a legacy,
// pre-scoping deployment wrote none, which is exactly the migration case -
// the owners currently holding the unscoped literal fall back to the canonical
// identity grammar the current runtime computes for them. A scan that PARTIALLY
// fails (an application directory it cannot read, or one with no environment
// entry) returns an error and the caller aborts its registration: guessing a
// foreign owner's project from partial state would re-book the record onto a
// project whose grammar lacks the owner anchor, leaving it unobservable again
// (card 0428-168, r2 review blocker).
func (r *localKeycloakRealization) ownerProjectMaps(placement capability.ProviderPlacement) ([]map[string]string, error) {
	maps := []map[string]string{{r.app.Name: legacyMigrationProject(r.namespace, r.app, placement, r.files.Project)}}
	// A shared placement's records are environment-agnostic (the sharing
	// boundary carries the environment), so the project grammar must not be
	// consulted and a state scan can never strand them: the caller's project
	// alone is the correct map.
	if placement.Scope != capability.ScopeApplication {
		return maps, nil
	}
	dir := r.dataDir
	if strings.TrimSpace(dir) == "" {
		if resolved, err := bhruntime.DataDir(""); err == nil {
			dir = resolved
		}
	}
	recorded, err := ListKeycloakOwnerProjects(dir, r.namespace)
	if err != nil {
		return nil, err
	}
	if len(recorded) > 0 {
		maps = append(maps, recorded)
	}
	// The derived legacy-owner recovery is NOT conditional on the recorded
	// map being empty (r3 review P1 blocker): a pre-scoping legacy holder may
	// have no state directory while other applications do. legacyLiteralOwners
	// collects EVERY app that currently holds the unscoped literal, and maps
	// are consulted in order, so a recorded owner keeps its own state-derived
	// project while an un-recorded legacy holder resolves through the
	// grammar. A derived owner token can never equal a recorded owner's
	// project unless the state names match, so this union cannot drag a
	// scoped, already-migrated record back onto a derived identity: an app
	// whose state directory exists is always in `recorded` first.
	if derived, err := legacyLiteralOwners(r.namespace); err != nil {
		return nil, err
	} else if len(derived) > 0 {
		maps = append(maps, derived)
	}
	// Fail-closed (r3 recipe): a legacy record whose owner NEITHER the state
	// scan nor the registry grammar resolves must abort the registration
	// BEFORE the mutation, instead of letting ProjectFor substitute the
	// caller's project — that identity's grammar lacks the owner anchor and
	// the re-booked record would be unobservable again.
	unresolved, err := r.unresolvedLegacyOwners(maps)
	if err != nil {
		return nil, err
	}
	if len(unresolved) > 0 {
		return nil, fmt.Errorf("keycloak legacy migration: no project resolves for legacy source owner(s) %s", strings.Join(unresolved, ", "))
	}
	return maps, nil
}

// unresolvedLegacyOwners lists the applications that hold the unscoped legacy
// Keycloak source of record but resolve to NO project: not through the
// owner->project maps the migration consults, and not through their own
// record's anchor-bearing ID or runtime:// target (the ProjectFor routes that
// need no caller knowledge).
func (r *localKeycloakRealization) unresolvedLegacyOwners(maps []map[string]string) ([]string, error) {
	sources, err := observability.ListAll()
	if err != nil {
		return nil, err
	}
	var unresolved []string
	for _, source := range sources {
		if source.ID != observability.LegacyKeycloakMetricsID || source.Provider != capability.ProviderKeycloak {
			continue
		}
		if source.Scope != capability.ScopeApplication || strings.TrimSpace(source.OwnerApplication) == "" {
			continue
		}
		resolved := false
		for _, ownerProject := range maps {
			if project, ok := ownerProject[source.OwnerApplication]; ok && strings.TrimSpace(project) != "" {
				resolved = true
				break
			}
		}
		if !resolved && !legacyRecordAnchorsOwnProject(source, r.files.Project) {
			unresolved = append(unresolved, source.OwnerApplication)
		}
	}
	return unresolved, nil
}

// legacyRecordAnchorsOwnProject reports whether a legacy record carries its
// own anchor-bearing project — the route ProjectFor walks WITHOUT any
// owner->project map, reproduced here so the abort is deterministic and never
// depends on the caller's project.
func legacyRecordAnchorsOwnProject(source observability.SignalSource, callerProject string) bool {
	if project, ok := observability.ScopedIDProject(source.ID); ok && legacyProjectCarriesAnchor(project, source.OwnerApplication) {
		return true
	}
	target := strings.TrimPrefix(source.Target, "runtime://")
	if idx := strings.Index(target, "/"); idx >= 0 {
		target = target[:idx]
	}
	return legacyProjectCarriesAnchor(target, source.OwnerApplication)
}

func legacyProjectCarriesAnchor(project, application string) bool {
	if strings.TrimSpace(project) == "" {
		return false
	}
	return strings.Contains("-"+strings.ToLower(project)+"-", "-"+strings.ToLower(application)+"-")
}

// legacyLiteralOwners collects owner->canonical identity project for every
// application that holds the unscoped legacy Keycloak source of record in the
// registry right now. It is the last-resort recovery when the Target carries
// no application placement state: the record's owner token is known, and the
// current runtime's project formula for that owner is
// "bh-<target>-<app>-identity-<environment>". The environment is the one the
// registering deployment lives in.
func legacyLiteralOwners(namespace string) (map[string]string, error) {
	sources, err := observability.ListAll()
	if err != nil {
		return nil, err
	}
	owners := map[string]string{}
	for _, source := range sources {
		if source.ID != observability.LegacyKeycloakMetricsID || source.Provider != capability.ProviderKeycloak {
			continue
		}
		if source.Scope != capability.ScopeApplication || strings.TrimSpace(source.OwnerApplication) == "" {
			continue
		}
		owners[source.OwnerApplication] = legacyOwnerProject(namespace, source.OwnerApplication)
	}
	if len(owners) == 0 {
		return nil, nil
	}
	return owners, nil
}

// legacyOwnerProject is the canonical identity project formula the current
// runtime writes for an application's Keycloak placement resource.
func legacyOwnerProject(namespace, application string) string {
	return bhruntime.ApplicationProjectName(namespace, application+"-identity", identityDeploymentEnvironment())
}

// identityDeploymentEnvironment reports the environment of the deployment
// running the migration: the state root the registry itself resolves through
// carries the canonical project name, whose trailing token is the environment
// (bhruntime derives state paths per deployment environment).
func identityDeploymentEnvironment() string {
	dir, err := bhruntime.DataDir("")
	if err != nil {
		return "dev"
	}
	tokens := strings.Split(strings.ToLower(filepath.Base(dir)), "-")
	if len(tokens) >= 2 {
		if env := tokens[len(tokens)-1]; env != "" && !reservedIdentityToken(env) {
			return env
		}
	}
	return "dev"
}

// reservedIdentityToken reports whether a state-dir token is platform
// vocabulary rather than an environment name: a token the environment token
// never equals (the reserved placement-resource vocabulary).
func reservedIdentityToken(token string) bool {
	switch token {
	case "shared", "postgres", "openbao", "runtime-executor", "object-storage",
		"metrics", "prometheus", "logs", "loki", "traces", "tempo",
		"telemetry", "otel-collector", "seaweedfs", "identity":
		return true
	default:
		return false
	}
}

// legacyMigrationProject names the canonical compose project an application
// placement's identity resource carries.
func legacyMigrationProject(namespace string, app application.Manifest, placement capability.ProviderPlacement, callerProject string) string {
	if placement.Scope != capability.ScopeApplication {
		return callerProject
	}
	// The canonical compose project of an app placement is the identity
	// project keycloakStateIdentity computes ("bh-<target>-<app>-identity
	// -<environment>").
	return bhruntime.ApplicationProjectName(namespace, app.Name+"-identity", app.Environment)
}

// ListKeycloakOwnerProjects recovers the owner->project map from the Keycloak
// state directories under dataDir: each application placement stores its
// manifest identity in "applications/<name>/<environment>", which yields the
// owner token, and its compose project is the canonical application project
// name for that placement. Shared placements contribute no application owner.
// A directory the scan cannot read at all is no state (nil, nil); but an
// application directory whose environment listing fails to read, or that
// carries no environment entry, is a PARTIAL scan and returns an error — the
// legacy migration must never guess a foreign owner's project from partial
// state (card 0428-168, r2 review blocker).
func ListKeycloakOwnerProjects(dataDir, namespace string) (map[string]string, error) {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "keycloak", "applications")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	projects := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		envs, err := os.ReadDir(filepath.Join(root, name))
		if err != nil {
			return nil, fmt.Errorf("keycloak owner state scan %s: %w", name, err)
		}
		if len(envs) == 0 {
			return nil, fmt.Errorf("keycloak owner state scan %s: no environment directory under %s", name, root)
		}
		environment := envs[len(envs)-1].Name()
		projects[name] = bhruntime.ApplicationProjectName(namespace, name+"-identity", environment)
	}
	return projects, nil
}

func (r *localKeycloakRealization) Existing(ctx context.Context) (KeycloakInstance, error) {
	files, err := ExistingKeycloakFilesAt(r.app, r.dataDir, r.namespace)
	if err != nil {
		return KeycloakInstance{}, err
	}
	r.files = files
	publicBase, err := localKeycloakPublicBaseURL(r.app, r.namespace, r.runtime, files)
	if err != nil {
		return KeycloakInstance{}, err
	}
	return r.instance(ctx, files, publicBase)
}

func (r *localKeycloakRealization) Diagnostics(ctx context.Context) string {
	if diagnostics, ok := r.runtime.(identityProjectDiagnostics); ok && r.files.Project != "" {
		return strings.TrimSpace(diagnostics.DiagnosticsProject(ctx, r.files.Project, r.files.Compose, r.files.Env))
	}
	return ""
}

func (r *localKeycloakRealization) Destroy(ctx context.Context) error {
	files := r.files
	if files.Dir == "" {
		var err error
		files, err = ExistingKeycloakFilesAt(r.app, r.dataDir, r.namespace)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	placement, err := application.ResolveProviderPlacement(r.app, capability.ProviderKeycloak)
	if err != nil {
		return err
	}
	if placement.Scope != capability.ScopeApplication {
		return nil
	}
	if err := r.lifecycle.Destroy(ctx, files); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func (r *localKeycloakRealization) instance(ctx context.Context, files KeycloakFiles, publicBase string) (KeycloakInstance, error) {
	publicClient, err := keycloakPublicHTTPClient(files)
	if err != nil {
		return KeycloakInstance{}, err
	}
	adminClient, err := serviceaccess.NewHTTPClient(files.AdminAccess.Material, false)
	if err != nil {
		return KeycloakInstance{}, err
	}
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return KeycloakInstance{}, err
	}
	adminUsername, adminPassword, err := activeKeycloakAdminCredential(files.Dir, values)
	if err != nil {
		return KeycloakInstance{}, err
	}

	workloadBase := files.PublicURL
	trustBundle, err := os.ReadFile(files.PublicAccess.Material.CA)
	if err != nil {
		return KeycloakInstance{}, err
	}
	if isDevelopmentIdentityEnvironment(r.app.Environment) {
		gatewayFiles, err := devgateway.FilesFor(localKeycloakTargetName(r.namespace))
		if err != nil {
			return KeycloakInstance{}, err
		}
		workloadBase = publicBase
		trustBundle, err = os.ReadFile(gatewayFiles.CA)
		if err != nil {
			return KeycloakInstance{}, err
		}
	}

	return KeycloakInstance{
		StateDir:         files.Dir,
		EndpointBaseURL:  files.PublicURL,
		PublicBaseURL:    publicBase,
		WorkloadBaseURL:  workloadBase,
		TrustBundle:      trustBundle,
		PublicHTTPClient: publicClient,
		AdminHTTPClient:  adminClient,
		AdminUsername:    adminUsername,
		AdminPassword:    adminPassword,
	}, nil
}

func localKeycloakTargetName(namespace string) string {
	if value := strings.TrimSpace(namespace); value != "" {
		return value
	}
	return "local"
}

// A first shared consumer must establish the installation server origin before
// applying its realm-local frontend URL. Retained explicit origins stay owned by
// installation reconciliation; native loopback origins migrate through that path.
func prepareApplicationKeycloakCanonicalURL(runtime KeycloakRuntime, namespace string, files KeycloakFiles, publicBase string) error {
	if files.SharedSQL != nil {
		if err := setCoreKeycloakCanonicalURL(runtime, namespace, files); err != nil {
			return err
		}
	}
	return setApplicationKeycloakCanonicalURL(files, publicBase)
}

// Shared application realms use their own frontendUrl attribute. Their
// reconciliation must preserve the installation and master realm authority.
func setApplicationKeycloakCanonicalURL(files KeycloakFiles, publicBase string) error {
	if files.SharedSQL != nil {
		values, err := readProtectedEnv(files.Env)
		if err != nil {
			return err
		}
		publicBase = values["BASEHARBOR_KEYCLOAK_CANONICAL_URL"]
		if strings.TrimSpace(publicBase) == "" {
			publicBase = files.PublicURL
		}
	}
	return SetKeycloakCanonicalURL(files, publicBase)
}

func localKeycloakPublicBaseURL(app application.Manifest, namespace string, runtime KeycloakRuntime, files KeycloakFiles) (string, error) {
	if !isDevelopmentIdentityEnvironment(app.Environment) {
		return files.PublicURL, nil
	}
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return "", err
	}
	target := localKeycloakTargetName(namespace)
	var host string
	switch placement.Scope {
	case capability.ScopeShared:
		host, err = devaccess.SharedHost(target, "identity")
	case capability.ScopeApplication:
		host, err = devaccess.ApplicationHost(target, app.Name, "identity")
	case capability.ScopeExternal:
		return "", errors.New("external OIDC has no managed Keycloak public URL")
	}
	if err != nil {
		return "", err
	}
	return devgateway.URLForRuntime(target, host, runtime), nil
}

func ensureLocalKeycloakDevelopmentRoute(ctx context.Context, app application.Manifest, namespace string, runtime KeycloakRuntime, issuer serviceaccess.Issuer, files KeycloakFiles) error {
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return err
	}
	target := localKeycloakTargetName(namespace)
	var owner, key, host string
	switch placement.Scope {
	case capability.ScopeShared:
		owner = "shared/keycloak"
		key = owner + "/login"
		host, err = devaccess.SharedHost(target, "identity")
	case capability.ScopeApplication:
		owner = "app/" + app.Name + "/" + app.Environment
		key = owner + "/identity"
		host, err = devaccess.ApplicationHost(target, app.Name, "identity")
	case capability.ScopeExternal:
		return errors.New("external OIDC has no managed Keycloak development route")
	}
	if err != nil {
		return err
	}
	route := devgateway.Route{
		Key:        key,
		Host:       host,
		Upstream:   "https://" + devaccess.ProviderAlias(files.Project, "identity") + ":8443",
		Network:    files.ConsumerNetwork,
		TrustFile:  files.PublicAccess.Material.CA,
		ServerName: files.PublicAccess.Material.ServerName,
	}
	return devgateway.UpsertOwnerRoutes(ctx, runtime, issuer, target, owner, []devgateway.Route{route})
}

func keycloakClientSecretPath(instance KeycloakInstance, realm string) string {
	return filepath.Join(instance.StateDir, "scopes", realm, "client-secret")
}
