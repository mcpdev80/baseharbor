package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

// Card 0428-062: app status + versions unified over the delivered source
// model. These tests pin the unification contract: one version section
// rendered identically by status, version and versions; the three separated
// reference dimensions preserved; unknown Git/image digest states explicit;
// -c filters and --all-targets preserved; no invented fields and no second
// parallel truth.

func unifiedVersionRecord(t *testing.T, appID, name string, references map[string]*deployment.ReferenceDimension) deployment.DeploymentRecord {
	t.Helper()
	identity, err := deployment.NewDeploymentIdentity("docker-dev", appID, name, "dev")
	if err != nil {
		t.Fatal(err)
	}
	return deployment.DeploymentRecord{
		Version:    deployment.DeploymentRecordVersion,
		Identity:   identity,
		Source:     deployment.DeploymentSource{Kind: "managed", Repository: "/synthetic/repo"},
		Applied:    deployment.AppliedDeployment{RuntimeProvider: "docker", Intent: mustIntentSnapshotJSON(t, application.WithWorkloadComponents(application.Manifest{Version: application.CurrentVersion, ApplicationID: appID, Name: name, Environment: "dev"}, "api", "worker"))},
		Observed:   deployment.ObservedDeployment{State: "ready", Ready: true},
		References: references,
	}
}

// TestStatusVersionsSectionRendersSeparatedCells proves the status view keeps
// the wanted/applied/observed machine fields intact and renders them from the
// delivered read model, one cell per recorded dimension.
func TestStatusVersionsSectionRendersSeparatedCells(t *testing.T) {
	resolved := resolvedApplication{
		Manifest: application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev"},
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "wanted-ref"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "applied-ref"),
			deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "observed-ref"),
		})),
	}
	versions, warnings := collectAppStatusVersions(resolved)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if versions.Stages.Wanted.Reference != "wanted-ref" || versions.Stages.Applied.Reference != "applied-ref" || versions.Stages.Observed.Reference != "observed-ref" {
		t.Fatalf("cells must map one-to-one from the recorded dimensions: %+v", versions.Stages)
	}
	var out bytes.Buffer
	renderVersionsSection(&out, versions, warnings)
	text := out.String()
	for _, want := range []string{"WANTED", "APPLIED", "OBSERVED", "wanted-ref", "applied-ref", "observed-ref"} {
		if !strings.Contains(text, want) {
			t.Fatalf("status versions section must contain %q, got:\n%s", want, text)
		}
	}
	// Component rows come from the declared identities: the logical workload
	// components of the intent, never an invented image version.
	for _, want := range []string{"api", "worker", "unknown"} {
		if !strings.Contains(text, want) {
			t.Fatalf("component rows must show the declared identities, missing %q:\n%s", want, text)
		}
	}
}

func ptrRecord(r deployment.DeploymentRecord) *deployment.DeploymentRecord { return &r }

// TestStatusVersionsUnknownDimensionIsUnclearNeverBlank pins the acceptance
// line: a dimension without a recorded version (e.g. unknown Git revision or
// image digest) is UNCLEAR, not a blank and not an inferred value.
func TestStatusVersionsUnknownDimensionIsUnclearNeverBlank(t *testing.T) {
	resolved := resolvedApplication{
		Manifest: application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev"},
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:known"),
		})),
	}
	versions, warnings := collectAppStatusVersions(resolved)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !versions.Stages.Applied.MissingData || versions.Stages.Applied.Reference != machinereadmodels.UnclearValue {
		t.Fatalf("the unrecorded applied dimension must be the explicit unclear cell: %+v", versions.Stages.Applied)
	}
	if !versions.Stages.Observed.MissingData || versions.Stages.Observed.Stale != true {
		t.Fatalf("no observation must count as stale, never as up to date: %+v", versions.Stages.Observed)
	}
	var out bytes.Buffer
	renderVersionsSection(&out, versions, warnings)
	if !strings.Contains(out.String(), "UNCLEAR") {
		t.Fatalf("human view must render UNCLEAR instead of a blank:\n%s", out.String())
	}
}

