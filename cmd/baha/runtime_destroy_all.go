package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/connectivityrelay"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

type fullDestroyResult struct {
	Status   string `json:"status"`
	Target   string `json:"target,omitempty"`
	Resource string `json:"resource"`
	Detail   string `json:"detail,omitempty"`
}

func runtimeDestroyCommand(parent context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "destroy")
	if err != nil {
		return err
	}
	args = filtered
	if format == outputJSON {
		confirmed, all := false, false
		for _, arg := range args {
			switch arg {
			case "--yes":
				confirmed = true
			case "--all":
				all = true
			default:
				return usageError("unknown destruction argument", "Use --yes or --all.")
			}
		}
		parent, report := withDestroyReport(parent, all)
		var err error
		if all {
			err = destroyInstallation(machineNoninteractiveContext(parent), confirmed, io.Discard, io.Discard)
		} else {
			err = destroyControlPlane(parent, confirmed, io.Discard)
		}
		if err != nil {
			return err
		}
		return writeJSON(out, report)
	}

	full := false
	for _, arg := range args {
		if arg == "--all" {
			full = true
			break
		}
	}
	if !full {
		return runtimeDestroy(parent, args, out)
	}
	return runtimeDestroyAll(parent, args, out, errOut)
}

func runtimeDestroyAll(parent context.Context, args []string, out, errOut io.Writer) error {
	confirmed := false
	for _, arg := range args {
		switch arg {
		case "--all":
		case "--yes":
			confirmed = true
		default:
			return usageError("unknown argument "+arg, "Usage: baha destroy --all [--yes]")
		}
	}

	return destroyInstallation(parent, confirmed, out, errOut)
}
func destroyInstallation(parent context.Context, confirmed bool, out, errOut io.Writer) error {
	if err := authorizeCurrentMCPContext(parent, "installation.destroy", "", "", ""); err != nil {
		return err
	}

	targets, discoveryResults := discoverFullDestroyTargets()
	deployments, deploymentResults := discoverFullDestroyDeployments()
	// Authorize every discovered stable deployment before any target/network or
	// best-effort cleanup. One unauthorized boundary blocks the whole operation.
	for _, record := range deployments {
		if err := authorizeMCPOperation(parent, "installation.destroy", record.Identity.Target, record.Identity.Environment, record.Identity.ApplicationID, record.Source.Manifest); err != nil {
			return err
		}
	}

	plans, err := collectFullDestroyInventory(parent, targets)
	if err != nil {
		return err
	}
	preserved := []fullDestroyResult{}
	for _, target := range targets {
		item, err := preservedTargetRecovery(parent, target)
		if err != nil {
			return fmt.Errorf("inventory external recovery material: %w", err)
		}
		if item.Status != "" {
			preserved = append(preserved, item)
		}
	}

	recordDestroyPlan(parent, plans, preserved)
	fmt.Fprintln(out, "WARNING")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "This will permanently remove all BaseHarbor-managed deployments,")
	fmt.Fprintln(out, "runtime resources, providers, secrets, control-plane data and local")
	fmt.Fprintln(out, "BaseHarbor configuration/state from this installation.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Application source repositories and external application-owned data are preserved.")
	fmt.Fprintf(out, "Targets: %d\n", len(targets))
	fmt.Fprintf(out, "Registered deployments: %d\n", len(deployments))
	for _, plan := range plans {
		plan.render(out)
	}
	for _, item := range preserved {
		fmt.Fprintf(out, "  PRESERVED %s %s\n", item.Resource, item.Detail)
	}
	if !confirmed {
		if noInput(parent) || !readerIsTerminal(runtimeInput) {
			fmt.Fprintln(out, "No changes were made. Re-run with --yes to perform the full cleanup.")
			return nil
		}
		ok, err := promptGuidedConfirmation(runtimeInput, out, "Continue?", false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Full destroy cancelled; no state was changed.")
			return nil
		}
	}

	results := append([]fullDestroyResult{}, preserved...)
	results = append(results, discoveryResults...)
	results = append(results, deploymentResults...)
	for _, target := range targets {
		releaseFullDestroyConnectivity(parent, target, &results)
	}
	for _, record := range deployments {
		targetCtx := withTargetOverride(parent, record.Identity.Target)
		args := []string{record.Identity.Application, "--environment", record.Identity.Environment, "--yes", "--full-reset"}
		if err := executeApplicationDestroyLifecycle(targetCtx, application.Store{}, args, out, errOut); err != nil {
			results = append(results, fullDestroyResult{
				Status:   "FAILED",
				Target:   record.Identity.Target,
				Resource: "deployment " + record.Identity.Application + "/" + record.Identity.Environment,
				Detail:   err.Error(),
			})
			bestEffortApplicationCleanup(targetCtx, record, &results)
			continue
		}
		results = append(results, fullDestroyResult{
			Status:   "REMOVED",
			Target:   record.Identity.Target,
			Resource: "deployment " + record.Identity.Application + "/" + record.Identity.Environment,
		})
	}

	for i, target := range targets {
		destroyTargetBestEffort(parent, target, &results, plans[i])
	}

	blockers := countFullDestroyBlockers(results)
	if blockers == 0 {
		removeFullDestroyLocalState(&results)
	} else {
		results = append(results,
			fullDestroyResult{Status: "SKIPPED", Resource: "xdg-data", Detail: "preserved because runtime cleanup is incomplete; retry destroy after resolving the reported failures"},
			fullDestroyResult{Status: "SKIPPED", Resource: "xdg-config", Detail: "preserved because runtime cleanup is incomplete; ownership evidence remains available for retry"},
		)
	}
	recordDestroyResults(parent, results, countFullDestroyBlockers(results) == 0)
	renderFullDestroyReport(out, results)

	blockers = countFullDestroyBlockers(results)
	if blockers > 0 {
		return fmt.Errorf("full destroy did not complete; %d cleanup result(s) require attention and BaseHarbor state was preserved", blockers)
	}
	fmt.Fprintln(out, "BaseHarbor-managed installation state was permanently removed.")
	return nil
}

