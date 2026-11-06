package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// appVersionCommand renders the readable release/component version view for one
// deployment. It is the primary version UI: the wanted, applied and observed
// reference dimensions from the accepted stage-version writer
// (internal/deployment/stage_versions.go) appear as three strictly separate
// columns, and each service shows its image repository/tag, observed digest and
// observation freshness. It is read-only: no registry query, no mutation.
func appVersionCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "version",
		Summary: "Show separated wanted/applied/observed release and component versions",
		Usage:   "baha app version [NAME] [-o json|--output json]",
		Long:    "Read-only version view of one deployment. Wanted, applied and observed come from the three independent recorded reference dimensions and are never copied between each other. A dimension without a recorded version shows an explicit UNCLEAR cell instead of a blank. Component rows show each service image repository/tag and the observed runtime identity with its freshness. Metadata claims are labeled unverified; no Git revision is inferred from a tag.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app version")
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
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source:   resolved.deploymentSource(),
	}
	// The persisted record is the authoritative source of the reference
	// dimensions. A repository resolution may not carry an in-memory copy.
	fresh, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err == nil {
		record = fresh
	} else if resolved.DeploymentRecord != nil {
		record = *resolved.DeploymentRecord
	}
	view := machinereadmodels.ReleaseViewFromRecord(record)
	components, warnings := collectVersionComponents(ctx, resolved, view.Stages)
	view.Warnings = append(view.Warnings, warnings...)
	view.Components = nil
	return applicationVersionViewResult{
		ContractVersion: machine.ContractVersion,
		View:            view,
		Components:      components,
	}, nil
}

// deploymentSource reconstructs the record's source shape for a resolution that
// did not load a persisted record (never-delivered path).
func (r resolvedApplication) deploymentSource() deployment.DeploymentSource {
	source := deployment.DeploymentSource{Kind: "repository", Repository: r.repositoryRoot(), Manifest: r.ManifestPath}
	if r.FromRepository && r.SourceAvailable {
		source.Kind = "repository"
	}
	return source
}

// collectVersionComponents renders one row per declared backend service. The
// wanted column reuses the SAME reference dimensions as the stage cells (the
// merged stage versions), never a duplicated store; applied/observed per
// component come from the live runtime identity when reachable.
func collectVersionComponents(ctx context.Context, resolved resolvedApplication, stages machinereadmodels.ViewStages) ([]machinereadmodels.ComponentVersion, []string) {
	var warnings []string
	desired := ""
	if stages.Wanted != nil && !stages.Wanted.MissingData {
		desired = stages.Wanted.Reference
	}
	services := declaredVersionServices(resolved.Manifest)
	components := make([]machinereadmodels.ComponentVersion, 0, len(services))
	for _, service := range services {
		row := machinereadmodels.ComponentVersion{
			Service:        service.name,
			Image:          service.image,
			SourceRevision: desired,
			Observation:    "unobserved",
		}
		row.Repository, row.Tag, _ = machinereadmodels.SplitImageReference(service.image)
		if row.Tag == "" {
			row.Tag = machinereadmodels.UnclearValue
		}
		// Annotation-derived metadata is a claim, never verified provenance.
		row.AnnotationNote = "declared image metadata claim; provenance unverified unless separately attested"
		components = append(components, row)
	}
	if len(components) == 0 || resolved.DeploymentRecord == nil {
		return components, warnings
	}
	runtimeValue, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return components, append(warnings, fmt.Sprintf("runtime observation unavailable: %v", err))
	}
	// Read-only observation seam: the concrete Compose provider exposes the
	// container inventory and image identity inspection; nothing here mutates.
	observer, ok := runtimeValue.(interface {
		ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error)
		ProjectServiceImageIdentity(context.Context, string, string) (bhruntime.ImageIdentity, error)
	})
	if !ok {
		return components, append(warnings, "runtime provider has no read-only image identity seam")
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return components, append(warnings, "runtime not applied yet; component identities are not observed")
	}
	containers, err := observer.ListRuntimeContainers(ctx)
	if err != nil {
		return components, append(warnings, fmt.Sprintf("container inventory unavailable: %v", err))
	}
	byService := map[string]bhruntime.RuntimeContainer{}
	for _, container := range containers {
		if container.Project == files.ResourceProject {
			byService[container.Service] = container
		}
	}
	for i := range components {
		container, ok := byService[components[i].Service]
		if !ok {
			continue
		}
		identity, err := observer.ProjectServiceImageIdentity(ctx, files.ResourceProject, components[i].Service)
		if err != nil || identity.Reference == "" {
			components[i].Observation = "unclear (container present, image identity unreadable)"
			continue
		}
		repository, tag, digest := machinereadmodels.SplitImageReference(identity.Reference)
		if repository != "" {
			components[i].Repository = repository
		}
		if tag != "" {
			components[i].Tag = tag
		}
		if digest == "" {
			digest = identity.Digest
		}
		components[i].Digest = digest
		components[i].Identity = identity.ImageID
		if !container.Running {
			components[i].Observation = "observed (container present, not running: " + container.State + ")"
		} else {
			components[i].Observation = "observed"
		}
		components[i].ObservedAt = resolved.DeploymentRecord.Observed.VerifiedAt
		components[i].Stale = resolved.DeploymentRecord.Stale() != nil
	}
	return components, warnings
}

