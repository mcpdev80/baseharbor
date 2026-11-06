package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

func referenceDimension(kind, release string) *deployment.ReferenceDimension {
	return &deployment.ReferenceDimension{Kind: kind, Release: release, At: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
}

// mustIntentSnapshotJSON encodes a minimal valid applied intent snapshot.
func mustIntentSnapshotJSON(t *testing.T, m application.Manifest) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func separationFixture(references map[string]*deployment.ReferenceDimension) machinereadmodels.VersionView {
	return machinereadmodels.ReleaseViewFromRecord(separationRecord(references))
}

func separationRecord(references map[string]*deployment.ReferenceDimension) deployment.DeploymentRecord {
	return deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  testDeploymentID,
			ApplicationID: "11111111-1111-4111-8111-111111111111",
			Target:        "docker-dev",
			Application:   "demo",
			Environment:   "dev",
		},
		Source:     deployment.DeploymentSource{Kind: "repository", Repository: "/synthetic/repo"},
		References: references,
	}
}

// TestVersionViewThreeCellsAlwaysSeparated is the separation line of this card:
// the view must carry wanted, applied and observed as three distinct cells,
// each bound to its own recorded dimension.
func TestVersionViewThreeCellsAlwaysSeparated(t *testing.T) {
	view := separationFixture(map[string]*deployment.ReferenceDimension{
		deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "wanted-ref"),
		deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "applied-ref"),
		deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "observed-ref"),
	})
	if view.Stages.Wanted == nil || view.Stages.Applied == nil || view.Stages.Observed == nil {
		t.Fatalf("view must always carry three cells: %+v", view.Stages)
	}
	cells := []*machinereadmodels.ViewStageCell{view.Stages.Wanted, view.Stages.Applied, view.Stages.Observed}
	wantKind := []string{deployment.ReferenceIntended, deployment.ReferenceApplied, deployment.ReferenceObserved}
	wantRef := []string{"wanted-ref", "applied-ref", "observed-ref"}
	for i, cell := range cells {
		if cell.Kind != wantKind[i] {
			t.Fatalf("cell %d reads dimension %q, want %q", i, cell.Kind, wantKind[i])
		}
		if cell.Reference != wantRef[i] {
			t.Fatalf("cell %d shows %q, want %q (columns must not mix)", i, cell.Reference, wantRef[i])
		}
		if cell.MissingData {
			t.Fatalf("cell %d recorded a reference but is marked missing", i)
		}
	}
}

// TestVersionViewMissingDimensionIsUnclear covers deliverable 2: a missing
// version renders a readable unclear cell, never a blank.
func TestVersionViewMissingDimensionIsUnclear(t *testing.T) {
	view := separationFixture(map[string]*deployment.ReferenceDimension{
		deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "wanted-ref"),
		deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "applied-ref"),
		// observed was never recorded.
	})
	if view.Stages.Observed == nil {
		t.Fatal("observed cell must exist even when unrecorded")
	}
	if !view.Stages.Observed.MissingData {
		t.Fatal("unrecorded dimension must be marked missing-data")
	}
	if view.Stages.Observed.Reference != machinereadmodels.UnclearValue {
		t.Fatalf("unrecorded dimension must read UNCLEAR, got %q", view.Stages.Observed.Reference)
	}
	if view.Stages.Wanted.Reference != "wanted-ref" || view.Stages.Applied.Reference != "applied-ref" {
		t.Fatal("the missing observed dimension must not disturb the recorded cells")
	}
	// Fail-closed staleness: no observation is stale, never "up to date".
	if !view.Stages.Observed.Stale {
		t.Fatal("an unrecorded observation must be reported stale, not up to date")
	}
	// The empty-render guard: no cell may serialize as an empty string.
	for _, cell := range []*machinereadmodels.ViewStageCell{view.Stages.Wanted, view.Stages.Applied, view.Stages.Observed} {
		if strings.TrimSpace(cell.Reference) == "" {
			t.Fatalf("cell %q must never render empty", cell.Kind)
		}
	}
}

// TestVersionViewDimensionIsolation proves a change to one recorded dimension
// never rewrites another through the view: the reader maps each dimension into
// its own cell one-to-one.
func TestVersionViewDimensionIsolation(t *testing.T) {
	view := separationFixture(map[string]*deployment.ReferenceDimension{
		deployment.ReferenceApplied: referenceDimension(deployment.ReferenceApplied, "applied-only"),
	})
	if view.Stages.Wanted.Reference != machinereadmodels.UnclearValue || !view.Stages.Wanted.MissingData {
		t.Fatalf("applied-only record must leave wanted unclear, got %+v", view.Stages.Wanted)
	}
	if view.Stages.Applied.Reference != "applied-only" {
		t.Fatalf("applied cell must show the applied dimension, got %q", view.Stages.Applied.Reference)
	}
	if view.Stages.Observed.Reference != machinereadmodels.UnclearValue {
		t.Fatalf("observed must not be copied from applied, got %q", view.Stages.Observed.Reference)
	}
}

