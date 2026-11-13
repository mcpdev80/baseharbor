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
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/machine"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
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
	incompleteInventory bool
	compose             bhruntime.RuntimeProvider
	existing            []bhruntime.ProjectResource
	repositoryVolumes   []bhruntime.RepositoryVolume
	coreRuntime         bhruntime.RuntimeProvider
	platformFiles       bhruntime.Files
	destroyOpenBaoScope bool
	// selection carries the explicit deployment-id selection of the command
	// invocation (the uuid-addressed repair) down to the mutation seam, which
	// refuses to delete anything but the record that was selected (0427-175).
	selection []string
}

// finishUninitializedRepositoryDestroy makes the common "already removed / not
// initialized" case idempotent. A repository manifest by itself does not
// establish ownership of runtime resources, so this succeeds only when the
// selected Target has no matching deployment and neither the repository nor
// any registered Target claims an application identity for this source. It
// deliberately preserves authored source files and never guesses at runtime
// resources whose owner cannot be proven.
func finishUninitializedRepositoryDestroy(ctx context.Context, target deployment.ResolvedTarget, args []string, format cliOutputFormat, out io.Writer) (bool, error) {
	filtered, environment, err := extractApplicationEnvironment(args, "destroy")
	if err != nil {
		return false, err
	}
	name, _, _, err := parseDestroyArgs(filtered)
	if err != nil {
		return false, err
	}
	if name != "" || strings.TrimSpace(environment) != "" || strings.TrimSpace(applicationEnvironmentOverride) != "" {
		return false, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("resolve current directory: %w", err)
	}
	selection, err := application.ResolveRepositoryEnvironment(cwd, "")
	if errors.Is(err, application.ErrRepositoryManifestNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	owner := application.Store{Root: filepath.Join(selection.RepositoryRoot, ".baseharbor", "apps")}
	snapshot, err := repositoryIdentityClaims(target, selection.RepositoryRoot, owner, selection.Manifest)
	if err != nil {
		return false, err
	}
	if snapshot.PreviouslyRegistered || len(snapshot.ApplicationIDs) != 0 {
		return false, nil
	}
	records, warnings, err := deployment.ListDeploymentsForDisplay(target.Name)
	if err != nil || len(warnings) != 0 {
		// A damaged deployment inventory must keep using the normal recovery path;
		// it is not evidence that the app has nothing to delete.
		return false, err
	}
	for _, record := range records {
		if record.Identity.Application == selection.Manifest.Name && record.Identity.Environment == selection.Manifest.Environment {
			return false, nil
		}
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return false, err
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(targetRoot, "provider-registry.json")}).Load()
	if err != nil {
		return false, err
	}
	for _, instance := range registry.Instances {
		if instance.OwnerApplication == selection.Manifest.Name && instance.OwnerEnvironment == selection.Manifest.Environment {
			return false, nil
		}
	}
	detail := "No application identity or deployment is registered on Target " + target.Name + "; there is no BaseHarbor-owned application state to remove. Repository source files were kept."
	if format == outputJSON {
		result := applicationlifecycle.NewResult("destroy", selection.Manifest.Name, selection.Manifest.Environment, "absent", true).WithTarget(target.Name)
		result.Detail = detail
		return true, writeJSON(out, result)
	}
	fmt.Fprintln(out, "Application already absent")
	fmt.Fprintln(out, "  "+detail)
	return true, nil
}

