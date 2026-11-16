package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

// appVersionsCommand is the aggregate readable version view across recorded
// deployments (and, with --all-targets, across every configured Target). It
// renders the same unified version section `app status` and `app version`
// render (card 0428-062): the three strictly separated reference dimensions
// from the delivered record-derived cells, plus one component row per declared
// identity with its portable source-model provenance. Read-only: no registry
// query, no mutation.
func appVersionsCommand() *cli.Command {
	return &cli.Command{
		Name:    "versions",
		Summary: "Show readable wanted/applied/observed versions for recorded deployments",
		Usage:   "baha app versions [--all-targets] [-c COMPONENT[,COMPONENT]] [-o json|--output json]",
		Long:    "Renders each deployment's three independent reference dimensions (wanted, applied, observed) as separate cells sourced from the recorded stage versions, followed by the unified component rows with their source-model provenance. A missing dimension shows UNCLEAR instead of a blank cell; an unmapped component says unmapped. --all-targets widens the listing to every configured Target through the existing display seam. An unknown -c component fails closed.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			selected, filtered, err := parseComponentSelectorArgs(args, "versions")
			if err != nil {
				return err
			}
			filtered, format, err := parseReadOutputArgs(filtered, "app versions")
			if err != nil {
				return err
			}
			allTargets := false
			for _, arg := range filtered {
				switch arg {
				case "--all-targets":
					allTargets = true
				default:
					return unknownOptionUsage("baha app versions", arg, "--all-targets")
				}
			}
			result, err := collectMachineApplicationVersions(ctx, machineApplicationVersionsInput{AllTargets: allTargets, Components: selected})
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			if len(result.Releases) == 0 {
				fmt.Fprintln(out, "No deployments registered.")
				return nil
			}
			for i, view := range result.Releases {
				if i > 0 {
					fmt.Fprintln(out)
				}
				fmt.Fprintf(out, "%s/%s (%s)\n", view.Identity.Application, view.Identity.Environment, view.Identity.Target)
				fmt.Fprintf(out, "  wanted=%s  applied=%s  observed=%s\n",
					stageCellSummary(view.Stages.Wanted),
					stageCellSummary(view.Stages.Applied),
					stageCellSummary(view.Stages.Observed),
				)
				for _, warning := range view.Warnings {
					fmt.Fprintf(out, "  [WARN] %s\n", warning)
				}
				for _, component := range view.Components {
					fmt.Fprintln(out, "  "+formatComponentRow(component))
				}
			}
			for _, warning := range result.Warnings {
				fmt.Fprintf(errOut, "[WARN] %s\n", warning)
			}
			return nil
		},
	}
}

type machineApplicationVersionsInput struct {
	AllTargets bool     `json:"all_targets,omitempty"`
	Target     string   `json:"target,omitempty" jsonschema:"optional BaseHarbor target; defaults to the effective target"`
	Components []string `json:"components,omitempty" jsonschema:"optional component or source identities; an unknown identity fails closed"`
}

func collectMachineApplicationVersions(ctx context.Context, input machineApplicationVersionsInput) (machinereadmodels.ApplicationVersionsResult, error) {
	var (
		items    []deployment.DeploymentRecord
		warnings []error
		err      error
	)
	if input.AllTargets {
		items, warnings, err = deployment.ListAllDeploymentsForDisplay()
	} else {
		targetName := strings.TrimSpace(input.Target)
		if targetName == "" {
			target, resolveErr := effectiveTarget(ctx)
			if resolveErr != nil {
				return machinereadmodels.ApplicationVersionsResult{}, resolveErr
			}
			targetName = target.Name
		}
		items, warnings, err = deployment.ListDeploymentsForDisplay(targetName)
	}
	if err != nil {
		return machinereadmodels.ApplicationVersionsResult{}, err
	}
	result := machinereadmodels.ApplicationVersionsResult{
		ContractVersion: machinereadmodels.ReleaseViewContractVersion,
		Releases:        machinereadmodels.ReleaseViewsFromRecords(items),
	}
	for i := range result.Releases {
		record := recordForView(items, result.Releases[i].Identity)
		components, componentWarnings := componentRowsForRecord(record, result.Releases[i].Stages)
		result.Releases[i].Components = components
		// The "no source model yet" state is the ordinary status of a
		// deployment that never initialized a workspace; it is readable in the
		// rows themselves (unmapped), not a listing warning.
		componentWarnings = withoutAbsentSourceModelNotice(componentWarnings)
		result.Releases[i].Warnings = append(result.Releases[i].Warnings, componentWarnings...)
		if err := applyVersionsComponentSelector(&result.Releases[i], input.Components); err != nil {
			return machinereadmodels.ApplicationVersionsResult{}, err
		}
	}
	for _, warning := range warnings {
		result.Warnings = append(result.Warnings, warning.Error())
	}
	return result, nil
}