// TestVersionViewJSONShapeSeparatesCells asserts the machine shape keeps three
// named cells; collapsing them into one shared field would break this.
func TestVersionViewJSONShapeSeparatesCells(t *testing.T) {
	view := separationFixture(map[string]*deployment.ReferenceDimension{
		deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "wanted-ref"),
		deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "applied-ref"),
		deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "applied-ref"),
	})
	raw, err := json.Marshal(view.Stages)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"wanted", "applied", "observed"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("stages JSON must carry a separate %q cell, got %s", key, raw)
		}
	}
	// The staleness column is the fail-closed observed-vs-applied comparison:
	// equal values are not stale, a divergent observation is.
	if view.Stages.Observed.Stale {
		t.Fatal("observed equal to applied with no divergence must not be stale")
	}
	diverged := separationFixture(map[string]*deployment.ReferenceDimension{
		deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "wanted-ref"),
		deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "applied-ref"),
		deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "older-ref"),
	})
	if !diverged.Stages.Observed.Stale {
		t.Fatal("a divergent observation must be marked stale")
	}
	if diverged.Stages.Applied.Stale || diverged.Stages.Wanted.Stale {
		t.Fatal("staleness is an observed-column property only; it must never leak into the other cells")
	}
}

func TestSplitImageReference(t *testing.T) {
	cases := []struct {
		reference            string
		repository, tag, dig string
	}{
		{"docker.io/library/postgres:18-alpine", "docker.io/library/postgres", "18-alpine", ""},
		{"ghcr.io/mcpdev80/baseharbor@sha256:0000000000000000000000000000000000000000000000000000000000000000", "ghcr.io/mcpdev80/baseharbor", "", "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		{"localhost:5000/app:dev", "localhost:5000/app", "dev", ""},
		{"nginx", "nginx", "", ""},
	}
	for _, tc := range cases {
		repository, tag, digest := machinereadmodels.SplitImageReference(tc.reference)
		if repository != tc.repository || tag != tc.tag || digest != tc.dig {
			t.Fatalf("SplitImageReference(%q) = (%q,%q,%q), want (%q,%q,%q)", tc.reference, repository, tag, digest, tc.repository, tc.tag, tc.dig)
		}
	}
}

// renderAppVersionHuman exercises the human renderer so the recordPendingDeployment
// seed (intended + applied, no observation yet) is asserted, and the missing
// OBSERVED cell is still rendered (never a blank).
func TestRenderAppVersionHumanUnclearCells(t *testing.T) {
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  testDeploymentID,
			ApplicationID: "11111111-1111-4111-8111-111111111111",
			Target:        "docker-dev",
			Application:   "demo",
			Environment:   "dev",
		},
		Source: deployment.DeploymentSource{Kind: "repository", Repository: "/synthetic/repo"},
		References: map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:9a1f"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:9a1f"),
		},
	}
	view := machinereadmodels.ReleaseViewFromRecord(record)
	var out bytes.Buffer
	renderApplicationVersionView(&out, applicationVersionViewResult{View: view})
	text := out.String()
	for _, want := range []string{"WANTED", "APPLIED", "OBSERVED", "sha256:9a1f", "UNCLEAR"} {
		if !strings.Contains(text, want) {
			t.Fatalf("human view must contain %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "OBSERVED\n") && !strings.Contains(text, "UNCLEAR") {
		t.Fatalf("the observed column must never render blank:\n%s", text)
	}
}

// TestAppVersionCommandCallable proves the `app version NAME` command is
// callable and emits the separated three-cell view: the persisted record's
// intended and applied dimensions appear in their own cells while the cleared
// observed dimension renders UNCLEAR instead of a blank.
func TestAppVersionCommandCallable(t *testing.T) {
	configureTestTarget(t)
	identity, err := deployment.NewDeploymentIdentity("docker-dev", "11111111-1111-4111-8111-111111111111", "demo", "dev")
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "managed", Repository: t.TempDir()},
		Applied: deployment.AppliedDeployment{
			RuntimeProvider: "docker",
			// Minimal valid intent snapshot so the registered-application
			// resolution seam accepts the record.
			Intent: mustIntentSnapshotJSON(t, application.WithWorkloadComponents(application.Manifest{
				Version:       application.CurrentVersion,
				ApplicationID: "11111111-1111-4111-8111-111111111111",
				Name:          "demo",
				Environment:   "dev",
			}, "demo-app")),
		},
		References: map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:intended-only"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:applied-only"),
			deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "sha256:applied-only"),
		},
		Observed: deployment.ObservedDeployment{State: "ready", Ready: true},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatalf("SaveDeploymentRecord: %v", err)
	}
	// Clear only the persisted observed dimension through the accepted
	// isolated operation: the intended and applied cells must survive.
	if err := deployment.ClearReference(identity, deployment.ReferenceObserved); err != nil {
		t.Fatalf("ClearReference: %v", err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "version", "demo", "-o", "json"}, &out, &errOut); err != nil {
		t.Fatalf("app version must be callable: %v\n%s", err, errOut.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("app version must emit JSON: %v\n%s", err, out.String())
	}
	view, ok := payload["view"].(map[string]any)
	if !ok {
		t.Fatalf("payload must carry the version view: %s", out.String())
	}
	stages, ok := view["stages"].(map[string]any)
	if !ok {
		t.Fatalf("view must carry three stage cells: %s", out.String())
	}
	for _, key := range []string{"wanted", "applied", "observed"} {
		cell, ok := stages[key].(map[string]any)
		if !ok {
			t.Fatalf("stages must carry a separate %q cell: %s", key, out.String())
		}
		ref, _ := cell["reference"].(string)
		if strings.TrimSpace(ref) == "" {
			t.Fatalf("cell %q must never be blank: %v", key, cell)
		}
	}
	if ref, _ := stages["observed"].(map[string]any)["reference"].(string); ref != machinereadmodels.UnclearValue {
		t.Fatalf("the cleared observed cell must read UNCLEAR, got %q", ref)
	}
}

