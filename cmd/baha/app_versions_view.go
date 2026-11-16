package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

// Unified app status/versions read-model seam (card 0428-062). The status,
// version and versions views consume the delivered record-derived
// wanted/applied/observed cells (internal/machinereadmodels over the accepted
// stage versions) and the delivered portable source model (card 0428-059).
// Nothing here opens a second parallel truth: a reference dimension the
// product never recorded stays UNCLEAR, and a component nobody declared is
// never invented.

// appStatusVersions is the version dimension of one deployment inside the
// status and version views: the three separated cells plus one row per
// declared component identity.
type appStatusVersions struct {
	Stages     machinereadmodels.ViewStages         `json:"stages"`
	Components []machinereadmodels.ComponentVersion `json:"components,omitempty"`
}

// collectAppStatusVersions derives the version section for one resolved
// deployment. The persisted record is the authoritative source of the three
// reference dimensions, read through the same seam the delivered `app version`
// view uses; a repository resolution may not carry an in-memory copy.
func collectAppStatusVersions(resolved resolvedApplication) (appStatusVersions, []string) {
	record := deployment.DeploymentRecord{Version: deployment.DeploymentRecordVersion, Identity: resolved.DeploymentIdentity}
	if resolved.DeploymentRecord != nil {
		record = *resolved.DeploymentRecord
	} else if fresh, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity); err == nil {
		record = fresh
	}
	view := machinereadmodels.ReleaseViewFromRecord(record)
	components, warnings := collectSourceModelComponents(resolved, view.Stages)
	return appStatusVersions{Stages: view.Stages, Components: components}, warnings
}

// collectSourceModelComponents renders one row per component identity this
// application declares: the backend service instances from the (optionally
// applied) intent, the logical workload components, merged with the portable
// source-model projections for their source provenance. A source model that
// cannot resolve (missing model, missing developer-local mapping, drifted
// identity) is never a status failure: the explicit "unmapped" column state
// and a warning carry it instead, and no neighbouring repository is ever
// scanned.
func collectSourceModelComponents(resolved resolvedApplication, stages machinereadmodels.ViewStages) ([]machinereadmodels.ComponentVersion, []string) {
	var warnings []string
	desired := ""
	if stages.Wanted != nil && !stages.Wanted.MissingData {
		desired = stages.Wanted.Reference
	}
	intent := resolved.Manifest
	if resolved.DeploymentRecord != nil && len(resolved.DeploymentRecord.Applied.Intent) > 0 {
		var applied application.Manifest
		if err := json.Unmarshal(resolved.DeploymentRecord.Applied.Intent, &applied); err == nil && applied.Name != "" {
			intent = applied
		}
	}
	declared := declaredVersionServices(intent)
	byName := make(map[string]struct{}, len(declared))
	components := make([]machinereadmodels.ComponentVersion, 0, len(declared))
	for _, service := range declared {
		components = append(components, componentRowFromWanted(service, desired))
		byName[service.name] = struct{}{}
	}
	// The application's own logical workload components are declared identities
	// of the deployment too; they carry no BaseHarbor-owned image version.
	for _, component := range application.WorkloadComponentNames(intent) {
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
	if !resolved.FromRepository || strings.TrimSpace(resolved.ManifestPath) == "" {
		return sortComponentRows(components), warnings
	}
	projections, err := projectSourceModelComponents(resolved)
	if err != nil {
		if errorsIsModelMissing(err) {
			return sortComponentRows(components), append(warnings, "no portable source model exists yet; component source provenance is unmapped")
		}
		return sortComponentRows(components), append(warnings, "component source provenance is unmapped: "+err.Error())
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
	return sortComponentRows(components), warnings
}

func componentRowIndex(components []machinereadmodels.ComponentVersion, name string) int {
	for i, component := range components {
		if component.Service == name {
			return i
		}
	}
	return -1
}

func errorsIsModelMissing(err error) bool {
	return errors.Is(err, development.ErrWorkspaceModelMissing) || os.IsNotExist(err)
}

// componentSourceProjection is one component's source provenance inside the
// unified status/version component rows.
type componentSourceProjection struct {
	Component      string
	Source         string
	SourceType     string
	SourceIdentity string
	SubPath        string
}

// projectSourceModelComponents returns the component-to-source projections of
// the delivered source model, or an explicit error when the model or its
// developer-local mapping cannot resolve. The projection reuses the same
// ResolveWorkspace seam `app workspace resolve` delivers.
func projectSourceModelComponents(resolved resolvedApplication) ([]componentSourceProjection, error) {
	model, _, err := development.LoadSourceModel(resolved.ManifestPath)
	if err != nil {
		return nil, err
	}
	if model.Application != resolved.Manifest.Name {
		return nil, fmt.Errorf("source model application %q does not match application %q", model.Application, resolved.Manifest.Name)
	}
	mapping, _, err := development.LoadWorkspaceMapping(resolved.ManifestPath, resolved.Manifest.Name)
	if err != nil {
		return nil, err
	}
	resolution, resolveErr := development.ResolveWorkspace(resolved.ManifestPath, model, mapping)
	if resolveErr != nil {
		fallback, partial := partialWorkspaceResolution(model)
		if len(partial) == 0 {
			return nil, resolveErr
		}
		resolution = fallback
	}
	projections := make([]componentSourceProjection, 0, len(resolution.Components))
	for _, component := range resolution.Components {
		identity := componentSourceIdentity(component)
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

// componentSourceIdentity names the source identity actually resolved for the
// component: the image for OCI sources, otherwise the concrete checkout or
// directory root the delivered resolution produced.
func componentSourceIdentity(component development.ResolvedComponentSource) string {
	if component.Type == development.SourceOCI {
		return component.Image
	}
	return component.Root
}

func componentRowFromWanted(service declaredVersionService, desired string) machinereadmodels.ComponentVersion {
	row := machinereadmodels.ComponentVersion{
		Service:        service.name,
		Image:          service.image,
		SourceRevision: desired,
		Observation:    "declared backend component; no source model mapping exists for it",
	}
	row.Repository, row.Tag, _ = machinereadmodels.SplitImageReference(service.image)
	if row.Tag == "" {
		row.Tag = machinereadmodels.UnclearValue
	}
	// Annotation-derived metadata is a claim, never verified provenance.
	row.AnnotationNote = "declared image metadata claim; provenance unverified unless separately attested"
	return row
}

func sortComponentRows(components []machinereadmodels.ComponentVersion) []machinereadmodels.ComponentVersion {
	sort.SliceStable(components, func(i, j int) bool { return components[i].Service < components[j].Service })
	return components
}

// parseComponentSelectorArgs strips -c/--component selections (single value or
// comma-separated list, repeatable) from a read command's arguments. An empty
// selection value fails closed with an actionable usage error.
func parseComponentSelectorArgs(args []string, command string) (selected []string, filtered []string, err error) {
	filtered = make([]string, 0, len(args))
	add := func(value string) error {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				return usageError("empty component selection", "List non-empty component or source identities: -c api,worker.")
			}
			if !containsExactString(selected, part) {
				selected = append(selected, part)
			}
		}
		return nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c" || arg == "--component":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, nil, usageError(arg+" requires COMPONENT[,COMPONENT...]", "Run 'baha app "+command+" --help'.")
			}
			i++
			if err := add(args[i]); err != nil {
				return nil, nil, err
			}
		case strings.HasPrefix(arg, "--component="):
			if err := add(strings.TrimPrefix(arg, "--component=")); err != nil {
				return nil, nil, err
			}
		default:
			filtered = append(filtered, arg)
		}
	}
	return selected, filtered, nil
}