func newApplicationDestroyExecution(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) (*applicationDestroyExecution, error) {
	// --uuid DEPLOYMENT-ID is the identity the registry-conflict repair
	// advertises ('baha app destroy --uuid <id> --full-reset', 0427-175), so it
	// must be routed to the resolver instead of dropped. extractApplicationUUID
	// reads the selector from the raw command arguments; the positional scan
	// uses the same list with the whole option pair removed, because the
	// selector VALUE is an identifier, never a NAME (the environment splitter
	// consumes only the FLAG and leaves the bare VALUE behind, app_context.go).
	// NAME and --uuid select through different seams and a combination could
	// name two deployments (0427-149/0427-175).
	filtered, environment, err := extractApplicationEnvironment(args, "destroy")
	if err != nil {
		return nil, err
	}
	// The selector is read from the SAME arguments the resolver below receives,
	// so the command and the resolver can never disagree about the identity
	// (0427-175). A malformed or value-less selector fails here with the shared
	// usage error, before anything is resolved.
	selectorUUID, uuidErr := extractApplicationUUID(args)
	if uuidErr != nil {
		return nil, uuidErr
	}
	name, confirmed, fullReset, err := parseDestroyArgs(filtered)
	if err != nil {
		return nil, err
	}
	if isApplicationStopMode(ctx) {
		fullReset = false
	}
	// NAME and --uuid select through different seams and are mutually exclusive:
	// combining them could name two deployments (0427-149/0427-175).
	if name != "" && selectorUUID != "" {
		return nil, usageError("baha app destroy accepts either NAME or --uuid DEPLOYMENT-ID, not both", "Address the twin to retire by uuid, or the application by NAME.")
	}
	// The resolver must receive the selector as an intact option pair: --uuid
	// routes at the registry seam, whose record is the authoritative identity,
	// so authorization, preflight and mutation below keep working on the
	// SELECTED twin and never on a same-named sibling (0427-175). Without a
	// selector the arguments keep their previous positional-name behaviour
	// verbatim.
	var appArgs []string
	switch {
	case selectorUUID != "":
		appArgs = []string{"--uuid", selectorUUID}
	case name != "":
		appArgs = []string{name}
	}
	// The selected deployment id travels with the call so that authorization,
	// preflight and the mutation below all act on THAT record and on nothing
	// else (0427-175).
	resolved, err := resolveApplicationEnvironment(ctx, store, appArgs, "destroy", environment, selectorUUID)
	if err != nil {
		return nil, err
	}
	m := resolved.Manifest
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return nil, err
	}

	if isRemoteApplication(resolved) {
		return &applicationDestroyExecution{resolved: resolved, manifest: m, term: cli.NewTerminal(ctx, out, errOut), out: out, confirmed: confirmed, fullReset: fullReset, selection: appArgs}, nil
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
		selection:       appArgs,
	}, nil
}