// TestStatusAndVersionsComponentRowsAreOneView proves unification: the status
// view and the aggregate versions view derive component rows from the same
// declared identities and the same wanted reference, so no parallel
// reality can drift between them.
func TestStatusAndVersionsComponentRowsAreOneView(t *testing.T) {
	resolved := resolvedApplication{
		Manifest: application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev"},
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:wanted"),
			deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "sha256:wanted"),
		})),
	}
	statusRows, _ := collectSourceModelComponents(resolved, mustView(t).Stages)
	record := *resolved.DeploymentRecord
	aggregateRows, aggregateWarnings := componentRowsForRecord(record, mustView(t).Stages)
	// The aggregate path lists many deployments; the ordinary "workspace not
	// initialized here" notice is readable in the rows themselves (unmapped)
	// and is suppressed per row, unlike the single-app status warning.
	if len(withoutAbsentSourceModelNotice(aggregateWarnings)) != 0 {
		t.Fatalf("unexpected aggregate warnings: %v", aggregateWarnings)
	}
	if len(statusRows) != len(aggregateRows) {
		t.Fatalf("both views must render the same rows: %d vs %d (%v / %v)", len(statusRows), len(aggregateRows), rowNames(statusRows), rowNames(aggregateRows))
	}
	_ = withoutAbsentSourceModelNotice
	for i := range statusRows {
		if statusRows[i].Service != aggregateRows[i].Service || statusRows[i].SourceRevision != aggregateRows[i].SourceRevision {
			t.Fatalf("row %d diverges between status and versions: %+v vs %+v", i, statusRows[i], aggregateRows[i])
		}
	}
}

func mustView(t *testing.T) machinereadmodels.VersionView {
	t.Helper()
	record := unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
		deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
		deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:wanted"),
		deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "sha256:wanted"),
	})
	return machinereadmodels.ReleaseViewFromRecord(record)
}

func rowNames(rows []machinereadmodels.ComponentVersion) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Service)
	}
	return names
}

// TestVersionsComponentRowsProjectSourceModel wires the DELIVERED source model
// (card 0428-059) into the unified component rows: the declared components
// gain their source provenance and a source-model component that names a
// backend keeps the backend image identity instead of an invented one.
func TestVersionsComponentRowsProjectSourceModel(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\nname: demo\nenvironment: dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion,
		Application:   "demo",
		Sources:       []development.SourceDefinition{{ID: "backend", Type: development.SourceRepository, Repository: "https://example.invalid/backend"}},
		Components:    []development.ComponentSource{{Component: "api", Source: "backend"}, {Component: "worker", Source: "backend"}},
	}
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	mapping := development.WorkspaceMapping{
		SchemaVersion: development.WorkspaceMappingVersion,
		Application:   "demo",
		Manifest:      manifestPath,
		Sources:       map[string]string{"backend": dir},
	}
	if _, err := development.SaveWorkspaceMapping(manifestPath, mapping); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		ManifestPath:    manifestPath,
		FromRepository:  true,
		SourceAvailable: true,
		Manifest:        application.WithWorkloadComponents(application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev"}, "api", "worker"),
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
		})),
	}
	rows, warnings := collectSourceModelComponents(resolved, mustView(t).Stages)
	if len(warnings) != 0 {
		t.Fatalf("resolvable source model must not warn: %v", warnings)
	}
	api := componentRowIndex(rows, "api")
	worker := componentRowIndex(rows, "worker")
	if api < 0 || worker < 0 {
		t.Fatalf("declared components must appear: %v", rowNames(rows))
	}
	if rows[api].Source != "backend" || rows[api].SourceType != "repository" {
		t.Fatalf("api row must carry its source-model provenance: %+v", rows[api])
	}
	if rows[api].SourceIdentity != dir {
		t.Fatalf("source identity must be the resolved checkout root: %+v", rows[api])
	}
	if rows[api].SourceRevision != "sha256:wanted" || rows[worker].SourceRevision != "sha256:wanted" {
		t.Fatalf("the wanted reference must flow into the rows unchanged: %+v %+v", rows[api], rows[worker])
	}
}

