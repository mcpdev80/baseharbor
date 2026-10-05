package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

type applicationDestroyExecution struct {
	resolved            resolvedApplication
	manifest            application.Manifest
	runtimeProject      string
	resourceProject     string
	term                *cli.Terminal
	out                 io.Writer
	confirmed           bool
	fullReset           bool
	files               application.RuntimeFiles
	runtimeErr          error
	partialRuntime      bool
	compose             bhruntime.RuntimeProvider
	existing            []bhruntime.ProjectResource
	replacedVolumes     []bhruntime.ProjectResource
	platformFiles       bhruntime.Files
	destroyOpenBaoScope bool
}

func newApplicationDestroyExecution(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) (*applicationDestroyExecution, error) {
	filtered, environment, err := extractApplicationEnvironment(args, "destroy")
	if err != nil {
		return nil, err
	}
	name, confirmed, fullReset, err := parseDestroyArgs(filtered)
	if err != nil {
		return nil, err
	}
	var appArgs []string
	if name != "" {
		appArgs = []string{name}
	}
	resolved, err := resolveApplicationEnvironment(ctx, store, appArgs, "destroy", environment)
	if err != nil {
		return nil, err
	}
	m := resolved.Manifest
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return nil, err
	}

	files, runtimeErr := application.ExistingRuntimeFiles(resolved.Store, m)
	partialRuntime := false
	if errors.Is(runtimeErr, application.ErrRuntimeNotApplied) {
		files = application.RuntimeFilesFor(resolved.Store, m)
		partialRuntime = true
	} else if runtimeErr != nil {
		return nil, runtimeErr
	}

	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(m.Name, m.Environment)
	term.Info("target", resolved.Target.Name)

	return &applicationDestroyExecution{
		resolved:        resolved,
		manifest:        m,
		runtimeProject:  application.RuntimeComposeProjectNameForStore(resolved.Store, m),
		resourceProject: application.RuntimeProjectNameForStore(resolved.Store, m),
		term:            term,
		out:             out,
		confirmed:       confirmed,
		fullReset:       fullReset,
		files:           files,
		runtimeErr:      runtimeErr,
		partialRuntime:  partialRuntime,
	}, nil
}

func (e *applicationDestroyExecution) runPreflight(ctx context.Context) error {
	m := e.manifest
	composeRequired := e.runtimeErr == nil || e.resolved.FromRepository || m.Services.Secrets || application.HasManagedRuntimeServices(m) || application.HasIdentity(m)
	checks := []preflight.Check{
		{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
		{Name: "connectivity policy", Run: func(context.Context) error {
			return application.CheckApplicationConnectivityReleasedAt(e.resolved.TargetStateRoot, m)
		}},
		{Name: "manifest permissions", Run: func(context.Context) error {
			return checkManifestPermissions(e.resolved.ManifestPath, e.resolved.FromRepository)
		}},
		{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(e.resolved) }},
	}
	if composeRequired {
		checks = append(checks, preflight.Check{Name: "runtime orchestration", Run: func(ctx context.Context) error {
			var err error
			e.compose, err = detectRuntimeForApplication(ctx, e.resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityResourceOwnership)
			return err
		}})
	}
	if e.resolved.FromRepository && application.HasManagedRuntimeServices(m) {
		checks = append(checks, preflight.Check{Name: "replaced repository infrastructure volumes", Run: func(ctx context.Context) error {
			var err error
			e.replacedVolumes, err = e.inspectReclaimableReplacedInfrastructureVolumes(ctx)
			return err
		}})
	}
	if e.runtimeErr == nil {
		checks = append(checks,
			preflight.Check{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(e.files) }},
			preflight.Check{Name: "runtime configuration", Run: func(ctx context.Context) error {
				return e.compose.ConfigProject(ctx, e.runtimeProject, e.files.Compose, e.files.Env)
			}},
		)
	}
	if application.HasManagedRuntimeServices(m) {
		checks = append(checks, preflight.Check{Name: "runtime ownership", Run: func(ctx context.Context) error {
			var err error
			e.existing, err = e.compose.InspectProjectResources(ctx, e.runtimeProject, application.ExpectedRuntimeResourcesForIdentity(m, e.runtimeProject, e.resourceProject))
			return err
		}})
	}
	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		checks = append(checks,
			preflight.Check{Name: "OpenBao cleanup state", Run: func(ctx context.Context) error {
				var err error
				e.platformFiles, err = existingTargetRuntimeFiles(ctx)
				if err != nil {
					return err
				}
				state, err := openbao.Inspect(ctx, e.compose, e.platformFiles)
				if err != nil {
					return err
				}
				e.destroyOpenBaoScope, err = openBaoDestroyScopeRequired(state)
				return err
			}},
			preflight.Check{Name: "OpenBao application scope ownership", Run: func(ctx context.Context) error {
				if !e.destroyOpenBaoScope {
					return nil
				}
				return openbao.CheckApplicationScopeOwnership(ctx, e.compose, e.platformFiles, identity)
			}},
		)
	}

	results, ok := preflight.RunWithTimeout(ctx, checks, 30*time.Second)
	renderPreflightUX(e.term, results)
	if !ok {
		return errors.New("application destroy preflight failed; nothing was deleted")
	}
	return nil
}

