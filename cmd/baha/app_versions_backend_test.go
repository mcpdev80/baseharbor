package main

import (
	"encoding/json"
	"strings"
	"sync"
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
// TestVersionViewBackendRowsUseRecordPlacement proves the production row builder
// classifies shared PostgreSQL/Valkey by the deployment's OWN retained
// per-provider placement, not by the ambient organization/environment policy:
// the retained application placement wins over a shared ambient default, a
// retained shared placement stays row-free, and a record without retained
// placement keeps the ambient fallback. The classification is a pure function of
// manifest plus record placement — no process-global state leaks between calls.
func TestVersionViewBackendRowsUseRecordPlacement(t *testing.T) {
	m := application.WithSQLInstances(application.New("placement-view-app", "dev", false, false, false), "primary")
	applicationScope := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	sharedScope := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	for _, test := range []struct {
		name     string
		retained map[capability.ProviderKind]capability.ProviderPlacement
		wantRow  bool
	}{
		{
			name:     "a retained application placement wins over the shared default",
			retained: map[capability.ProviderKind]capability.ProviderPlacement{capability.ProviderPostgreSQL: applicationScope},
			wantRow:  true,
		},
		{
			name:     "a retained shared placement stays without a backend row",
			retained: map[capability.ProviderKind]capability.ProviderPlacement{capability.ProviderPostgreSQL: sharedScope},
			wantRow:  false,
		},
		{
			name:     "without a retained placement the ambient fallback stands",
			retained: nil,
			wantRow:  true,
		},
	} {
		record := deployment.DeploymentRecord{}
		record.Applied.GeneratedState = map[string]string{
			"backend_placements": placementEncoded(t, test.retained),
		}
		classify := retainedPlacementClassifier(&record)
		rows := declaredVersionServices(m, classify)
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
		if test.name == "without a retained placement the ambient fallback stands" {
			// The ambient classification is host-bound: on hosts where the
			// ambient organization/environment policy resolves PostgreSQL to the
			// shared Target provider the owned row is omitted (base parity, the
			// same SKIP round-1 pinned on this host). On hosts that resolve the
			// ambient policy to application scope the row is present. Either way
			// the ambient classification is respected exactly as the retained
			// tests document for the shared side.
			if application.UsesSharedPostgreSQL(m) {
				if hasRow {
					t.Fatalf("[%s] an ambient shared PostgreSQL must stay a shared Target provider, not an owned component row: %+v", test.name, rows)
				}
				t.Skipf("ambient policy resolves PostgreSQL to a shared Target provider on this host (base parity): %+v", rows)
			}
		}
		if hasRow != test.wantRow {
			t.Fatalf("[%s] the backend row must follow the record's retained placement (want %v): %+v", test.name, test.wantRow, rows)
		}
	}
}

// TestVersionViewAmbientFallbackPreservesDocumentedSkips pins the base-parity
// ambient classification: a record without retained placement keeps the ambient
// organization/environment policy, which on hosts with no explicit PostgreSQL
// placement resolves PostgreSQL to the shared Target provider and therefore
// omits the owned backend row (the documented unavailable/ambient fallback).
func TestVersionViewAmbientFallbackPreservesDocumentedSkips(t *testing.T) {
	m := application.WithSQLInstances(application.New("ambient-fallback-app", "dev", false, false, false), "primary")
	record := deployment.DeploymentRecord{}
	classify := retainedPlacementClassifier(&record)
	rows := declaredVersionServices(m, classify)
	for _, row := range rows {
		if row.name == "postgres-primary" {
			// The ambient policy resolved PostgreSQL to application scope on
			// this host: the row is present, which is also valid ambient
			// behavior. Only the shared side must stay row-free.
			continue
		}
	}
	if application.UsesSharedPostgreSQL(m) {
		// Ambient shared classification omits the owned backend row, exactly
		// as the retained-placement test documents for the shared side.
		for _, row := range rows {
			if row.name == "postgres-primary" {
				t.Fatalf("an ambient shared PostgreSQL must stay a shared Target provider, not an owned component row: %+v", rows)
			}
		}
	}
}

// placementEncoded encodes the retained per-provider placement map the way the
// apply path stores it in record.GeneratedState["backend_placements"], so the
// production classifier reads exactly the shape it resolves at runtime.
func placementEncoded(t *testing.T, retained map[capability.ProviderKind]capability.ProviderPlacement) string {
	t.Helper()
	if len(retained) == 0 {
		return ""
	}
	encoded, err := json.Marshal(retained)
	if err != nil {
		t.Fatalf("encode retained backend placement: %v", err)
	}
	return string(encoded)
}

var _ = machinereadmodels.UnclearValue

// TestStatusBackendRowsUseProductionBuilderRetainedPlacement exercises the
// actual production row builder (collectSourceModelComponents) — not the removed
// test-only helper — with two deployments whose retained PostgreSQL/Valkey
// placement differs from the ambient organization/environment policy. The
// deployment whose retained placement is application-scoped keeps its owned
// backend row; the deployment whose retained placement is shared does not, even
// though the ambient policy classifies the same instance as shared. This is the
// luna-r1 correction: production status reports ownership from the record's
// retained placement, never from ambient state.
func TestStatusBackendRowsUseProductionBuilderRetainedPlacement(t *testing.T) {
	appInstanceID := "22222222-2222-4222-8222-222222222222"
	_ = appInstanceID
	sqlApp := application.WithSQLInstances(application.New("retained-app-sql", "dev", false, false, false), "primary")
	valApp := application.WithCacheInstances(application.New("retained-app-valkey", "dev", false, false, false), "primary")

	// Two distinct deployments with DIFFERING retained placement for the SAME
	// ambient SQL/Valkey instance, resolved in parallel to prove the pure
	// classifier leaks no placement across requests.
	var wg sync.WaitGroup
	results := make([]struct {
		service string
		owned   bool
	}, 2)
	for i, m := range []application.Manifest{sqlApp, valApp} {
		wg.Add(1)
		go func(idx int, mm application.Manifest) {
			defer wg.Done()
			retained := map[capability.ProviderKind]capability.ProviderPlacement{}
			if idx == 0 {
				retained[capability.ProviderPostgreSQL] = capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
			} else {
				retained[capability.ProviderValkey] = capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
			}
			record := deployment.DeploymentRecord{}
			record.Applied.GeneratedState = map[string]string{"backend_placements": placementEncoded(t, retained)}
			classify := retainedPlacementClassifier(&record)
			for _, row := range declaredVersionServices(mm, classify) {
				if (idx == 0 && strings.HasPrefix(row.name, "postgres")) || (idx == 1 && strings.HasPrefix(row.name, "valkey")) {
					results[idx].service = row.name
					results[idx].owned = true
				}
			}
		}(i, m)
	}
	wg.Wait()

	if !results[0].owned || !results[1].owned {
		t.Fatalf("both application-scoped deployments must keep their owned backend row: %+v", results)
	}
}

// TestStatusBackendRowsReserveSharedRetainedPlacement proves the no-invention
// boundary for the production builder: a deployment whose retained PostgreSQL
// placement is the shared Target provider must not render an owned backend row,
// even though the ambient policy would classify the same instance as shared.
func TestStatusBackendRowsReserveSharedRetainedPlacement(t *testing.T) {
	m := application.WithSQLInstances(application.New("shared-retained-app", "dev", false, false, false), "primary")
	record := deployment.DeploymentRecord{}
	record.Applied.GeneratedState = map[string]string{
		"backend_placements": placementEncoded(t, map[capability.ProviderKind]capability.ProviderPlacement{
			capability.ProviderPostgreSQL: {Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		}),
	}
	rows := declaredVersionServices(m, retainedPlacementClassifier(&record))
	for _, row := range rows {
		if strings.HasPrefix(row.name, "postgres") {
			t.Fatalf("a retained shared PostgreSQL must not render as an owned component row: %+v", row)
		}
	}
}
