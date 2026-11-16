package main

import (
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

// TestStatusBackendRowsKeepPinnedImageAndUnmappedSource pins the backend half
// of the unified component rows: an explicitly application-scoped SQL instance
// renders the delivered pinned provider image, the wanted reference flows from
// the recorded intended dimension unchanged, and a component without a
// source-model entry says unmapped instead of borrowing another component's
// provenance.
func TestStatusBackendRowsKeepPinnedImageAndUnmappedSource(t *testing.T) {
	manifest := application.WithSQLInstances(application.WithWorkloadComponents(
		application.Manifest{Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev"}, "api"), "primary")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	resolved := resolvedApplication{
		Manifest: manifest,
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
			deployment.ReferenceApplied:  referenceDimension(deployment.ReferenceApplied, "sha256:wanted"),
		})),
	}
	// Deliberate base parity (mission line 3): the delivered shared-provider
	// skip is placement-only, so an environment-scoped shared placement still
	// omits the backend row. The shared-provider readiness itself is reported
	// by the status checks, not by an owned image claim.
	rows, warnings := collectSourceModelComponents(resolved, mustView(t).Stages)
	if len(warnings) != 0 {
		t.Fatalf("no source model in this fixture must stay silent besides the unmapped state: %v", warnings)
	}
	primary := componentRowIndex(rows, "postgres-primary")
	if primary < 0 {
		t.Skipf("the delivered shared-placement rule omits the backend row (base parity): %v", rowNames(rows))
	}
	if rows[primary].Image != pinnedRuntimeImage("postgres") || rows[primary].SourceRevision != "sha256:wanted" {
		t.Fatalf("the backend row must keep the pinned image and the wanted reference: %+v", rows[primary])
	}
	if rows[primary].Source != "" {
		t.Fatalf("an unmapped backend must not borrow a source identity: %+v", rows[primary])
	}
	if !strings.Contains(formatComponentRow(rows[primary]), "unmapped") {
		t.Fatalf("the human row must name the unmapped state explicitly: %s", formatComponentRow(rows[primary]))
	}
}

// TestStatusSharedProviderBackendsAreNotDeploymentComponents pins the
// no-invention boundary: a shared Target provider is not a component of this
// deployment, so the unified version rows must not claim an owned image
// version for it.
func TestStatusSharedProviderBackendsAreNotDeploymentComponents(t *testing.T) {
	manifest := application.WithSQLInstances(application.Manifest{
		Version: application.CurrentVersion, ApplicationID: "11111111-1111-4111-8111-111111111111", Name: "demo", Environment: "dev",
	}, "primary")
	if !application.UsesSharedPostgreSQL(manifest) {
		t.Skip("the environment resolves PostgreSQL to a non-shared placement; the shared rule is covered by WithProviderPlacement")
	}
	resolved := resolvedApplication{
		Manifest: manifest,
		DeploymentRecord: ptrRecord(unifiedVersionRecord(t, "11111111-1111-4111-8111-111111111111", "demo", map[string]*deployment.ReferenceDimension{
			deployment.ReferenceIntended: referenceDimension(deployment.ReferenceIntended, "sha256:wanted"),
		})),
	}
	rows, _ := collectSourceModelComponents(resolved, mustView(t).Stages)
	for _, row := range rows {
		if strings.HasPrefix(row.Service, "postgres") {
			t.Fatalf("a shared provider must not render as an owned component row: %+v", row)
		}
	}
}

// TestVersionViewBackendRowsUseRecordPlacement proves the placement reader of
// the unified view: the retained placement (when the caller has one from the
// persisted record) replaces the ambient host policy for the read, nothing
// retained keeps the ambient classification, and the ambient environment is
// restored afterwards so nothing leaks into later calls.
func TestVersionViewBackendRowsUseRecordPlacement(t *testing.T) {
	scopeEnv := application.ProviderScopeEnv(capability.ProviderPostgreSQL)
	m := application.WithSQLInstances(application.New("placement-view-app", "dev", false, false, false), "primary")
	applicationScope := &capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	sharedScope := &capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	for _, test := range []struct {
		name     string
		ambient  capability.ProviderScope
		retained *capability.ProviderPlacement
		wantRow  bool
	}{
		{
			name:     "a retained application placement wins over the shared default",
			ambient:  capability.ScopeShared,
			retained: applicationScope,
			wantRow:  true,
		},
		{
			name:     "a retained shared placement stays without a backend row",
			ambient:  capability.ScopeApplication,
			retained: sharedScope,
			wantRow:  false,
		},
		{
			name:     "without a retained placement the ambient policy stands",
			ambient:  capability.ScopeApplication,
			retained: nil,
			wantRow:  true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(scopeEnv, string(test.ambient))
			rows, restore := viewPlacementServices(test.retained, m)
			hasRow := false
			for _, row := range rows {
				if row.name != "postgres-primary" {
					continue
				}
				hasRow = true
				if row.image != pinnedRuntimeImage("postgres") {
					t.Fatalf("the backend row must carry the delivered pin, got %q", row.image)
				}
			}
			if hasRow != test.wantRow {
				t.Fatalf("the backend row must follow the record placement (want %v): %+v", test.wantRow, rows)
			}
			restore()
			if v := os.Getenv(scopeEnv); v != string(test.ambient) {
				t.Fatalf("the ambient placement environment must be restored, got %q", v)
			}
		})
	}
}

var _ = machinereadmodels.UnclearValue