func discoverFullDestroyTargets() ([]deployment.ResolvedTarget, []fullDestroyResult) {
	var results []fullDestroyResult
	cfg, cfgErr := deployment.LoadConfig()
	if cfgErr != nil {
		results = append(results, fullDestroyResult{Status: "FAILED", Resource: "config", Detail: cfgErr.Error()})
		cfg = deployment.Config{Version: deployment.ConfigVersion, Access: map[string]deployment.AccessDefinition{}, Targets: map[string]deployment.TargetDefinition{}}
	}

	byName := map[string]deployment.ResolvedTarget{}
	for _, name := range cfg.TargetNames() {
		target, err := cfg.ResolveTarget(name, "")
		if err != nil {
			results = append(results, fullDestroyResult{Status: "FAILED", Target: name, Resource: "target-config", Detail: err.Error()})
			continue
		}
		byName[name] = target
	}

	root, err := deployment.DataRoot()
	if err != nil {
		results = append(results, fullDestroyResult{Status: "FAILED", Resource: "data-root", Detail: err.Error()})
	} else {
		entries, readErr := os.ReadDir(filepath.Join(root, "targets"))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			results = append(results, fullDestroyResult{Status: "FAILED", Resource: "target-state", Detail: readErr.Error()})
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if _, ok := byName[name]; ok {
				continue
			}
			if err := deployment.ValidateTargetName(name); err != nil {
				results = append(results, fullDestroyResult{Status: "SKIPPED", Target: name, Resource: "target-state", Detail: "invalid target directory name; local BaseHarbor data root will still be removed"})
				continue
			}
			provider := ""
			if records, listErr := deployment.ListDeployments(name); listErr == nil {
				for _, record := range records {
					candidate := strings.TrimSpace(record.Applied.RuntimeProvider)
					if candidate == "" {
						continue
					}
					if provider == "" {
						provider = candidate
					} else if provider != candidate {
						provider = ""
						break
					}
				}
			}
			if provider == "" && name == "local" {
				provider = "compose"
			}
			byName[name] = deployment.ResolvedTarget{Name: name, RuntimeProvider: provider}
			if provider == "" {
				results = append(results, fullDestroyResult{Status: "SKIPPED", Target: name, Resource: "runtime-provider", Detail: "target config is missing and runtime provider cannot be inferred; only BaseHarbor-owned local state can be removed safely"})
			}
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	targets := make([]deployment.ResolvedTarget, 0, len(names))
	for _, name := range names {
		targets = append(targets, byName[name])
	}
	return targets, results
}

func releaseFullDestroyConnectivity(parent context.Context, target deployment.ResolvedTarget, results *[]fullDestroyResult) {
	dataDir, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity", Detail: err.Error()})
		return
	}
	rules, err := application.LoadConnectivityRulesAt(dataDir)
	if errors.Is(err, os.ErrNotExist) || len(rules) == 0 {
		return
	}
	if err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity-policy", Detail: err.Error()})
		return
	}
	if strings.TrimSpace(target.RuntimeProvider) == "" {
		*results = append(*results, fullDestroyResult{Status: "SKIPPED", Target: target.Name, Resource: "connectivity-runtime", Detail: "runtime provider is unknown; refusing to guess external resources"})
		return
	}
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	compose, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity-runtime", Detail: err.Error()})
		return
	}
	containers, err := compose.ListRuntimeContainers(ctx)
	if err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity-runtime", Detail: err.Error()})
		return
	}
	for _, rule := range rules {
		resource := "connectivity " + application.ConnectivityRuleID(rule)
		if err := suspendConnectivityRuleAt(ctx, compose, dataDir, target.Name, rule, containers); err != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: resource, Detail: err.Error()})
			continue
		}
		if err := application.RemoveConnectivityRuleAt(dataDir, rule); err != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: resource + " policy", Detail: err.Error()})
			continue
		}
		if err := connectivityrelay.RemoveFilesAt(dataDir, application.ConnectivityRuleID(rule)); err != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: resource + " state", Detail: err.Error()})
			continue
		}
		*results = append(*results, fullDestroyResult{Status: "REMOVED", Target: target.Name, Resource: resource})
	}
}

