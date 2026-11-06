package machinereadmodels

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestReleaseViewExampleShapeKeepsEveryCellReadable guards the shipped read
// model example: the documented shape must always render a readable reference
// (never a blank cell) in each of the three dimensions, and it must show one
// explicitly unclear cell.
func TestReleaseViewExampleShapeKeepsEveryCellReadable(t *testing.T) {
	var view VersionView
	found := false
	for _, example := range Examples() {
		if example.Operation == "app.versions" {
			view = example.Value.(ApplicationVersionsResult).Releases[0]
			found = true
			break
		}
	}
	if !found {
		t.Fatal("read-model examples must include the app.versions shape")
	}
	for _, cell := range []*ViewStageCell{view.Stages.Wanted, view.Stages.Applied, view.Stages.Observed} {
		if cell == nil {
			t.Fatal("the version view example must carry all three separated cells")
		}
		if cell.Reference == "" {
			t.Fatalf("cell %q must never render blank", cell.Kind)
		}
	}
	if cell := view.Stages.Observed; cell.MissingData || cell.Reference == "" || cell.Kind != deployment.ReferenceObserved {
		t.Fatalf("the example's observed cell must carry a readable reference bound to its dimension: %+v", cell)
	}

	// Deriving from a real record with only the applied dimension recorded must
	// render the other two as explicit unclear cells, not blanks.
	derived := ReleaseViewFromRecord(deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		References: map[string]*deployment.ReferenceDimension{
			deployment.ReferenceApplied: {Kind: deployment.ReferenceApplied, Release: "sha256:applied"},
		},
	})
	if derived.Stages.Wanted.Reference != UnclearValue || derived.Stages.Observed.Reference != UnclearValue {
		t.Fatalf("missing dimensions must read UNCLEAR: %+v", derived.Stages)
	}
	if derived.Stages.Applied.Reference != "sha256:applied" {
		t.Fatalf("recorded dimension must survive: %+v", derived.Stages.Applied)
	}
}
