package identityprovider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// TestRegisterKeycloakSignalsPreservesLegacySourceForOwner is the end-to-end
// production-path test card 0428-168 review P1/P4 demands: a legacy appA
// "keycloak-metrics" source of record, a DIFFERENT Keycloak project/appB
// registering through the real registerKeycloakSignals path, with appB's
// metrics both ENABLED and DISABLED, and in both cases appA's source stays
// observable - asserted BEFORE and AFTER the migration - under its derived
// identity with its own owner verbatim, and remains removable by that
// identity.
func TestRegisterKeycloakSignalsPreservesLegacySourceForOwner(t *testing.T) {
	for _, appBMetricsEnabled := range []bool{true, false} {
		name := "appB-metrics-enabled"
		if !appBMetricsEnabled {
			name = "appB-metrics-disabled"
		}
		t.Run(name, func(t *testing.T) {
			dir := keycloakTestDataDir(t)
			t.Setenv("BASEHARBOR_STATE_DIR", dir)
			// The Keycloak placement is app-scoped so the source identities
			// carry their project grammar (the listing contract resolves the
			// environment from the source's own data, route 2).
			t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), string(capability.ScopeApplication))

			// 1. appA's legacy source of record, written exactly as appA's
			// registerKeycloakSignals did BEFORE the ID scoping: the unscoped
			// literal the old runtime wrote under appA's placement. appA
			// deployed before the scoping, so it left its placement state
			// directory, which the migration's owner->project map reads.
			appA := keycloakSignalTestApp(t, "alpha", true)
			writeKeycloakStateDirs(t, dir, appA)
			if err := observability.Update(observability.MetricsSource{
				ID:               observability.LegacyKeycloakMetricsID,
				Provider:         capability.ProviderKeycloak,
				Class:            observability.SourceApplicationProvider,
				Scope:            capability.ScopeApplication,
				OwnerApplication: appA.Name,
				Network:          "baseharbor-alpha-dev",
				Target:           "keycloak:9000",
				Path:             "/metrics",
				Scheme:           "http",
			}); err != nil {
				t.Fatalf("write legacy keycloak source of record: %v", err)
			}
			// P4: the source EXISTS in the registry before anything else runs,
			// yet is NOT listable for its owner under the unscoped literal -
			// the route-2 environment filter fails closed on a project-less
			// ID. That is the corpse the migration makes observable.
			if ok, err := observability.RegistryHasSignal(observability.LegacyKeycloakMetricsID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
				t.Fatal(err)
			} else if !ok {
				t.Fatal("legacy source not present after its own write")
			}
			preList, err := observability.ListMetrics(appScopedPlacement(), []string{appA.Name}, true, true, appA.Environment)
			if err != nil {
				t.Fatalf("pre-migration ListMetrics: %v", err)
			}
			for _, source := range preList {
				if source.ID == observability.LegacyKeycloakMetricsID {
					t.Fatal("pre-migration witness broken: the unscoped literal must not be listable (route 2 fails closed)")
				}
			}

			// 2. The NEW production path runs for a DIFFERENT Keycloak project
			// (appB, application "bravo"). appB's registration must neither
			// drop nor overwrite appA's source of record.
			appB := keycloakSignalTestApp(t, "bravo", appBMetricsEnabled)
			bravoProject := bhruntime.ApplicationProjectName("target", appB.Name+"-identity", appB.Environment)
			rB := &localKeycloakRealization{
				app:       appB,
				namespace: "target",
				dataDir:   dir,
				files:     KeycloakFiles{Project: bravoProject, ConsumerNetwork: "baseharbor-bravo-dev"},
			}
			if err := rB.registerKeycloakSignals(); err != nil {
				t.Fatalf("register new keycloak signals for appB: %v", err)
			}

			// 3. The legacy literal is gone (re-booked); appA's source stands
			// under its derived identity - its own canonical application
			// project with the legacy signal name - owner verbatim, disjoint
			// from appB's registration identity.
			migratedID := "keycloak:" + bhruntime.ApplicationProjectName("target", appA.Name, appA.Environment) + ":" + observability.LegacyKeycloakMetricsID
			if ok, err := observability.RegistryHasSignal(observability.LegacyKeycloakMetricsID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
				t.Fatal(err)
			} else if ok {
				t.Fatal("legacy literal still present after the production path -> not migrated")
			}
			if ok, err := observability.RegistryHasSource(migratedID, capability.ProviderKeycloak, observability.SignalMetrics, appA.Name); err != nil {
				t.Fatal(err)
			} else if !ok {
				t.Fatalf("appA's migrated source vanished or lost its owner: %q", migratedID)
			}
			if appBMetricsEnabled {
				// appB's registration stands beside appA's migrated entry,
				// distinct identities.
				newID := keycloakMetricsSignalID(bravoProject)
				if newID == migratedID {
					t.Fatalf("appA's migrated identity %q collapses onto appB's registration", newID)
				}
				if ok, err := observability.RegistryHasSignal(newID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
					t.Fatal(err)
				} else if !ok {
					t.Fatalf("appB's scoping-ID source %q not present", newID)
				}
			}

			// 4. appA's source is observable for its owner through the list
			// contract - the derived identity resolves an environment from its
			// own data (route 2), which the unscoped literal never could.
			list, err := observability.ListMetrics(appScopedPlacement(), []string{appA.Name, appB.Name}, true, true, appB.Environment)
			if err != nil {
				t.Fatalf("ListMetrics for the registering app: %v", err)
			}
			found := false
			for _, source := range list {
				if source.ID == migratedID && source.Provider == capability.ProviderKeycloak && source.OwnerApplication == appA.Name {
					found = true
				}
			}
			if !found {
				t.Fatalf("the migrated keycloak source %q (owner appA verbatim) not visible through ListMetrics: %#v", migratedID, list)
			}
			if err := observability.Remove(migratedID); err != nil {
				t.Fatalf("Remove(appA identity %q): %v", migratedID, err)
			}
			if ok, err := observability.RegistryHasSignal(migratedID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
				t.Fatal(err)
			} else if ok {
				t.Fatal("appA's source still present after removing its project identity")
			}
		})
	}
}