func discoverFullDestroyDeployments() ([]deployment.DeploymentRecord, []fullDestroyResult) {
	records, warnings, err := deployment.ListAllDeploymentsForDisplay()
	if err != nil {
		return nil, []fullDestroyResult{{Status: "FAILED", Resource: "deployment-registry", Detail: err.Error()}}
	}

	var results []fullDestroyResult
	for _, warning := range warnings {
		stateErr, ok := deployment.DeploymentRecordState(warning)
		if !ok {
			results = append(results, fullDestroyResult{
				Status:   "FAILED",
				Resource: "deployment-registry",
				Detail:   warning.Error(),
			})
			continue
		}
		resource := "deployment " + stateErr.Identity.DeploymentID
		if stateErr.Identity.DeploymentID == "" {
			resource = "deployment unknown"
		}
		results = append(results, fullDestroyResult{
			Status:   "SKIPPED",
			Target:   stateErr.Identity.Target,
			Resource: resource,
			Detail:   "incomplete deployment state has no usable deployment.json; refusing to guess semantic/runtime ownership, local BaseHarbor state will still be removed",
		})
	}
	return records, results
}

func bestEffortApplicationCleanup(parent context.Context, record deployment.DeploymentRecord, results *[]fullDestroyResult) {
	target := deployment.ResolvedTarget{Name: record.Identity.Target, RuntimeProvider: record.Applied.RuntimeProvider}
	cfg, err := deployment.LoadConfig()
	if err == nil {
		if configured, resolveErr := cfg.ResolveTarget(record.Identity.Target, ""); resolveErr == nil {
			target = configured
		}
	}
	targetRoot, err := deployment.TargetStateRoot(record.Identity.Target)
	if err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: record.Identity.Target, Resource: "deployment-fallback", Detail: err.Error()})
		return
	}
	resolved, err := resolveRegisteredApplication(target, targetRoot, record.Identity.Application, record.Identity.Environment, "destroy")
	if err == nil && strings.TrimSpace(target.RuntimeProvider) != "" {
		compose, composeErr := detectRuntimeForTarget(parent, target)
		if composeErr != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "deployment-fallback-runtime", Detail: composeErr.Error()})
		} else {
			m := resolved.Manifest
			files, filesErr := application.ExistingRuntimeFiles(resolved.Store, m)
			if filesErr != nil {
				files = application.RuntimeFilesFor(resolved.Store, m)
			}
			runtimeProject := application.RuntimeComposeProjectNameForStore(resolved.Store, m)
			resourceProject := application.RuntimeProjectNameForStore(resolved.Store, m)
			if resolved.FromRepository {
				if _, stopErr := stopRepositoryWorkload(parent, compose, resolved, files); stopErr != nil {
					*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "workload " + m.Name + "/" + m.Environment, Detail: stopErr.Error()})
				} else if removed, cleanupErr := destroyRepositoryWorkloadRuntime(parent, compose, resolved, files); cleanupErr != nil {
					*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "workload-runtime " + m.Name + "/" + m.Environment, Detail: cleanupErr.Error()})
				} else if removed {
					*results = append(*results, fullDestroyResult{Status: "REMOVED", Target: target.Name, Resource: "workload-runtime " + m.Name + "/" + m.Environment, Detail: "owned containers and networks removed; repository volumes preserved"})
				}
			}
			if application.RequiresRuntimeBroker(m) {
				if destroyErr := destroyRuntimeBroker(parent, compose, m, files); destroyErr != nil {
					*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "runtime-broker " + m.Name + "/" + m.Environment, Detail: destroyErr.Error()})
				}
			}
			if destroyErr := compose.DestroyOwnedProjectResources(parent, runtimeProject, application.ExpectedRuntimeResourcesForIdentity(m, runtimeProject, resourceProject)); destroyErr != nil {
				*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "application-runtime " + m.Name + "/" + m.Environment, Detail: destroyErr.Error()})
			}
			if application.HasSharedBackends(m) {
				if releaseErr := application.ReleaseSharedBackendApplication(parent, compose, targetRoot, target.Name, m); releaseErr != nil {
					*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "shared-data-resources " + m.Name + "/" + m.Environment, Detail: releaseErr.Error()})
				}
			}
			if placement, found, placementErr := application.RegisteredProviderPlacementAt(targetRoot, m, capability.ProviderTempo); placementErr == nil && found && placement.Scope == capability.ScopeApplication {
				if destroyErr := tracesprovider.DestroyProvider(parent, compose, m); destroyErr != nil {
					*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "application-traces " + m.Name + "/" + m.Environment, Detail: destroyErr.Error()})
				}
			}
			if placement, found, placementErr := application.RegisteredProviderPlacementAt(targetRoot, m, capability.ProviderPrometheus); placementErr == nil && found && placement.Scope == capability.ScopeApplication {
				if destroyErr := metricsprovider.DestroyProvider(parent, compose, m); destroyErr != nil {
					*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "application-metrics " + m.Name + "/" + m.Environment, Detail: destroyErr.Error()})
				}
			}
			if releaseErr := application.ReleaseApplicationProviderRegistryAt(targetRoot, m); releaseErr != nil {
				*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "provider-bindings " + m.Name + "/" + m.Environment, Detail: releaseErr.Error()})
			}
		}
	}
	if err := deployment.DeleteDeploymentRecord(record.Identity); err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: record.Identity.Target, Resource: "deployment-record " + record.Identity.Application + "/" + record.Identity.Environment, Detail: err.Error()})
	} else {
		*results = append(*results, fullDestroyResult{Status: "REMOVED", Target: record.Identity.Target, Resource: "deployment-record " + record.Identity.Application + "/" + record.Identity.Environment, Detail: "removed during recovery cleanup"})
	}
}