func (e *applicationDestroyExecution) inspectReclaimableReplacedInfrastructureVolumes(ctx context.Context) ([]bhruntime.ProjectResource, error) {
	if !e.resolved.FromRepository {
		return nil, nil
	}
	repositoryRoot := e.resolved.repositoryRoot()
	selected, composePath, found, err := application.SelectedWorkloadServices(repositoryRoot, e.manifest)
	if err != nil || !found {
		return nil, err
	}
	environment, err := repositoryWorkloadStopEnvironment(e.resolved, e.files)
	if err != nil {
		return nil, err
	}
	project := application.WorkloadProjectNameForRuntime(e.manifest, e.files)
	rendered, err := e.compose.ConfigJSONProjectFilesEnv(ctx, project, repositoryRoot, environment, composePath)
	if err != nil {
		return nil, fmt.Errorf("render repository Compose for replaced-volume ownership: %w", err)
	}
	candidates, err := repositoryinspect.ReclaimableReplacedInfrastructureVolumes(
		[]byte(rendered),
		selected,
		e.manifest.Services.SQL,
		e.manifest.Services.Cache,
	)
	if err != nil {
		return nil, err
	}
	resources := make([]bhruntime.ProjectResource, 0, len(candidates))
	for _, candidate := range candidates {
		name := candidate.RuntimeName
		if name == "" {
			name = project + "_" + candidate.LogicalName
		}
		resources = append(resources, bhruntime.ProjectResource{Kind: "volume", Name: name})
	}
	return e.compose.InspectProjectResources(ctx, project, resources)
}