func appScopedPlacement() capability.ProviderPlacement {
	return capability.ProviderPlacement{Scope: capability.ScopeApplication}
}

// writeKeycloakStateDirs creates the on-disk placement state (providers/
// keycloak/applications/<name>/<environment>) a deployed application
// placement leaves behind, so ListKeycloakOwnerProjects resolves the owner's
// canonical project.
func writeKeycloakStateDirs(t *testing.T, dir string, app application.Manifest) {
	t.Helper()
	state := filepath.Join(dir, "providers", "keycloak", "applications", app.Name, app.Environment)
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatalf("state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(state, "compose.yaml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("compose: %v", err)
	}
}

// keycloakTestDataDir is a shared per-run temp dir for the realization state
// and the observability registry, so the legacy write, the migration and the
// listing all address ONE state root.
func keycloakTestDataDir(t *testing.T) string {
	t.Helper()
	if dir := keycloakTestDataDirCache[t.Name()]; dir != "" {
		return dir
	}
	dir := t.TempDir()
	keycloakTestDataDirCache[t.Name()] = dir
	return dir
}

var keycloakTestDataDirCache = map[string]string{}

// TestRegisterKeycloakSignalsSecondRegistrationIsNoMutationOnFirst proves P2
// end-to-end on the production path: after appA's legacy record has been
// migrated and appB registered, a SECOND registration by the same appB leaves
// the registry file byte-identical (no rewrite, no second copy of anything).
func TestRegisterKeycloakSignalsSecondRegistrationIsNoMutationOnFirst(t *testing.T) {
	dir := keycloakTestDataDir(t)
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), string(capability.ScopeApplication))
	appA := keycloakSignalTestApp(t, "alpha", true)
	writeKeycloakStateDirs(t, dir, appA)
	if err := observability.Update(observability.MetricsSource{
		ID:               observability.LegacyKeycloakMetricsID,
		Provider:         capability.ProviderKeycloak,
		Class:            observability.SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: appA.Name,
		Network:          "baseharbor-alpha-dev",
		Target:           "keycloak:9000",
		Path:             "/metrics",
		Scheme:           "http",
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}
	appB := keycloakSignalTestApp(t, "bravo", true)
	bravoProject := bhruntime.ApplicationProjectName("target", appB.Name+"-identity", appB.Environment)
	rB := &localKeycloakRealization{
		app:       appB,
		namespace: "target",
		dataDir:   dir,
		files:     KeycloakFiles{Project: bravoProject, ConsumerNetwork: "baseharbor-bravo-dev"},
	}
	if err := rB.registerKeycloakSignals(); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	path := filepath.Join(dir, "observability", "signals.json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if err := rB.registerKeycloakSignals(); err != nil {
		t.Fatalf("second registration: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read registry after second registration: %v", err)
	}
	if string(first) != string(after) {
		t.Fatalf("an idempotent re-registration rewrote the registry:\nfirst=%s\nafter=%s", first, after)
	}
}