func destroyTargetBestEffort(parent context.Context, target deployment.ResolvedTarget, results *[]fullDestroyResult, plan targetDestroyInventory) {
	dataDir, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "target-state", Detail: err.Error()})
		return
	}
	if strings.TrimSpace(target.RuntimeProvider) == "" {
		if err := os.RemoveAll(dataDir); err != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "target-state", Detail: err.Error()})
		} else {
			*results = append(*results, fullDestroyResult{Status: "REMOVED", Target: target.Name, Resource: "target-state", Detail: "runtime provider unknown; no external runtime resources were guessed"})
		}
		return
	}

	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	cleanupResultStart := len(*results)
	compose, composeErr := detectRuntimeForTarget(ctx, target)
	if composeErr != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "runtime-provider", Detail: composeErr.Error()})
		*results = append(*results, fullDestroyResult{Status: "SKIPPED", Target: target.Name, Resource: "target-state", Detail: "preserved because runtime cleanup could not be verified"})
		return
	}

	rules, rulesErr := application.LoadConnectivityRulesAt(dataDir)
	if rulesErr != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity-policy", Detail: rulesErr.Error()})
	} else if len(rules) > 0 {
		containers, listErr := compose.ListRuntimeContainers(ctx)
		if listErr != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity-runtime", Detail: listErr.Error()})
		} else {
			for _, rule := range rules {
				if err := suspendConnectivityRuleAt(ctx, compose, dataDir, target.Name, rule, containers); err != nil {
					*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity " + application.ConnectivityRuleID(rule), Detail: err.Error()})
				}
				_ = application.RemoveConnectivityRuleAt(dataDir, rule)
				_ = connectivityrelay.RemoveFilesAt(dataDir, application.ConnectivityRuleID(rule))
			}
		}
	}

	if removed, err := hosttrust.RemoveOwned(ctx, dataDir); err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "host-trust", Detail: err.Error()})
	} else if removed > 0 {
		*results = append(*results, fullDestroyResult{Status: "REMOVED", Target: target.Name, Resource: "host-trust", Detail: fmt.Sprintf("%d CA anchor(s)", removed)})
	}

	if relays, err := connectivityrelay.ExistingInstancesAt(dataDir, target.Name); err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity-relays", Detail: err.Error()})
	} else {
		for _, relay := range relays {
			if err := compose.DestroyProject(ctx, relay.Project, relay.Compose, relay.Env); err != nil {
				*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "connectivity-relay " + relay.Project, Detail: err.Error()})
			}
		}
	}

	runCleanup := func(resource string, fn func() error) {
		if err := fn(); err != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: resource, Detail: err.Error()})
		} else {
			*results = append(*results, fullDestroyResult{Status: "REMOVED", Target: target.Name, Resource: resource})
		}
	}
	runCleanup("development-gateway", func() error { return devgateway.DestroyTarget(ctx, compose, target.Name) })
	runCleanup("identity", func() error { return identityprovider.DestroyAllSharedKeycloakAt(ctx, compose, dataDir, target.Name) })
	runCleanup("runtime-executor", func() error { return runtimeexecutor.DestroySharedAt(ctx, compose, dataDir, target.Name) })
	runCleanup("data-providers", func() error { return application.DestroyAllSharedBackendsAt(ctx, compose, dataDir, target.Name) })
	runCleanup("object-storage", func() error { return objectstorage.DestroySharedProviderAt(ctx, compose, dataDir, target.Name) })
	runCleanup("telemetry", func() error { return telemetry.DestroySharedProviderAt(ctx, compose, dataDir, target.Name) })
	runCleanup("traces", func() error { return tracesprovider.DestroyAllSharedProvidersAt(ctx, compose, dataDir, target.Name) })
	runCleanup("metrics", func() error { return metricsprovider.DestroyAllSharedProvidersAt(ctx, compose, dataDir, target.Name) })

	runtimeRoot := filepath.Join(dataDir, "runtime")
	files, filesErr := bhruntime.ExistingFilesForProject(runtimeRoot, targetRuntimeProjectName(target))
	if filesErr == nil {
		runCleanup("control-plane", func() error { return compose.DestroyProject(ctx, files.Project, files.Compose, files.Env) })
	} else if errors.Is(filesErr, os.ErrNotExist) {
		*results = append(*results, fullDestroyResult{Status: "NOT FOUND", Target: target.Name, Resource: "control-plane"})
	} else {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "control-plane-state", Detail: filesErr.Error()})
	}

	plan.cleanup(ctx, results)
	cleanupOrphanedTargetRuntimeProjects(ctx, compose, target.Name, results)

	if containers, err := compose.ListRuntimeContainers(ctx); err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "runtime-audit", Detail: err.Error()})
	} else if residual := targetOwnedRuntimeContainers(target.Name, containers); len(residual) > 0 {
		var names []string
		for _, container := range residual {
			names = append(names, container.Project+"/"+container.Service+" ("+container.Name+")")
		}
		sort.Strings(names)
		*results = append(*results, fullDestroyResult{
			Status:   "FAILED",
			Target:   target.Name,
			Resource: "runtime-audit",
			Detail:   "BaseHarbor-owned containers remain: " + strings.Join(names, ", "),
		})
	}

	if countFullDestroyBlockers((*results)[cleanupResultStart:]) > 0 {
		*results = append(*results, fullDestroyResult{
			Status:   "SKIPPED",
			Target:   target.Name,
			Resource: "target-state",
			Detail:   "preserved because one or more target runtime resources could not be removed or verified",
		})
		return
	}
	if err := os.RemoveAll(dataDir); err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Target: target.Name, Resource: "target-state", Detail: err.Error()})
	} else {
		*results = append(*results, fullDestroyResult{Status: "REMOVED", Target: target.Name, Resource: "target-state"})
	}
}

