package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// appVersionCommand renders the readable release/component version view for one
// deployment. It is the version face of the unified status/versions view
// (card 0428-062): the wanted, applied and observed reference dimensions come
// from the delivered record-derived cells, the component rows come from the
// delivered portable source model and the declared backend identities, and
// live runtime observation enriches the same rows. It is read-only: no
// registry query, no mutation.
func appVersionCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "version",
		Summary: "Show separated wanted/applied/observed release and component versions",
		Usage:   "baha app version [NAME] [-c COMPONENT[,COMPONENT]] [-o json|--output json]",
		Long:    "Read-only version view of one deployment. Wanted, applied and observed come from the three independent recorded reference dimensions and are never copied between each other. A dimension without a recorded version shows an explicit UNCLEAR cell instead of a blank. Component rows carry their portable source-model provenance (source id, kind, identity) or an explicit unmapped state; observed runtime identity enriches the same rows when reachable. An unknown -c component fails closed. Metadata claims are labeled unverified; no Git revision is inferred from a tag.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			selected, filtered, err := parseComponentSelectorArgs(args, "version")
			if err != nil {
				return err
			}
			filtered, format, err := parseReadOutputArgs(filtered, "app version")
			if err != nil {
				return err
			}
			resolved, err := resolveApplication(ctx, store, filtered, "version")
			if err != nil {
				return err
			}
			result, err := collectApplicationVersionView(ctx, resolved)
			if err != nil {
				return err
			}
			if err := applyComponentSelector(&result, selected, "version"); err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			renderApplicationVersionView(out, result)
			return nil
		},
	}
}

// applicationVersionViewResult is the callable bounded result of the version
// view: the deployment-level separated stage cells plus per-component versions.
type applicationVersionViewResult struct {
	ContractVersion string                               `json:"contract_version"`
	View            machinereadmodels.VersionView        `json:"view"`
	Components      []machinereadmodels.ComponentVersion `json:"components"`
}

func collectApplicationVersionView(ctx context.Context, resolved resolvedApplication) (applicationVersionViewResult, error) {
	// Authorization goes through the same decision seam as every other
	// read-only application operation (app.show): a typed operation ID plus the
	// resolved target/environment/identity, below the presentation layer.
	if err := authorizeApplicationOperation(ctx, "app.versions", resolved); err != nil {
		return applicationVersionViewResult{}, err
	}
	versions, warnings := collectAppStatusVersions(resolved)
	view := machinereadmodels.VersionView{
		ContractVersion: machinereadmodels.ReleaseViewContractVersion,
		Identity:        machinereadmodels.ViewIdentity{Target: resolved.Target.Name, Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment},
		Source:          machinereadmodels.ViewSourceState{Available: resolved.SourceAvailable},
		Stages:          versions.Stages,
		Warnings:        warnings,
	}
	if !view.Source.Available {
		view.Source.Detail = "deployment source is not readable; shown state is recorded state only"
	}
	components, liveWarnings := observeComponentRuntimeIdentities(ctx, resolved, versions.Components)
	view.Warnings = append(view.Warnings, liveWarnings...)
	if result, statusErr := collectApplicationStatusResult(ctx, resolved.Store, applicationStatusSelector(resolved)); statusErr == nil {
		view.State = result.State
		view.Ready = result.Ready
	}
	return applicationVersionViewResult{
		ContractVersion: machine.ContractVersion,
		View:            view,
		Components:      components,
	}, nil
}

// applicationStatusSelector rebuilds the positional selector set that
// collectApplicationStatusResult resolves back to this same deployment.
func applicationStatusSelector(resolved resolvedApplication) []string {
	args := []string{resolved.Manifest.Name}
	if resolved.Manifest.Environment != "" {
		args = append(args, resolved.Manifest.Environment)
	}
	return args
}