func containsExactString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// requireKnownComponentSelections refuses -c selections that name neither a
// declared component identity nor a source-model component: filtering by an
// unknown identity must fail closed, not silently show an empty section.
func requireKnownComponentSelections(selected []string, components []machinereadmodels.ComponentVersion, command string) error {
	for _, name := range selected {
		found := false
		for _, component := range components {
			if component.Service == name || component.Source == name {
				found = true
				break
			}
		}
		if !found {
			return usageError("component "+name+" is not declared for this application", "List declared components with 'baha app workspace component list' or drop -c to show all components. Run 'baha app "+command+" --help'.")
		}
	}
	return nil
}

// filterComponentRows keeps the rows whose component identity or source ID is
// selected by -c/--component.
func filterComponentRows(components []machinereadmodels.ComponentVersion, selected []string) []machinereadmodels.ComponentVersion {
	if len(selected) == 0 {
		return components
	}
	result := make([]machinereadmodels.ComponentVersion, 0, len(components))
	for _, component := range components {
		for _, name := range selected {
			if component.Service == name || component.Source == name {
				result = append(result, component)
				break
			}
		}
	}
	return result
}

// renderVersionsSection renders the separated reference dimensions plus the
// component rows inside the human status view.
func renderVersionsSection(out io.Writer, versions appStatusVersions, warnings []string) {
	fmt.Fprintln(out, "\nVersions (wanted, applied and observed stay separate recorded dimensions):")
	fmt.Fprintf(out, "%-12s %-9s %s\n", "WANTED", "APPLIED", "OBSERVED")
	fmt.Fprintf(out, "%s %s %s\n",
		versionCellText(versions.Stages.Wanted),
		versionCellText(versions.Stages.Applied),
		versionCellText(versions.Stages.Observed))
	for _, warning := range warnings {
		fmt.Fprintf(out, "[WARN] %s\n", warning)
	}
	if len(versions.Components) == 0 {
		fmt.Fprintln(out, "Components: none declared")
		return
	}
	fmt.Fprintln(out, "Components:")
	for _, component := range versions.Components {
		fmt.Fprintln(out, "  "+formatComponentRow(component))
	}
}