func cleanupOrphanedTargetRuntimeProjects(ctx context.Context, runtime bhruntime.RuntimeProvider, target string, results *[]fullDestroyResult) {
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		*results = append(*results, fullDestroyResult{
			Status:   "FAILED",
			Target:   target,
			Resource: "orphan-runtime-discovery",
			Detail:   err.Error(),
		})
		return
	}

	projects := map[string]struct{}{}
	for _, container := range targetOwnedRuntimeContainers(target, containers) {
		project := strings.TrimSpace(container.Project)
		if project != "" {
			projects[project] = struct{}{}
		}
	}
	names := make([]string, 0, len(projects))
	for project := range projects {
		names = append(names, project)
	}
	sort.Strings(names)

	for _, project := range names {
		resources, err := runtime.ListOwnedProjectResources(ctx, project)
		if err != nil {
			*results = append(*results, fullDestroyResult{
				Status:   "FAILED",
				Target:   target,
				Resource: "orphan-runtime " + project,
				Detail:   "inventory owned resources: " + err.Error(),
			})
			continue
		}
		if len(resources) == 0 {
			continue
		}
		if err := runtime.DestroyOwnedProjectResources(ctx, project, resources); err != nil {
			*results = append(*results, fullDestroyResult{
				Status:   "FAILED",
				Target:   target,
				Resource: "orphan-runtime " + project,
				Detail:   err.Error(),
			})
			continue
		}
		remaining, err := runtime.ListOwnedProjectResources(ctx, project)
		if err != nil {
			*results = append(*results, fullDestroyResult{
				Status:   "FAILED",
				Target:   target,
				Resource: "orphan-runtime " + project,
				Detail:   "verify owned resources: " + err.Error(),
			})
			continue
		}
		if len(remaining) > 0 {
			var residual []string
			for _, resource := range remaining {
				residual = append(residual, resource.Kind+" "+resource.Name)
			}
			sort.Strings(residual)
			*results = append(*results, fullDestroyResult{
				Status:   "FAILED",
				Target:   target,
				Resource: "orphan-runtime " + project,
				Detail:   "owned resources remain: " + strings.Join(residual, ", "),
			})
			continue
		}
		*results = append(*results, fullDestroyResult{
			Status:   "REMOVED",
			Target:   target,
			Resource: "orphan-runtime " + project,
			Detail:   "removed using provider ownership labels",
		})
	}
}