// observeComponentRuntimeIdentities reads the live image identity per declared
// backend service through the delivered read-only runtime seam. Every failure
// is an explicit observation state, never a status error.
func observeComponentRuntimeIdentities(ctx context.Context, resolved resolvedApplication, rows []machinereadmodels.ComponentVersion) ([]machinereadmodels.ComponentVersion, []string) {
	var warnings []string
	if len(rows) == 0 || resolved.DeploymentRecord == nil {
		return rows, warnings
	}
	runtimeValue, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return rows, append(warnings, fmt.Sprintf("runtime observation unavailable: %v", err))
	}
	observer, ok := runtimeValue.(interface {
		ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error)
		ProjectServiceImageIdentity(context.Context, string, string) (bhruntime.ImageIdentity, error)
	})
	if !ok {
		return rows, append(warnings, "runtime provider has no read-only image identity seam")
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return rows, append(warnings, "runtime not applied yet; component identities are not observed")
	}
	containers, err := observer.ListRuntimeContainers(ctx)
	if err != nil {
		return rows, append(warnings, fmt.Sprintf("container inventory unavailable: %v", err))
	}
	byService := map[string]bhruntime.RuntimeContainer{}
	for _, container := range containers {
		if container.Project == files.ResourceProject {
			byService[container.Service] = container
		}
	}
	result := make([]machinereadmodels.ComponentVersion, len(rows))
	copy(result, rows)
	for i := range result {
		container, ok := byService[result[i].Service]
		if !ok {
			continue
		}
		identity, err := observer.ProjectServiceImageIdentity(ctx, files.ResourceProject, result[i].Service)
		if err != nil || identity.Reference == "" {
			result[i].Observation = "unclear (container present, image identity unreadable)"
			continue
		}
		repository, tag, digest := machinereadmodels.SplitImageReference(identity.Reference)
		if repository != "" {
			result[i].Repository = repository
		}
		if tag != "" {
			result[i].Tag = tag
		}
		if digest == "" {
			digest = identity.Digest
		}
		result[i].Digest = digest
		result[i].Identity = identity.ImageID
		if !container.Running {
			result[i].Observation = "observed (container present, not running: " + container.State + ")"
		} else {
			result[i].Observation = "observed"
		}
		result[i].ObservedAt = resolved.DeploymentRecord.Observed.VerifiedAt
		result[i].Stale = resolved.DeploymentRecord.Stale() != nil
	}
	return result, warnings
}

// applyComponentSelector filters the unified component rows by -c selections
// and keeps the command's view consistent. An unknown selection fails closed.
func applyComponentSelector(result *applicationVersionViewResult, selected []string, command string) error {
	if len(selected) == 0 {
		return nil
	}
	if err := requireKnownComponentSelections(selected, result.Components, command); err != nil {
		return err
	}
	result.Components = filterComponentRows(result.Components, selected)
	return nil
}

func renderApplicationVersionView(out io.Writer, result applicationVersionViewResult) {
	v := result.View
	fmt.Fprintf(out, "Application: %s/%s (%s)\n", v.Identity.Application, v.Identity.Environment, v.Identity.Target)
	source := "OK"
	if !v.Source.Available {
		source = "MISSING"
	}
	fmt.Fprintf(out, "Source: %s\n", source)
	fmt.Fprintln(out, "\nReference dimensions (kept separate; never copied between columns):")
	fmt.Fprintf(out, "%-12s %-9s %s\n", "WANTED", "APPLIED", "OBSERVED")
	fmt.Fprintf(out, "%s %s %s\n",
		versionCellText(v.Stages.Wanted),
		versionCellText(v.Stages.Applied),
		versionCellText(v.Stages.Observed))
	if len(result.Components) > 0 {
		fmt.Fprintln(out, "\nComponents (source provenance; image repository/tag; observed identity):")
		fmt.Fprintf(out, "%-18s %-38s %-16s %-24s %s\n", "SERVICE", "REPOSITORY", "TAG", "SOURCE", "OBSERVATION")
		for _, component := range result.Components {
			tag := component.Tag
			if tag == "" {
				tag = "UNCLEAR"
			}
			repository := component.Repository
			if repository == "" {
				repository = component.Image
			}
			origin := component.Source
			if origin == "" {
				origin = "unmapped"
			}
			if component.SubPath != "" && component.SubPath != "." {
				origin += " @" + component.SubPath
			}
			fmt.Fprintf(out, "%-18s %-38s %-16s %-24s %s\n", component.Service, truncateColumn(repository, 38), truncateColumn(tag, 16), truncateColumn(origin, 24), component.Observation)
		}
	}
	for _, warning := range v.Warnings {
		fmt.Fprintf(out, "[WARN] %s\n", warning)
	}
}