// TestVersionsUnresolvableSourceModelIsExplicitNotFailure proves the unified
// view never lets a missing or unmappable source model become a silent blank
// or a status failure: the rows stay, the provenance says unmapped and one
// warning explains why.
func TestVersionsUnresolvableSourceModelIsExplicitNotFailure(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "baseharbor.yaml")
	resolved := resolvedApplication{
		ManifestPath:   manifestPath,
		FromRepository: true,
		Manifest:       application.WithWorkloadComponents(application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev"}, "api"),
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
		})),
	}
	rows, warnings := collectSourceModelComponents(resolved, mustView(t).Stages)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unmapped") {
		t.Fatalf("a missing source model must surface one explicit unmapped warning: %v", warnings)
	}
	if componentRowIndex(rows, "api") < 0 || rows[componentRowIndex(rows, "api")].Source != "" {
		t.Fatalf("the declared component must remain with empty (unmapped) provenance: %+v", rows)
	}
}

// TestComponentSelectorParsesAndFilters pins -c COMPONENT / comma lists and
// the fail-closed unknown selection across the unified surfaces.
func TestComponentSelectorParsesAndFilters(t *testing.T) {
	selected, filtered, err := parseComponentSelectorArgs([]string{"app-name", "-c", "api,worker", "--component", "extra"}, "status")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(selected, ",") != "api,worker,extra" {
		t.Fatalf("selector parsing lost values: %v", selected)
	}
	if strings.Join(filtered, ",") != "app-name" {
		t.Fatalf("positional arguments must survive selection: %v", filtered)
	}
	rows := []machinereadmodels.ComponentVersion{{Service: "api"}, {Service: "worker"}, {Service: "postgres", Source: "backend"}}
	if err := requireKnownComponentSelections([]string{"ghost"}, rows, "status"); err == nil {
		t.Fatal("an unknown component selection must fail closed")
	}
	if err := requireKnownComponentSelections([]string{"backend"}, rows, "status"); err != nil {
		t.Fatalf("a source identity is a valid selection: %v", err)
	}
	kept := filterComponentRows(rows, []string{"api"})
	if len(kept) != 1 || kept[0].Service != "api" {
		t.Fatalf("filtering must keep exactly the selected row: %+v", kept)
	}
	if _, _, err := parseComponentSelectorArgs([]string{"-c", "-o"}, "status"); err == nil {
		t.Fatal("-c without a value must fail closed")
	}
	if _, _, err := parseComponentSelectorArgs([]string{"-c", "a,,"}, "status"); err == nil {
		t.Fatal("an empty list element must fail closed")
	}
}