func removeFullDestroyLocalState(results *[]fullDestroyResult) {
	dataRoot, dataErr := deployment.DataRoot()
	if dataErr != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Resource: "xdg-data", Detail: dataErr.Error()})
	} else if err := os.RemoveAll(dataRoot); err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Resource: "xdg-data", Detail: err.Error()})
	} else {
		*results = append(*results, fullDestroyResult{Status: "REMOVED", Resource: "xdg-data", Detail: dataRoot})
	}

	configPath, configErr := deployment.ConfigPath()
	if configErr != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Resource: "xdg-config", Detail: configErr.Error()})
		return
	}
	configRoot := filepath.Dir(configPath)
	if err := os.RemoveAll(configRoot); err != nil {
		*results = append(*results, fullDestroyResult{Status: "FAILED", Resource: "xdg-config", Detail: err.Error()})
	} else {
		*results = append(*results, fullDestroyResult{Status: "REMOVED", Resource: "xdg-config", Detail: configRoot})
	}
}

func renderFullDestroyReport(out io.Writer, results []fullDestroyResult) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Cleanup report")
	if len(results) == 0 {
		fmt.Fprintln(out, "  NOT FOUND  BaseHarbor-managed state")
		return
	}
	for _, result := range results {
		target := result.Target
		if target == "" {
			target = "-"
		}
		if result.Detail == "" {
			fmt.Fprintf(out, "  %-9s %-16s %s\n", result.Status, target, result.Resource)
		} else {
			fmt.Fprintf(out, "  %-9s %-16s %s: %s\n", result.Status, target, result.Resource, result.Detail)
		}
	}
}

func countFullDestroyBlockers(results []fullDestroyResult) int {
	count := 0
	for _, result := range results {
		if result.Status == "FAILED" {
			count++
			continue
		}
		if result.Status == "SKIPPED" && strings.Contains(strings.ToLower(result.Detail), "preserved because") {
			count++
		}
	}
	return count
}

func targetOwnedRuntimeContainers(target string, containers []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer {
	sharedProject := bhruntime.SharedProjectName(target)
	projectPrefix := strings.TrimSuffix(sharedProject, "shared")
	var result []bhruntime.RuntimeContainer
	for _, container := range containers {
		project := strings.TrimSpace(container.Project)
		if project == sharedProject || strings.HasPrefix(project, projectPrefix) {
			result = append(result, container)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Project != result[j].Project {
			return result[i].Project < result[j].Project
		}
		if result[i].Service != result[j].Service {
			return result[i].Service < result[j].Service
		}
		return result[i].Name < result[j].Name
	})
	return result
}