func (e *applicationDestroyExecution) runPreflight(ctx context.Context) error {
	// Reject a changed installation before any provider cleanup can begin.
	selected := withTargetOverride(ctx, e.resolved.Target.Name)
	if _, _, err := applicationCoreTarget(selected); err != nil {
		return err
	}
	current, err := effectiveTarget(selected)
	if err != nil {
		return err
	}
	if current != e.resolved.Target {
		return machine.NewError(machine.ErrorPolicyDenied,
			"Application Target binding changed during execution.", "Resolve the selected Target again before retrying.", false)
	}
	m := e.manifest
	composeRequired := devaccess.Enabled(m.Environment) || e.runtimeErr == nil || e.resolved.FromRepository || m.Services.Secrets || application.HasManagedRuntimeServices(m) || application.HasIdentity(m)
	checks := []preflight.Check{
		{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
		{Name: "connectivity policy", Run: func(context.Context) error {
			return application.CheckApplicationConnectivityReleasedAt(e.resolved.TargetStateRoot, m)
		}},
		{Name: "manifest permissions", Run: func(context.Context) error {
			return checkManifestPermissions(e.resolved.ManifestPath, e.resolved.FromRepository)
		}},
		applicationWorkloadContractCheck(e.resolved),
	}
	if !e.incompleteInventory {
		checks = append(checks, preflight.Check{Name: "retained provider ownership", Run: func(ctx context.Context) error {
			return checkRetainedApplicationProviderOwnership(ctx, e.resolved)
		}})
	}
	if composeRequired {
		checks = append(checks, preflight.Check{Name: "runtime orchestration", Run: func(ctx context.Context) error {
			var err error
			e.compose, err = detectRuntimeForApplication(ctx, e.resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityResourceOwnership)
			return err
		}})
	}
	if e.resolved.FromRepository {
		checks = append(checks, preflight.Check{Name: "repository data volume inventory", Run: func(ctx context.Context) error {
			inventory, ok := e.compose.(repositoryVolumeInventory)
			if !ok {
				return errors.New("selected runtime cannot inventory repository data volumes; destroy refused")
			}
			previous, err := e.previousRepositoryVolumeNames()
			if err != nil {
				return err
			}
			e.repositoryVolumes, err = inventory.InventoryRepositoryVolumes(ctx, application.WorkloadProjectNameForRuntime(m, e.files), previous...)
			return err
		}})
	}

	if e.runtimeErr == nil {
		checks = append(checks,
			preflight.Check{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(e.files) }},
			preflight.Check{Name: "runtime configuration", Run: func(ctx context.Context) error {
				if !application.HasApplicationScopedRuntimeServices(m) {
					return nil
				}
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
	if e.fullReset && m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		checks = append(checks,
			preflight.Check{Name: "OpenBao cleanup state", Run: func(ctx context.Context) error {
				var err error
				if e.runtimeErr != nil && len(e.existing) == 0 {
					state, err := managedTrustCoreState(selected)
					if err != nil {
						return err
					}
					if state == "not_installed" {
						root, err := targetRuntimeStateRoot(e.resolved.Target)
						if err != nil {
							return err
						}
						entries, err := os.ReadDir(root)
						if err != nil && !errors.Is(err, os.ErrNotExist) {
							return err
						}
						if len(entries) > 0 {
							return errors.New("Core cleanup state is incomplete; inspect retained state before retrying destroy")
						}
						resources, err := e.compose.ListOwnedProjectResources(ctx, targetRuntimeProjectName(e.resolved.Target))
						if err != nil {
							return err
						}
						if len(resources) > 0 {
							return errors.New("Core resources remain without runtime state; repair ownership state before retrying destroy")
						}
						if _, err := os.Stat(e.files.Bindings); err == nil {
							return errors.New("application bindings remain without a Core; repair cleanup state before retrying destroy")
						} else if !errors.Is(err, os.ErrNotExist) {
							return err
						}
						return nil
					}
				}
				e.coreRuntime, e.platformFiles, err = resolveApplicationCoreRuntime(ctx, e.resolved, e.compose)
				if err != nil {
					return err
				}
				state, err := openbao.Inspect(ctx, e.coreRuntime, e.platformFiles)
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
				return openbao.CheckApplicationScopeOwnershipForDestroy(ctx, e.coreRuntime, e.platformFiles, identity)
			}},
		)
	}

	results, ok := preflight.RunWithTimeout(ctx, checks, 30*time.Second)
	renderPreflightUX(e.term, results)
	if !ok {
		return applicationDestroyPreflightError(results)
	}
	e.excludeManagedInfrastructureVolumes()
	return nil
}

func applicationDestroyPreflightError(results []preflight.Result) error {
	failed := make([]string, 0)
	for _, result := range results {
		if result.OK {
			continue
		}
		detail := strings.TrimSpace(result.Detail)
		if len(detail) > 200 {
			detail = detail[:197] + "..."
		}
		if detail == "" {
			failed = append(failed, result.Name)
		} else {
			failed = append(failed, result.Name+": "+detail)
		}
	}
	if len(failed) == 0 {
		return errors.New("application destroy preflight failed; nothing was deleted")
	}
	return fmt.Errorf("application destroy preflight failed; nothing was deleted (%s)", strings.Join(failed, "; "))
}

func (e *applicationDestroyExecution) renderDeletePlan() error {
	m := e.manifest
	e.term.Section("Delete plan")
	if e.incompleteInventory {
		fmt.Fprintln(e.out, "  partial cleanup: Target consumer inventory is damaged; unknown and shared provider resources will be retained")
	}
	if e.resolved.FromRepository {
		project := application.WorkloadProjectNameForRuntime(m, e.files)
		renderRepositoryDestroyVolumes(e.out, e.repositoryVolumes, project)
	}

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
		// Planning displays the selected source. Materialization would rewrite
		// protected runtime metadata before consent and invalidate this plan.
		if source, err := selectedRepositoryComposeSource(e.resolved.repositoryRoot(), e.manifest); err == nil && source != "" {
			fmt.Fprintf(e.out, "  workload:   %s (containers stopped; application-owned volumes preserved)\n", filepath.Join(e.resolved.repositoryRoot(), source))
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
		fmt.Fprintf(e.out, "  deployment: %s (removed)\n", repositoryInitEnvPathFromStateRoot(e.resolved.stateRoot()))
		fmt.Fprintf(e.out, "  local TLS:  %s (removed; external certificate source is never touched)\n", filepath.Join(e.resolved.stateRoot(), repositoryTLSDirName))
	}
	return nil
}

func (e *applicationDestroyExecution) destroyRuntimeResources(ctx context.Context) error {
	if !e.confirmed {
		return errors.New("application destroy requires explicit --yes consent; nothing was deleted")
	}
	m := e.manifest
	if e.runtimeErr == nil {
		if err := destroyManagedExposure(ctx, e.compose, m, e.files); err != nil {
			return err
		}
	}
	if e.resolved.FromRepository {
		if _, err := destroyRepositoryWorkloadStop(ctx, e.compose, e.resolved, e.files); err != nil {
			return err
		}
		if _, err := destroyRepositoryWorkloadRuntime(ctx, e.compose, e.resolved, e.files); err != nil {
			return err
		}
		if err := e.destroyRepositoryDataVolumes(ctx); err != nil {
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
	if e.runtimeErr == nil && application.HasApplicationScopedRuntimeServices(m) {
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
		if err := openbao.DestroyVerifiedApplicationScope(ctx, e.coreRuntime, e.platformFiles, identity); err != nil {
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
		var err error
		issuer, err = resolveApplicationServiceIssuer(ctx, e.resolved, e.compose)
		if err != nil {
			return fmt.Errorf("load selected trust authority for identity cleanup: %w", err)
		}
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
	registered, err := logsprovider.ApplicationRegisteredAt(e.resolved.TargetStateRoot, e.resolved.Target.Name, e.manifest)
	if err != nil {
		return fmt.Errorf("inspect application log registration before cleanup: %w", err)
	}
	if !registered {
		if err := logsprovider.RemoveWorkloadOverride(e.files); err != nil {
			return err
		}
		return logsprovider.RemoveProviderSourceOverride(e.files)
	}
	if e.platformFiles.Compose == "" {
		var err error
		e.coreRuntime, e.platformFiles, err = resolveApplicationCoreRuntime(ctx, e.resolved, e.compose)
		if err != nil {
			return fmt.Errorf("load managed trust plane for log collector cleanup: %w", err)
		}
	}
	var issuer serviceaccess.Issuer = openbao.NewServiceIssuer(e.coreRuntime, e.platformFiles)
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
			e.coreRuntime, e.platformFiles, err = resolveApplicationCoreRuntime(ctx, e.resolved, e.compose)
			if err != nil {
				return fmt.Errorf("load managed trust plane for metrics cleanup: %w", err)
			}
		}
		issuer := openbao.NewServiceIssuer(e.coreRuntime, e.platformFiles)
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

func (e *applicationDestroyExecution) removeApplicationState() error {
	// The selector-vs-resolved drift guard now runs earlier, in the
	// resolve-and-authorize phase of executeApplicationDestroyLifecycle, before
	// ANY cleanup (routes, runtime, provider) and before this seam — see
	// app_lifecycle.go. A check that only sits here would be after the very
	// mutations it must prevent, so it is intentionally absent from this seam.
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
	if e.resolved.FromRepository {
		if err := os.Remove(e.repositoryVolumeObservationPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove obsolete preserved-volume observation: %w", err)
		}
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
		e.term.Info("repository", "authored source files are preserved; application runtime data and BaseHarbor state were deleted")
	}
	return nil
}

// assertSelectionResolvedIdentity fails closed when the operator-visible
// --uuid selector does not match the resolved deployment identity. It is the
// selector-vs-resolved guard (0427-175), now executed in the resolve-and-
// authorize phase of executeApplicationDestroyLifecycle — BEFORE the
// cleanupDevelopmentCanonicalRoutes / destroyRuntimeResources /
// cleanupProviderState cleanup and before the DeploymentRecord removal in
// removeApplicationState — so a selection that drifted onto another twin can
// never remove a record the operator did not address. An absent selector
// (ordinary NAME/destroy) keeps the guard inert.
func (e *applicationDestroyExecution) assertSelectionResolvedIdentity() error {
	selectedID := selectionDeploymentID(e.selection)
	if selectedID == "" {
		return nil
	}
	if e.resolved.DeploymentIdentity.DeploymentID != selectedID {
		return fmt.Errorf("deployment selection %s did not route to the resolved record %s; refusing to delete either twin", selectedID, e.resolved.DeploymentIdentity.DeploymentID)
	}
	return nil
}

// selectionDeploymentID reads the explicit deployment-id selector out of a
// destroy command's raw arguments (`--uuid <id>`), the form the CLI entry
// forwards to the resolver. It complements selectedDeploymentIDFromArguments,
// which only recognizes the internal `deployment-id:` routing slot: the guard
// in removeApplicationState must see the operator-visible selector so it can
// compare it against the resolved identity and fail closed on drift. An absent
// selector returns "" so the guard stays inert for ordinary NAME/destroys.
func selectionDeploymentID(command []string) string {
	for i := 0; i < len(command)-1; i++ {
		if command[i] == "--uuid" {
			return strings.TrimSpace(command[i+1])
		}
	}
	return ""
}

type repositoryVolumeInventory interface {
	InventoryRepositoryVolumes(context.Context, string, ...string) ([]bhruntime.RepositoryVolume, error)
}

func renderRepositoryVolumePreservation(out io.Writer, volumes []bhruntime.RepositoryVolume, engine string) {
	for _, volume := range volumes {
		fmt.Fprintf(out, "PRESERVED volume %s (Compose project: %s; BaseHarbor owner: %s)\n  %s\n", volume.Name, volume.Project, volume.Owner, volume.Reason)
		if !volume.Shared && volume.Project != "" && (engine == "docker" || engine == "podman") {
			// Quote engine-provided names as shell data, never interpolate as code.
			name := "'" + strings.ReplaceAll(volume.Name, "'", "'\"'\"'") + "'"
			fmt.Fprintf(out, "  Deliberate cleanup after verifying no other consumers and accepting data loss: %s volume rm -- %s\n", engine, name)
		} else {
			fmt.Fprintln(out, "  Inspect ownership and all consumers manually before considering cleanup.")
		}
	}
}