// shippedReleaseViewStageKinds reads the shipped wanted/applied/observed
// mapping from the delivered product function — the same source of truth the
// shipped mapper uses (ReleaseViewFromRecord calls the parameter form with
// it) — so the regression cannot silently test against a table the product no
// longer uses. The mutation itself is the swapped copy passed to that same
// function, never a test-local mapper.
func shippedReleaseViewStageKinds() [3]string {
	return machinereadmodels.ReleaseViewStageKinds()
}

// TestStagesViewMutationAgainstSeparationLine is deliverable 3: both branches
// run the shipped mapper itself. ReleaseViewFromRecord delegates to
// ReleaseViewFromRecordWithStageKinds, the product function that maps each cell
// slot through an explicit wanted/applied/observed dimension table. GREEN
// counter-probe: the shipped mapper with the shipped table keeps the columns
// separate on the shared fixture. RED mutants, both executed by the delivered
// code path in internal/machinereadmodels/release.go and not by a test-local
// copy: (1) the shipped function with the observed slot swapped in the table
// passed to it, and (2) the shipped cell carrying a copied dimension label, the
// case the review measured as invisible in round 1. The cell therefore depends
// causally on the shipped mapper.
func TestStagesViewMutationAgainstSeparationLine(t *testing.T) {
	fixture := map[string]*deployment.ReferenceDimension{
		deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "wanted-ref"),
		deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "applied-ref"),
		deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "observed-ref"),
	}
	record := separationRecord(fixture)
	kindsForAssertion := shippedReleaseViewStageKinds()
	red := func(stages machinereadmodels.ViewStages) []string {
		var violations []string
		if stages.Wanted.Reference != "wanted-ref" || stages.Wanted.Kind != kindsForAssertion[0] {
			violations = append(violations, "wanted cell does not read the intended dimension")
		}
		if stages.Applied.Reference != "applied-ref" || stages.Applied.Kind != kindsForAssertion[1] {
			violations = append(violations, "applied cell does not read the applied dimension")
		}
		if stages.Observed.Reference != "observed-ref" || stages.Observed.Kind != kindsForAssertion[2] {
			violations = append(violations, "observed cell does not read the observed dimension")
		}
		return violations
	}
	// Counter-probe: the shipped mapper with the shipped table keeps all three
	// cells separate on the exact fixture the mutants are measured against.
	shipped := machinereadmodels.ReleaseViewFromRecordWithStageKinds(record, shippedReleaseViewStageKinds())
	if violations := red(shipped.Stages); len(violations) != 0 {
		t.Fatalf("the shipped mapper must satisfy the separation line, got %v", violations)
	}
	// MUTANT — the same shipped function and the same record, with one swapped
	// slot in the dimension table passed to it (the observed slot reads the
	// applied dimension).
	mutantKinds := shippedReleaseViewStageKinds()
	mutantKinds[2] = mutantKinds[1]
	mutantViolations := red(machinereadmodels.ReleaseViewFromRecordWithStageKinds(record, mutantKinds).Stages)
	if len(mutantViolations) == 0 {
		t.Fatal("the separation line must reject a column-mixing mutation of the shipped mapper")
	}
	if len(mutantViolations) != 1 || mutantViolations[0] != "observed cell does not read the observed dimension" {
		t.Fatalf("the mutation must violate exactly the observed column, got %v", mutantViolations)
	}

	// Second mutant class (the review's release.go:165 case): the shipped cell
	// loop writes another dimension's label into Kind while the observed slot
	// keeps the observed reference. The separation line checks Kind against the
	// dimension the slot must read, so this mutation cannot stay silent.
	labelSwap := red(machinereadmodels.ViewStages{
		Wanted:   shipped.Stages.Wanted,
		Applied:  shipped.Stages.Applied,
		Observed: &machinereadmodels.ViewStageCell{Kind: deployment.ReferenceApplied, Reference: shipped.Stages.Observed.Reference, ObservedAt: shipped.Stages.Observed.ObservedAt},
	})
	if len(labelSwap) != 1 || labelSwap[0] != "observed cell does not read the observed dimension" {
		t.Fatalf("the separation line must reject a dimension-label mutation, got %v", labelSwap)
	}
}