func (e *applicationDestroyExecution) renderDeletePlan() error {
	m := e.manifest
	e.term.Section("Delete plan")

	if e.partialRuntime {
		fmt.Fprintln(e.out, "  recovery:   generated runtime definition is incomplete; using ownership-verified cleanup")
	}
	if len(e.existing) == 0 {
		fmt.Fprintln(e.out, "  runtime resources: none currently present")
	} else {
		for _, resource := range e.existing {
			fmt.Fprintf(e.out, "  %-10s %s\n", resource.Kind+":", resource.Name)
		}
	}
	if e.resolved.FromRepository {
		if workload, found, err := materializeRepositoryWorkload(e.resolved, e.files); err == nil && found {
			fmt.Fprintf(e.out, "  workload:   %s (containers stopped; application-owned volumes preserved)\n", workload.Compose)
		}
		for _, volume := range e.replacedVolumes {
			fmt.Fprintf(e.out, "  volume:     %s (reclaimed; belongs only to replaced repository infrastructure)\n", volume.Name)
		}
	} else if e.resolved.IncompleteDeployment {
		fmt.Fprintln(e.out, "  repo data:  preserved; repository source is unavailable, so replaced-service volumes cannot be classified safely")
	}
	if m.Services.Secrets {
		if e.destroyOpenBaoScope {
			fmt.Fprintf(e.out, "  secrets:    baseharbor/apps/%s/%s\n", m.Name, m.Environment)
		} else {
			fmt.Fprintln(e.out, "  secrets:    current OpenBao is uninitialized; no application scope exists to delete")
		}
		fmt.Fprintln(e.out, "  broker:     per-application mTLS Application Runtime Broker")
	}
	if len(m.Exposures) > 0 {
		fmt.Fprintf(e.out, "  exposure:   %d BaseHarbor-managed HTTP route(s) via application-scoped Caddy provider\n", len(m.Exposures))
	}
	if application.HasSharedBackends(m) {
		fmt.Fprintln(e.out, "  data:       application-owned logical SQL/cache resources removed; shared Target provider infrastructure preserved while still in use")
	}
	if application.HasIdentity(m) {
		fmt.Fprintln(e.out, "  identity:   BaseHarbor-owned application/environment identity scope removed; shared provider infrastructure preserved")
	}
	if e.resolved.FromRepository {
		if policy, policyErr := application.LogsPolicy(m); policyErr == nil && policy.Enabled && policy.Collect[application.LogsSourceApplication] {
			fmt.Fprintln(e.out, "  logs:       application log registration and BaseHarbor-owned collector state removed according to placement")
		}
	}
	if len(m.Metrics.Sources) > 0 || application.HasRuntimeMetricsPermissions(m) {
		placement, found, err := application.RegisteredProviderPlacementAt(e.resolved.TargetStateRoot, m, capability.ProviderPrometheus)
		if err != nil {
			return err
		}
		if found {
			fmt.Fprintf(e.out, "  metrics:    registered provider placement %s; application-owned metrics state/trust edges removed according to placement\n", placement.Scope)
		} else {
			fmt.Fprintln(e.out, "  metrics:    no registered provider placement; no provider lifecycle ownership will be assumed")
		}
	}

	appDir := filepath.Join(e.resolved.Store.Root, m.Name)
	fmt.Fprintf(e.out, "  state:      %s\n", appDir)
	if e.resolved.FromRepository {
		fmt.Fprintf(e.out, "  manifest:   %s (preserved)\n", e.resolved.ManifestPath)
		if e.fullReset {
			fmt.Fprintf(e.out, "  deployment: %s (removed by --full-reset)\n", repositoryInitEnvPathFromStateRoot(e.resolved.stateRoot()))
			fmt.Fprintf(e.out, "  local TLS:  %s (removed by --full-reset; external certificate source is never touched)\n", filepath.Join(e.resolved.stateRoot(), repositoryTLSDirName))
		} else {
			fmt.Fprintf(e.out, "  deployment: %s (preserved)\n", repositoryInitEnvPathFromStateRoot(e.resolved.stateRoot()))
			fmt.Fprintf(e.out, "  local TLS:  %s (preserved)\n", filepath.Join(e.resolved.stateRoot(), repositoryTLSDirName))
		}
	}
	return nil
}