// recordForView locates the original record for one sorted view so the
// component rows read the same intent and source model the record was derived
// from. Matching is by the view identity only.
func recordForView(records []deployment.DeploymentRecord, identity machinereadmodels.ViewIdentity) deployment.DeploymentRecord {
	for _, record := range records {
		if record.Identity.Target == identity.Target && record.Identity.Application == identity.Application && record.Identity.Environment == identity.Environment {
			return record
		}
	}
	return deployment.DeploymentRecord{}
}

// componentRowsForRecord renders the unified component rows from the record's
// applied intent snapshot and its portable source model (Core-owned
// deployment state first, repository manifest as the delivered fallback). It
// shares the row builder with the status view so CLI, JSON and MCP emit one
// view, never two parallel realities.
func componentRowsForRecord(record deployment.DeploymentRecord, stages machinereadmodels.ViewStages) ([]machinereadmodels.ComponentVersion, []string) {
	manifest, manifestPath := recordIntentManifest(record)
	desired := ""
	if stages.Wanted != nil && !stages.Wanted.MissingData {
		desired = stages.Wanted.Reference
	}
	components := make([]machinereadmodels.ComponentVersion, 0, len(declaredVersionServices(manifest)))
	byName := map[string]struct{}{}
	for _, service := range declaredVersionServices(manifest) {
		row := componentRowFromWanted(service, desired)
		components = append(components, row)
		byName[service.name] = struct{}{}
	}
	for _, component := range application.WorkloadComponentNames(manifest) {
		if _, exists := byName[component]; exists {
			continue
		}
		byName[component] = struct{}{}
		components = append(components, machinereadmodels.ComponentVersion{
			Service:        component,
			SourceRevision: desired,
			Observation:    "logical workload component; image identity comes from the repository workload source",
		})
	}
	if strings.TrimSpace(manifestPath) == "" {
		return sortComponentRows(components), nil
	}
	projections, err := projectRecordSourceComponents(record, manifestPath)
	if err != nil {
		if errorsIsModelMissing(err) {
			return sortComponentRows(components), []string{"no portable source model exists yet; component source provenance is unmapped"}
		}
		return sortComponentRows(components), []string{"component source provenance is unmapped: " + err.Error()}
	}
	for _, projected := range projections {
		if idx := componentRowIndex(components, projected.Component); idx >= 0 {
			components[idx].Source = projected.Source
			components[idx].SourceType = projected.SourceType
			components[idx].SourceIdentity = projected.SourceIdentity
			components[idx].SubPath = projected.SubPath
			continue
		}
		components = append(components, machinereadmodels.ComponentVersion{
			Service:        projected.Component,
			Source:         projected.Source,
			SourceType:     projected.SourceType,
			SourceIdentity: projected.SourceIdentity,
			SubPath:        projected.SubPath,
			SourceRevision: desired,
			Observation:    "declared by the source model; no backend component identity matches it",
		})
	}
	return sortComponentRows(components), nil
}

// recordIntentManifest decodes the applied intent snapshot; a record without a
// readable snapshot has no declared components and reports no invented rows.
func recordIntentManifest(record deployment.DeploymentRecord) (application.Manifest, string) {
	if len(record.Applied.Intent) == 0 {
		return application.Manifest{}, ""
	}
	var manifest application.Manifest
	if err := json.Unmarshal(record.Applied.Intent, &manifest); err != nil {
		return application.Manifest{}, ""
	}
	manifestPath := ""
	if strings.TrimSpace(record.Source.Repository) != "" {
		manifestPath = filepath.Join(record.Source.Repository, application.RepositoryManifestName)
	}
	return manifest, manifestPath
}