// TestRegisterKeycloakSignalsAbortsOnPartialOwnerStateScan closes the r2
// review blocker: when the Target's Keycloak owner-state scan PARTIALLY fails
// — an application directory that cannot be read, or one that carries no
// environment entry — registration must return an error BEFORE the registry
// mutation, leaving the legacy record and all registry bytes untouched. A
// foreign owner's record may never be re-booked under the caller's project by
// guesswork: that identity's grammar lacks the owner anchor, so the record
// would fail closed in listing and be unobservable again.
func TestRegisterKeycloakSignalsAbortsOnPartialOwnerStateScan(t *testing.T) {
	for _, mode := range []string{"unreadable-owner-dir", "owner-dir-without-environment"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("BASEHARBOR_STATE_DIR", dir)
			t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), string(capability.ScopeApplication))
			appA := keycloakSignalTestApp(t, "alpha", true)
			writeKeycloakStateDirs(t, dir, appA)
			if err := observability.Update(observability.MetricsSource{
				ID:               observability.LegacyKeycloakMetricsID,
				Provider:         capability.ProviderKeycloak,
				Class:            observability.SourceApplicationProvider,
				Scope:            capability.ScopeApplication,
				OwnerApplication: appA.Name,
				Network:          "baseharbor-alpha-dev",
				Target:           "keycloak:9000",
				Path:             "/metrics",
				Scheme:           "http",
			}); err != nil {
				t.Fatalf("write legacy source: %v", err)
			}
			// A second owner's application directory makes the scan PARTIAL.
			foreign := filepath.Join(dir, "providers", "keycloak", "applications", "charlie")
			var before []byte
			switch mode {
			case "unreadable-owner-dir":
				if err := os.MkdirAll(foreign, 0o000); err != nil {
					t.Fatalf("unreadable owner dir: %v", err)
				}
				if _, err := os.ReadDir(foreign); err == nil {
					t.Skip("the current user may read 0o000 directories (root): no deterministic unreadable seam")
				}
			case "owner-dir-without-environment":
				if err := os.MkdirAll(foreign, 0o700); err != nil {
					t.Fatalf("empty owner dir: %v", err)
				}
			}
			t.Cleanup(func() { _ = os.Chmod(foreign, 0o700) })

			appB := keycloakSignalTestApp(t, "bravo", true)
			bravoProject := bhruntime.ApplicationProjectName("target", appB.Name+"-identity", appB.Environment)
			rB := &localKeycloakRealization{
				app:       appB,
				namespace: "target",
				dataDir:   dir,
				files:     KeycloakFiles{Project: bravoProject, ConsumerNetwork: "baseharbor-bravo-dev"},
			}
			path := filepath.Join(dir, "observability", "signals.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read registry before: %v", err)
			}
			if err := rB.registerKeycloakSignals(); err == nil {
				t.Fatal("a partial owner-state scan must abort registration, not guess")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read registry after: %v", err)
			}
			if string(before) != string(after) {
				t.Fatalf("the aborted registration changed registry bytes:\nbefore=%s\nafter =%s", before, after)
			}
			if ok, err := observability.RegistryHasSignal(observability.LegacyKeycloakMetricsID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
				t.Fatal(err)
			} else if !ok {
				t.Fatal("the legacy record must stand verbatim after the abort")
			}
		})
	}
}

// TestListKeycloakOwnerProjectsReportsPartialScan proves the scan surface
// itself: no state directory is NO error (nil, nil — the legacy migration
// case), a complete scan maps owners, and a per-owner read failure or an
// owner directory without any environment entry returns an error instead of
// silently omitting the owner.
func TestListKeycloakOwnerProjectsReportsPartialScan(t *testing.T) {
	dir := t.TempDir()
	// No state at all: nil, nil.
	if got, err := ListKeycloakOwnerProjects(dir, "target"); err != nil || got != nil {
		t.Fatalf("absent state: got=%v err=%v, want nil,nil", got, err)
	}
	// Complete scan.
	appA := keycloakSignalTestApp(t, "alpha", true)
	writeKeycloakStateDirs(t, dir, appA)
	got, err := ListKeycloakOwnerProjects(dir, "target")
	if err != nil {
		t.Fatalf("complete scan: %v", err)
	}
	if want := bhruntime.ApplicationProjectName("target", "alpha-identity", appA.Environment); got["alpha"] != want {
		t.Fatalf("complete scan alpha=%q, want %q", got["alpha"], want)
	}
	// Partial scan: an owner directory with no environment entry errors.
	if err := os.MkdirAll(filepath.Join(dir, "providers", "keycloak", "applications", "charlie"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ListKeycloakOwnerProjects(dir, "target"); err == nil {
		t.Fatal("an owner directory without an environment entry must error, not be omitted")
	}
}