// TestAppVersionsCommandUnifiedRowsCallable proves the aggregate `app
// versions -o json` carries the unified component rows through the machine
// read model, and -c narrows them while an unknown selection fails closed.
func TestAppVersionsCommandUnifiedRowsCallable(t *testing.T) {
	configureTestTarget(t)
	record := unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "versions-demo", map[string]*deployment.ReferenceDimension{
		deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
		deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:applied"),
		deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "sha256:applied"),
	})
	record.Identity.Application = "versions-demo"
	record.Identity.ApplicationID = "11111111-1111-4111-8111-111111111111"
	record.Identity, _ = deployment.NewDeploymentIdentity("docker-dev", "11111111-1111-4111-8111-111111111111", "versions-demo", "dev")
	record.Applied.Intent = mustIntentSnapshotJSON(t, application.WithWorkloadComponents(application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "versions-demo", Environment: "dev"}, "api"))
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "versions", "-o", "json"}, &out, &errOut); err != nil {
		t.Fatalf("app versions must stay callable: %v\n%s", err, errOut.String())
	}
	var payload struct {
		Releases []struct {
			Stages     map[string]json.RawMessage           `json:"stages"`
			Components []machinereadmodels.ComponentVersion `json:"components"`
		} `json:"releases"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Releases) != 1 || len(payload.Releases[0].Components) == 0 {
		t.Fatalf("the aggregate view must carry unified component rows: %s", out.String())
	}
	for _, key := range []string{"wanted", "applied", "observed"} {
		if _, ok := payload.Releases[0].Stages[key]; !ok {
			t.Fatalf("stages must keep the separated %q cell: %s", key, out.String())
		}
	}
	if payload.Releases[0].Components[0].Service != "api" || payload.Releases[0].Components[0].SourceRevision != "sha256:wanted" {
		t.Fatalf("the row must carry the wanted reference unchanged: %+v", payload.Releases[0].Components[0])
	}

	out.Reset()
	errOut.Reset()
	if err := runWithIO(context.Background(), []string{"app", "versions", "-c", "ghost", "-o", "json"}, &out, &errOut); err == nil {
		t.Fatalf("an unknown -c selection must fail closed:\n%s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if err := runWithIO(context.Background(), []string{"app", "versions", "--all-targets", "-c", "api"}, &out, &errOut); err != nil {
		t.Fatalf("--all-targets with -c must stay callable: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "api") {
		t.Fatalf("the human aggregate must render the component rows:\n%s", out.String())
	}
}

// TestStatusJSONCarriesVersionsSection proves CLI/JSON/MCP parity through the
// shared collectApplicationStatusResult seam: the machine result embeds the
// separated stages and component rows, and the unknown-dimension state stays
// explicit in JSON, never blank.
func TestStatusJSONCarriesVersionsSection(t *testing.T) {
	resolved := resolvedApplication{
		Manifest: application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev"},
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:wanted"),
		})),
	}
	versions, warnings := collectAppStatusVersions(resolved)
	result := applicationStatusResult{StatusResult: application.StatusResult{ContractVersion: "v1", Application: "demo", Environment: "dev", State: "running", Ready: true, Checks: []application.StatusCheck{}}, Versions: versions, VersionWarnings: warnings}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Versions struct {
			Stages struct {
				Wanted   map[string]any `json:"wanted"`
				Applied  map[string]any `json:"applied"`
				Observed map[string]any `json:"observed"`
			} `json:"stages"`
			Components []machinereadmodels.ComponentVersion `json:"components"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if ref, _ := decoded.Versions.Stages.Wanted["reference"].(string); ref != "sha256:wanted" {
		t.Fatalf("the machine field must keep the wanted dimension: %v", decoded.Versions.Stages.Wanted)
	}
	observed := decoded.Versions.Stages.Observed
	if missing, _ := observed["missing_data"].(bool); !missing {
		t.Fatalf("an unrecorded dimension must be explicit missing_data in JSON: %v", observed)
	}
	if ref, _ := observed["reference"].(string); strings.TrimSpace(ref) == "" {
		t.Fatalf("the machine cell must never be blank: %v", observed)
	}
}

// TestVersionSectionCarriesNoSecrets pins the secret-protection rule for the
// new section: normal output never includes secret values or credential
// material even when warnings quote resolution errors.
func TestVersionSectionCarriesNoSecrets(t *testing.T) {
	versions := appStatusVersions{
		Stages: machinereadmodels.ViewStages{
			Wanted:   &machinereadmodels.ViewStageCell{Kind: "intended", Reference: "sha256:abc"},
			Applied:  &machinereadmodels.ViewStageCell{Kind: "applied", Reference: "sha256:abc"},
			Observed: &machinereadmodels.ViewStageCell{Kind: "observed", Reference: "sha256:def", Stale: true},
		},
		Components: []machinereadmodels.ComponentVersion{{Service: "api", Source: "backend", SourceType: "repository", SourceIdentity: "https://example.invalid/backend"}},
	}
	var out bytes.Buffer
	renderVersionsSection(&out, versions, []string{"component source provenance is unmapped: mapping drifted"})
	text := out.String()
	for _, forbidden := range []string{"password", "token", "secret_key=", "://user:"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("the version section must never leak %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "[stale]") {
		t.Fatalf("a divergent observation must stay labeled stale:\n%s", text)
	}
}