func (e *applicationDestroyExecution) destroyRuntimeResources(ctx context.Context) error {
	m := e.manifest
	if e.runtimeErr == nil {
		if err := destroyManagedExposure(ctx, e.compose, m, e.files); err != nil {
			return err
		}
	}
	if e.resolved.FromRepository {
		if _, err := stopRepositoryWorkload(ctx, e.compose, e.resolved, e.files); err != nil {
			return err
		}
		if _, err := destroyRepositoryWorkloadRuntime(ctx, e.compose, e.resolved, e.files); err != nil {
			return err
		}
		if len(e.replacedVolumes) > 0 {
			project := application.WorkloadProjectNameForRuntime(m, e.files)
			if err := e.compose.DestroyOwnedProjectResources(ctx, project, e.replacedVolumes); err != nil {
				return fmt.Errorf("reclaim replaced repository infrastructure volumes: %w", err)
			}
			remaining, err := e.compose.InspectProjectResources(ctx, project, e.replacedVolumes)
			if err != nil {
				return fmt.Errorf("verify replaced repository infrastructure volume cleanup: %w", err)
			}
			if len(remaining) != 0 {
				return fmt.Errorf("verify replaced repository infrastructure volume cleanup: %d volume(s) remain", len(remaining))
			}
		}
	}
	if e.runtimeErr == nil && application.RequiresRuntimeBroker(m) {
		if err := destroyRuntimeBroker(ctx, e.compose, m, e.files); err != nil {
			return err
		}
	}
	sharedBackendsRegistered := false
	for _, provider := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		placement, found, err := application.RegisteredProviderPlacementAt(e.resolved.TargetStateRoot, m, provider)
		if err != nil {
			return fmt.Errorf("inspect registered %s placement before destroy: %w", provider, err)
		}
		if found && placement.Scope == capability.ScopeShared {
			sharedBackendsRegistered = true
		}
	}
	if sharedBackendsRegistered {
		if err := application.ReleaseSharedBackendApplication(
			ctx,
			e.compose,
			e.resolved.TargetStateRoot,
			e.resolved.Target.Name,
			m,
		); err != nil {
			return fmt.Errorf("release application resources from shared data providers: %w", err)
		}
	}
	if e.runtimeErr == nil {
		if err := e.compose.DestroyProject(ctx, e.runtimeProject, e.files.Compose, e.files.Env); err != nil {
			return err
		}
	} else if e.partialRuntime && len(e.existing) != 0 {
		if err := e.compose.DestroyOwnedProjectResources(ctx, e.runtimeProject, application.ExpectedRuntimeResourcesForIdentity(m, e.runtimeProject, e.resourceProject)); err != nil {
			return fmt.Errorf("recover incomplete application runtime destruction: %w", err)
		}
	}
	if application.HasManagedRuntimeServices(m) {
		remaining, err := e.compose.InspectProjectResources(ctx, e.runtimeProject, application.ExpectedRuntimeResourcesForIdentity(m, e.runtimeProject, e.resourceProject))
		if err != nil {
			return fmt.Errorf("verify application runtime destruction: %w", err)
		}
		if len(remaining) != 0 {
			return fmt.Errorf("verify application runtime destruction: %d managed resources remain", len(remaining))
		}
	}
	if application.HasObjectStorage(m) {
		driver := objectstorage.NewDriverAt(e.compose, m, e.files, nil, e.resolved.TargetStateRoot, e.resolved.Target.Name)
		for _, bucket := range application.ObjectStorageBucketNames(m) {
			if err := driver.DestroyBucket(ctx, bucket); err != nil {
				return fmt.Errorf("destroy managed S3 bucket %s: %w", bucket, err)
			}
		}
		if err := objectstorage.UnregisterManagementUIConsumerAt(e.resolved.TargetStateRoot, e.resolved.Target.Name, m); err != nil {
			return fmt.Errorf("remove object-storage management UI registration: %w", err)
		}
	}
	if m.Services.Secrets && e.destroyOpenBaoScope {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		if err := openbao.DestroyVerifiedApplicationScope(ctx, e.compose, e.platformFiles, identity); err != nil {
			return fmt.Errorf("destroy OpenBao application scope after runtime removal: %w", err)
		}
	}
	return nil
}

func (e *applicationDestroyExecution) cleanupProviderState(ctx context.Context) error {
	if err := e.cleanupIdentity(ctx); err != nil {
		return err
	}
	if err := e.cleanupLogs(ctx); err != nil {
		return err
	}
	if err := e.cleanupTraces(ctx); err != nil {
		return err
	}
	return e.cleanupMetrics(ctx)
}