type declaredVersionService struct {
	name  string
	image string
}

// declaredVersionServices enumerates the components the runtime owner would
// realize for this intent, using the same service-name and image sources as the
// native Compose renderer. The application's own workload image is deliberately
// not listed: BaseHarbor does not own it and must not invent its version.
func declaredVersionServices(m application.Manifest) []declaredVersionService {
	var services []declaredVersionService
	add := func(name, image string) {
		services = append(services, declaredVersionService{name: name, image: image})
	}
	for _, instance := range application.SQLInstanceNames(m) {
		if application.UsesSharedPostgreSQL(m) {
			continue
		}
		add(runtimeServiceNameForView("postgres", instance), "docker.io/library/postgres:18-alpine")
	}
	for _, instance := range application.ValkeyInstanceNames(m) {
		if application.UsesSharedValkey(m) {
			continue
		}
		add(runtimeServiceNameForView("valkey", instance), "docker.io/valkey/valkey:9.1.2-alpine")
	}
	for _, instance := range application.RabbitMQInstanceNames(m) {
		add(runtimeServiceNameForView("rabbitmq", instance), "docker.io/library/rabbitmq:4.3.6-alpine")
	}
	for _, instance := range application.DocumentDatabaseInstanceNames(m) {
		add(runtimeServiceNameForView("mongodb", instance), application.MongoDBImage)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].name < services[j].name })
	return services
}

// runtimeServiceNameForView mirrors the runtime owner's service naming without
// exporting a new product API from the application package.
func runtimeServiceNameForView(kind, instance string) string {
	if instance == "" || instance == "default" {
		return kind
	}
	return kind + "-" + instance
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
	wanted, applied, observed := "<unclear>", "<unclear>", "<unclear>"
	if v.Stages.Wanted != nil {
		wanted = versionCellText(v.Stages.Wanted)
	}
	if v.Stages.Applied != nil {
		applied = versionCellText(v.Stages.Applied)
	}
	if v.Stages.Observed != nil {
		observed = versionCellText(v.Stages.Observed)
	}
	fmt.Fprintf(out, "%-12s %-9s %s\n", truncateVersionCell(wanted), truncateVersionCell(applied), truncateVersionCell(observed))
	if len(result.Components) > 0 {
		fmt.Fprintln(out, "\nComponents (image repository/tag; observed identity):")
		fmt.Fprintf(out, "%-18s %-38s %-16s %s\n", "SERVICE", "REPOSITORY", "TAG", "OBSERVATION")
		for _, component := range result.Components {
			tag := component.Tag
			if tag == "" {
				tag = "<unclear>"
			}
			repository := component.Repository
			if repository == "" {
				repository = component.Image
			}
			fmt.Fprintf(out, "%-18s %-38s %-16s %s\n", component.Service, truncateColumn(repository, 38), truncateColumn(tag, 16), component.Observation)
		}
	}
	for _, warning := range v.Warnings {
		fmt.Fprintf(out, "[WARN] %s\n", warning)
	}
}

func versionCellText(cell *machinereadmodels.ViewStageCell) string {
	if cell.MissingData || strings.TrimSpace(cell.Reference) == "" {
		return "UNCLEAR"
	}
	text := cell.Reference
	if cell.Stale {
		text += " [stale]"
	}
	return text
}

func truncateVersionCell(text string) string {
	return truncateColumn(text, 12)
}

func truncateColumn(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}