// formatComponentRow is the shared human line for one component row: identity,
// source provenance (explicit "unmapped" when none), and the readable version
// or image reference with its observation state.
func formatComponentRow(component machinereadmodels.ComponentVersion) string {
	origin := component.Source
	if origin == "" {
		origin = "unmapped"
	} else if component.SourceType != "" {
		origin = component.SourceType + " " + origin
	}
	if component.SourceIdentity != "" {
		origin += " " + component.SourceIdentity
	}
	if component.SubPath != "" && component.SubPath != "." {
		origin += " @" + filepath.ToSlash(component.SubPath)
	}
	version := component.Image
	if version == "" && component.Repository != "" {
		version = component.Repository
		if component.Tag != "" && component.Tag != machinereadmodels.UnclearValue {
			version += ":" + component.Tag
		}
	}
	if version == "" {
		version = "unknown"
	}
	if component.Digest != "" {
		version += " " + component.Digest
	}
	return fmt.Sprintf("%-18s %-32s %s (%s)", component.Service, truncateColumn(origin, 32), version, component.Observation)
}

// declaredVersionService is one backend component identity with its provider
// image reference.
type declaredVersionService struct {
	name  string
	image string
}

// declaredVersionServices enumerates the backend component identities this
// application realizes INSIDE its own runtime project, reusing the accepted
// placement and instance-name seams so the view never restates provider
// naming and never restates the shared-provider rule. Shared providers are
// Target-level resources, not this deployment's components: `app status`
// reports their readiness through capability checks, and the version view must
// not claim an image version the application does not own. The application's
// own workload image is deliberately not listed either: BaseHarbor does not
// own it and must not invent its version. Backend images come from the
// delivered pinned-provider contract, never from a parallel literal here.
func declaredVersionServices(m application.Manifest) []declaredVersionService {
	var services []declaredVersionService
	add := func(name, image string) {
		services = append(services, declaredVersionService{name: name, image: image})
	}
	for _, instance := range application.SQLInstanceNames(m) {
		if application.UsesSharedPostgreSQL(m) {
			continue
		}
		add(runtimeServiceNameForView("postgres", instance), pinnedRuntimeImage("postgres"))
	}
	for _, instance := range application.ValkeyInstanceNames(m) {
		if application.UsesSharedValkey(m) {
			continue
		}
		add(runtimeServiceNameForView("valkey", instance), pinnedRuntimeImage("valkey"))
	}
	for _, instance := range application.RabbitMQInstanceNames(m) {
		add(runtimeServiceNameForView("rabbitmq", instance), pinnedRuntimeImage("rabbitmq"))
	}
	for _, instance := range application.DocumentDatabaseInstanceNames(m) {
		add(runtimeServiceNameForView("mongodb", instance), application.MongoDBImage)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].name < services[j].name })
	return services
}

// viewPlacementServices renders the backend component rows for the version
// view under the record's retained placement: the retained placement (when
// the persisted record carries one) replaces the ambient organization and
// environment policy for the duration of the read, and the ambient
// environment is restored afterwards so nothing leaks into later calls.
// Without a retained placement the ambient classification is respected: the
// view never invents a backend row the deployment does not own.
func viewPlacementServices(
	retained *capability.ProviderPlacement,
	m application.Manifest,
) ([]declaredVersionService, func()) {
	if retained == nil {
		return declaredVersionServices(m), func() {}
	}
	scopeEnv := application.ProviderScopeEnv(capability.ProviderPostgreSQL)
	previous, hadPrev := os.LookupEnv(scopeEnv)
	restore := func() {
		if hadPrev {
			_ = os.Setenv(scopeEnv, previous)
		} else {
			_ = os.Unsetenv(scopeEnv)
		}
	}
	_ = os.Setenv(scopeEnv, string(retained.Scope))
	return declaredVersionServices(m), restore
}

// pinnedRuntimeImage reports the backend provider image the delivered runtime
// renderers pin for one backend kind. The values live in exactly one place in
// this file and mirror the shipped renderer images; the separation regression
// TestUnifiedViewPinnedImagesMatchRenderers fails if a renderer image moves
// while this view keeps the old literal.
func pinnedRuntimeImage(kind string) string {
	switch kind {
	case "postgres":
		return "docker.io/library/postgres:18-alpine"
	case "valkey":
		return "docker.io/valkey/valkey:9.1.2-alpine"
	case "rabbitmq":
		return "docker.io/library/rabbitmq:4.3.6-alpine"
	default:
		return ""
	}
}

// runtimeServiceNameForView mirrors the runtime owner's service naming without
// exporting a new product API from the application package.
func runtimeServiceNameForView(kind, instance string) string {
	if instance == "" || instance == "default" {
		return kind
	}
	return kind + "-" + instance
}

// versionCellText renders one separated reference dimension: an unrecorded
// dimension is explicit UNCLEAR, never blank; a stale observation is labeled.
func versionCellText(cell *machinereadmodels.ViewStageCell) string {
	if cell == nil || cell.MissingData || strings.TrimSpace(cell.Reference) == "" {
		return machinereadmodels.UnclearSummary
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