func (e *applicationDestroyExecution) cleanupIdentity(ctx context.Context) error {
	if !application.HasIdentity(e.manifest) {
		return nil
	}

	_, keycloakFound, err := application.RegisteredProviderPlacementAt(
		e.resolved.TargetStateRoot, e.manifest, capability.ProviderKeycloak,
	)
	if err != nil {
		return err
	}
	if keycloakFound {
		return e.cleanupKeycloakIdentity(ctx)
	}

	if _, found, err := application.RegisteredProviderPlacementAt(
		e.resolved.TargetStateRoot, e.manifest, capability.ProviderExternalOIDC,
	); err != nil {
		return err
	} else if found {
		return nil
	}

	providerPath := filepath.Join(e.files.Bindings, application.IdentityBindingName, "provider")
	data, err := os.ReadFile(providerPath)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("identity provider ownership cannot be determined safely; refusing destroy")
	}
	if err != nil {
		return fmt.Errorf("inspect identity provider ownership before destroy: %w", err)
	}
	switch capability.ProviderKind(strings.TrimSpace(string(data))) {
	case capability.ProviderKeycloak:
		return e.cleanupKeycloakIdentity(ctx)
	case capability.ProviderExternalOIDC:
		return nil
	default:
		return fmt.Errorf("identity provider ownership is unsupported or ambiguous")
	}
}

func (e *applicationDestroyExecution) cleanupKeycloakIdentity(ctx context.Context) error {
	var issuer serviceaccess.Issuer
	if devaccess.Enabled(e.manifest.Environment) {
		if e.platformFiles.Compose == "" {
			var err error
			e.platformFiles, err = existingTargetRuntimeFiles(ctx)
			if err != nil {
				return fmt.Errorf("load managed trust plane for identity cleanup: %w", err)
			}
		}
		issuer = openbao.NewServiceIssuer(e.compose, e.platformFiles)
	}
	driver := identityprovider.NewKeycloakDriver(
		e.compose, e.manifest, e.files, issuer,
		e.resolved.TargetStateRoot, e.resolved.Target.Name,
	)
	if err := driver.DestroyApplication(ctx); err != nil {
		return fmt.Errorf("destroy managed identity scope: %w", err)
	}
	return nil
}

func (e *applicationDestroyExecution) cleanupLogs(ctx context.Context) error {
	if !e.resolved.FromRepository {
		return nil
	}
	if e.platformFiles.Compose == "" {
		var err error
		e.platformFiles, err = existingTargetRuntimeFiles(ctx)
		if err != nil {
			return fmt.Errorf("load managed trust plane for log collector cleanup: %w", err)
		}
	}
	var issuer serviceaccess.Issuer = openbao.NewServiceIssuer(e.compose, e.platformFiles)
	if err := logsprovider.UnregisterApplicationAt(ctx, e.compose, issuer, e.resolved.TargetStateRoot, e.resolved.Target.Name, e.manifest); err != nil {
		return fmt.Errorf("remove application log collector registration: %w", err)
	}
	if err := logsprovider.RemoveWorkloadOverride(e.files); err != nil {
		return fmt.Errorf("remove workload logging override: %w", err)
	}
	if err := logsprovider.RemoveProviderSourceOverride(e.files); err != nil {
		return fmt.Errorf("remove provider logging override: %w", err)
	}
	return nil
}

func (e *applicationDestroyExecution) cleanupTraces(ctx context.Context) error {
	placement, found, err := application.RegisteredProviderPlacementAt(e.resolved.TargetStateRoot, e.manifest, capability.ProviderTempo)
	if err != nil {
		return err
	}
	if found && placement.Scope == capability.ScopeApplication {
		if err := tracesprovider.DestroyProviderAt(ctx, e.compose, e.manifest, e.resolved.TargetStateRoot, e.resolved.Target.Name); err != nil {
			return fmt.Errorf("destroy application-scoped traces provider: %w", err)
		}
	}
	return nil
}