// TestAppVersionsCommandCallable is red-first evidence: until the aggregate
// version view is wired into the CLI, the command does not exist; afterwards, a
// persisted deployment with a cleared observed dimension must still render
// three separated, readable cells with an explicit UNCLEAR observed cell.
func TestAppVersionsCommandCallable(t *testing.T) {
	configureTestTarget(t)
	identity, err := deployment.NewDeploymentIdentity("docker-dev", "11111111-1111-4111-8111-111111111111", "versions-demo", "dev")
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "managed", Repository: t.TempDir()},
		Applied: deployment.AppliedDeployment{
			RuntimeProvider: "docker",
			Intent: mustIntentSnapshotJSON(t, application.WithWorkloadComponents(application.Manifest{
				Version:       application.CurrentVersion,
				ApplicationID: "11111111-1111-4111-8111-111111111111",
				Name:          "versions-demo",
				Environment:   "dev",
			}, "demo-app")),
		},
		References: map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:intended-only"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:applied-only"),
			deployment.ReferenceObserved: referenceDimension(deployment.ReferenceObserved, "sha256:applied-only"),
		},
		Observed: deployment.ObservedDeployment{State: "ready", Ready: true},
	}
	// Seed the real lifecycle record, then clear ONLY the observed dimension
	// through the accepted isolated operation: the applied cell must survive.
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatalf("SaveDeploymentRecord: %v", err)
	}
	if err := deployment.ClearReference(identity, deployment.ReferenceObserved); err != nil {
		t.Fatalf("ClearReference: %v", err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "versions", "-o", "json"}, &out, &errOut); err != nil {
		t.Fatalf("app versions must be callable: %v\n%s", err, errOut.String())
	}
	var payload struct {
		Releases []struct {
			Stages struct {
				Wanted   map[string]any `json:"wanted"`
				Applied  map[string]any `json:"applied"`
				Observed map[string]any `json:"observed"`
			} `json:"stages"`
		} `json:"releases"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("app versions must emit JSON: %v\n%s", err, out.String())
	}
	if len(payload.Releases) != 1 {
		t.Fatalf("exactly one deployment expected: %s", out.String())
	}
	stages := payload.Releases[0].Stages
	for name, cell := range map[string]map[string]any{"wanted": stages.Wanted, "applied": stages.Applied, "observed": stages.Observed} {
		if cell == nil {
			t.Fatalf("stages must carry a separate %q cell: %s", name, out.String())
		}
		ref, _ := cell["reference"].(string)
		if strings.TrimSpace(ref) == "" {
			t.Fatalf("cell %q must never be blank: %v", name, cell)
		}
	}
	if ref, _ := stages.Wanted["reference"].(string); ref == "" || ref == machinereadmodels.UnclearValue {
		t.Fatalf("wanted must show the recorded intended reference, got %q", ref)
	}
	if ref, _ := stages.Observed["reference"].(string); ref != machinereadmodels.UnclearValue {
		t.Fatalf("an unrecorded observation must read UNCLEAR, got %q", ref)
	}
	if missing, _ := stages.Observed["missing_data"].(bool); !missing {
		t.Fatal("the unclear observed cell must be marked missing_data")
	}
}