// projectRecordSourceComponents resolves the source model beside the
// deployment's canonical manifest. Provenance must survive a developer-local
// detail: a source model resolves its component/source identities even when no
// workstation has mapped the repository checkouts yet, and an unresolvable
// component keeps its explicit unmapped state instead of failing the whole
// aggregate listing (the same fail-soft rule the status view applies).
func projectRecordSourceComponents(record deployment.DeploymentRecord, manifestPath string) ([]componentSourceProjection, error) {
	model, _, err := development.LoadSourceModel(manifestPath)
	if err != nil {
		return nil, err
	}
	if record.Identity.Application != "" && model.Application != record.Identity.Application {
		return nil, fmt.Errorf("source model application %q does not match application %q", model.Application, record.Identity.Application)
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifestPath, model.Application)
	if os.IsNotExist(err) {
		mapping = development.WorkspaceMapping{SchemaVersion: development.WorkspaceMappingVersion, Application: model.Application, Manifest: manifestPath, Sources: map[string]string{}}
	} else if err != nil {
		return nil, err
	}
	resolution, resolveErr := development.ResolveWorkspace(manifestPath, model, mapping)
	if resolveErr != nil {
		fallback, partial := partialWorkspaceResolution(model)
		if len(partial) == 0 {
			return nil, resolveErr
		}
		resolution = fallback
	}
	projections := make([]componentSourceProjection, 0, len(resolution.Components))
	for _, component := range resolution.Components {
		identity := componentResolvedSourceIdentity(component)
		if strings.TrimSpace(identity) == "" {
			identity = component.Identity
		}
		projections = append(projections, componentSourceProjection{
			Component:      component.Component,
			Source:         component.Source,
			SourceType:     string(component.Type),
			SourceIdentity: identity,
			SubPath:        component.SubPath,
		})
	}
	return projections, nil
}

// partialWorkspaceResolution preserves the declared component/source
// identities of a model whose developer-local checkout cannot be resolved on
// this workstation. Every entry is marked unresolved so callers can show the
// explicit state instead of inventing a root.
func partialWorkspaceResolution(model development.SourceModel) (development.WorkspaceResolution, []string) {
	resolution := development.WorkspaceResolution{SchemaVersion: development.WorkspaceResolutionVersion, Application: model.Application}
	var unresolved []string
	for _, component := range model.Components {
		sourceID := strings.TrimSpace(component.Source)
		var definition development.SourceDefinition
		for _, source := range model.Sources {
			if source.ID == sourceID {
				definition = source
			}
		}
		identity := sourceDefinitionIdentity(definition)
		resolution.Components = append(resolution.Components, development.ResolvedComponentSource{
			Component: component.Component,
			Source:    sourceID,
			Type:      definition.Type,
			Identity:  identity,
			Ref:       definition.Ref,
			Image:     definition.Image,
			SubPath:   filepath.ToSlash(strings.TrimSpace(component.SubPath)),
		})
		if definition.Type != development.SourceOCI {
			unresolved = append(unresolved, component.Component)
		}
	}
	return resolution, unresolved
}

func componentResolvedSourceIdentity(component development.ResolvedComponentSource) string {
	if component.Type == development.SourceOCI {
		return component.Image
	}
	return component.Root
}

func applyVersionsComponentSelector(view *machinereadmodels.VersionView, selected []string) error {
	if len(selected) == 0 {
		return nil
	}
	if err := requireKnownComponentSelections(selected, view.Components, "versions"); err != nil {
		return err
	}
	view.Components = filterComponentRows(view.Components, selected)
	return nil
}

func withoutAbsentSourceModelNotice(warnings []string) []string {
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if strings.Contains(warning, "no portable source model exists yet") {
			continue
		}
		result = append(result, warning)
	}
	return result
}

func stageCellSummary(cell *machinereadmodels.ViewStageCell) string {
	return versionCellText(cell)
}