func (e *applicationDestroyExecution) cleanupMetrics(ctx context.Context) error {
	placement, found, err := application.RegisteredProviderPlacementAt(e.resolved.TargetStateRoot, e.manifest, capability.ProviderPrometheus)
	if err != nil || !found {
		return err
	}

	switch placement.Scope {
	case capability.ScopeShared:
		if err := metricsprovider.PruneRegisteredApplicationTargetsAt(e.resolved.TargetStateRoot, e.resolved.Target.Name, e.manifest, nil); err != nil {
			return fmt.Errorf("remove application metrics targets: %w", err)
		}
		if e.platformFiles.Compose == "" {
			e.platformFiles, err = existingTargetRuntimeFiles(ctx)
			if err != nil {
				return fmt.Errorf("load managed trust plane for metrics cleanup: %w", err)
			}
		}
		issuer := openbao.NewServiceIssuer(e.compose, e.platformFiles)
		if err := metricsprovider.UnregisterSharedApplicationAt(ctx, e.compose, issuer, e.resolved.TargetStateRoot, e.resolved.Target.Name, e.manifest); err != nil {
			return fmt.Errorf("remove application metrics trust edges: %w", err)
		}
	case capability.ScopeApplication:
		if err := metricsprovider.DestroyProviderAt(ctx, e.compose, e.resolved.TargetStateRoot, e.resolved.Target.Name, e.manifest); err != nil {
			return fmt.Errorf("destroy application-scoped metrics provider: %w", err)
		}
	case capability.ScopeExternal:
	}
	return nil
}

func (e *applicationDestroyExecution) cleanupDevelopmentCanonicalRoutes(ctx context.Context) error {
	if !devaccess.Enabled(e.manifest.Environment) {
		return nil
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return fmt.Errorf("load target runtime for development gateway cleanup: %w", err)
	}
	issuer := openbao.NewServiceIssuer(e.compose, files)
	appOwner := "app/" + e.manifest.Name + "/" + e.manifest.Environment
	if err := devgateway.RemoveOwners(
		ctx,
		e.compose,
		issuer,
		e.resolved.Target.Name,
		appOwner,
	); err != nil {
		return fmt.Errorf("remove canonical development routes: %w", err)
	}
	return nil
}

func (e *applicationDestroyExecution) removeApplicationState() error {
	appDir := filepath.Join(e.resolved.Store.Root, e.manifest.Name)
	if _, err := os.Stat(appDir); err == nil {
		if err := e.resolved.Store.Delete(e.manifest.Name); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect application state before removal: %w", err)
	}

	if e.fullReset && e.resolved.FromRepository {
		if err := os.Remove(repositoryInitEnvPathFromStateRoot(e.resolved.stateRoot())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove repository deployment state: %w", err)
		}
		if err := os.RemoveAll(filepath.Join(e.resolved.stateRoot(), repositoryTLSDirName)); err != nil {
			return fmt.Errorf("remove normalized repository TLS state: %w", err)
		}
	}
	if err := application.ReleaseApplicationProviderRegistryAt(e.resolved.TargetStateRoot, e.manifest); err != nil {
		return fmt.Errorf("application resources were destroyed but provider registry cleanup failed: %w", err)
	}
	if _, err := os.Stat(appDir); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return errors.New("verify application destruction: application state still exists")
		}
		return fmt.Errorf("verify application destruction: %w", err)
	}
	if err := deployment.DeleteDeploymentRecord(e.resolved.DeploymentIdentity); err != nil {
		return fmt.Errorf("remove target deployment record after verified destruction: %w", err)
	}

	e.term.Section("Application")
	e.term.Result("DELETED", "application", e.manifest.Name+" permanently deleted")
	if e.resolved.FromRepository {
		e.term.Info("repository", "baseharbor.yaml and application-owned Compose data preserved; use 'baha app apply' to recreate")
	}
	return nil
}
