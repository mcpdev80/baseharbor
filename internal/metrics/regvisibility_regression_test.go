package metrics

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// readSingleTargetGroup reads the single targetGroup payload from the named file
// in dir and returns it; fails the test on any mismatch.
func readSingleTargetGroup(t *testing.T, dir, name string) targetGroup {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var groups []targetGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		t.Fatalf("%s is not a targetGroup array: %v", name, err)
	}
	if len(groups) != 1 {
		t.Fatalf("%s has %d groups, want 1", name, len(groups))
	}
	return groups[0]
}

// listTargetJSONFiles returns the sorted *.json entries in dir, skipping
// directories.
func listTargetJSONFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	return names
}

// appTargetFileNameForID reconstructs the application target filename an ID maps
// to, mirroring appTargetFileName's grammar (application:metrics:<app>:<env>:<src>).
func appTargetFileNameForID(id string) string {
	const prefix = "application:metrics:"
	if !strings.HasPrefix(id, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(id, prefix), ":")
	if len(parts) != 3 {
		return ""
	}
	m := application.New(parts[0], parts[1], false, false, false)
	return applicationTargetPrefix + targetFileName(m, parts[2])
}

// TestRegVisibilityApplicationNamedProviderIsDisjoint pins finding #1: a valid
// application whose slug is "provider" must render to a file that is selected
// exactly once by the baseharbor-applications job and never by the
// baseharbor-providers job, and must survive a following provider sync.
func TestRegVisibilityApplicationNamedProviderIsDisjoint(t *testing.T) {
	dir := t.TempDir()
	appID := observability.ApplicationMetricsSourceID("provider", "dev", "api")
	appSource := observability.MetricsSource{
		ID:               appID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "provider",
		Network:          "baseharbor-metrics",
		Target:           "provider-api:8080",
		Path:             "/metrics",
		Scheme:           "http",
		Verification:     capability.ObservabilityVerifyBackend,
	}
	if err := syncProviderTargets(dir, []observability.MetricsSource{appSource}); err != nil {
		t.Fatalf("syncProviderTargets: %v", err)
	}

	appFileName := appTargetFileNameForID(appID)
	if !strings.HasPrefix(appFileName, applicationTargetPrefix) {
		t.Fatalf("app target filename %q is not in the application-- namespace", appFileName)
	}

	appCount := 0
	for _, name := range listTargetJSONFiles(t, dir) {
		group := readSingleTargetGroup(t, dir, name)
		switch {
		case name == appFileName:
			appCount++
			if group.Labels["job"] != "baseharbor-applications" {
				t.Fatalf("%s job label = %q, want baseharbor-applications", name, group.Labels["job"])
			}
			if group.Labels["baseharbor_application"] != "provider" ||
				group.Labels["baseharbor_environment"] != "dev" ||
				group.Labels["baseharbor_source"] != appID {
				t.Fatalf("%s missing required baseharbor labels: %#v", name, group.Labels)
			}
		case strings.HasPrefix(name, "provider--"), strings.HasPrefix(name, "provider-secure--"):
			for _, target := range group.Targets {
				t.Fatalf("application source %q selected by baseharbor-providers job in %q (not disjoint): %v", appID, name, target)
			}
		}
	}
	if appCount != 1 {
		t.Fatalf("application source %q selected %d times, want exactly once", appID, appCount)
	}

	// A following provider sync (no application sources) must not delete the
	// application target, but the application-target namespace is reconciled.
	if err := syncProviderTargets(dir, []observability.MetricsSource{}); err != nil {
		t.Fatalf("empty syncProviderTargets: %v", err)
	}
}

// TestRegVisibilityStaleApplicationTargetReconciledOnSync pins finding #2:
// reconciling the application-target namespace on every sync retains exactly the
// currently selected sources and deletes the rest, while foreign provider and
// runtime-target files owned by other writers are left untouched.
func TestRegVisibilityStaleApplicationTargetReconciledOnSync(t *testing.T) {
	dir := t.TempDir()

	// Files owned by other writers that must survive reconciliation: a
	// runtime-target file and a foreign (non-namespace) config file.
	runtimeTargetName := "0-runtime-target.json"
	runtimeTarget := []byte("[{\"targets\":[\"runtime:9091\"],\"labels\":{\"job\":\"runtime\"}}]\n")
	foreignConfigName := "provider-ca.pem"
	foreignConfig := []byte("-----BEGIN CERTIFICATE-----\nF0REIGN\n-----END CERTIFICATE-----\n")
	writeFile := func(name string, data []byte) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir targets: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeFile(runtimeTargetName, runtimeTarget)
	writeFile(foreignConfigName, foreignConfig)
	runtimeTargetDigest := fileDigest(t, filepath.Join(dir, runtimeTargetName))
	foreignConfigDigest := fileDigest(t, filepath.Join(dir, foreignConfigName))

	// First sync selects the stage source for the same application.
	stageID := observability.ApplicationMetricsSourceID("demo", "stage", "api")
	stageSource := observability.MetricsSource{
		ID:               stageID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "baseharbor-metrics",
		Target:           "demo-stage:8080",
		Path:             "/metrics",
		Scheme:           "http",
		Verification:     capability.ObservabilityVerifyBackend,
	}
	if err := syncProviderTargets(dir, []observability.MetricsSource{stageSource}); err != nil {
		t.Fatalf("syncProviderTargets (stage): %v", err)
	}
	stageFileName := appTargetFileNameForID(stageID)
	if _, err := os.Stat(filepath.Join(dir, stageFileName)); err != nil {
		t.Fatalf("stage application target %q not written: %v", stageFileName, err)
	}
	stageData := mustRead(t, filepath.Join(dir, stageFileName))
	if !strings.Contains(string(stageData), "stage") {
		t.Fatalf("stage target does not reference the stage endpoint: %s", stageData)
	}

	// Now select only the dev source. The stage file (and its endpoint) must be
	// gone; the dev file remains; foreign files are unchanged.
	devID := observability.ApplicationMetricsSourceID("demo", "dev", "api")
	devSource := observability.MetricsSource{
		ID:               devID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "baseharbor-metrics",
		Target:           "demo-dev:8080",
		Path:             "/metrics",
		Scheme:           "http",
		Verification:     capability.ObservabilityVerifyBackend,
	}
	if err := syncProviderTargets(dir, []observability.MetricsSource{devSource}); err != nil {
		t.Fatalf("syncProviderTargets (dev): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, stageFileName)); !os.IsNotExist(err) {
		t.Fatalf("stale stage application target %q survived dev-only sync (err=%v)", stageFileName, err)
	}
	devData := mustRead(t, filepath.Join(dir, appTargetFileNameForID(devID)))
	if strings.Contains(string(devData), "stage") {
		t.Fatalf("dev target references the stage endpoint: %s", devData)
	}
	if d := fileDigest(t, filepath.Join(dir, runtimeTargetName)); d != runtimeTargetDigest {
		t.Fatalf("runtime target file was modified by reconciliation")
	}
	if d := fileDigest(t, filepath.Join(dir, foreignConfigName)); d != foreignConfigDigest {
		t.Fatalf("foreign config file was modified by reconciliation")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("digest %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}
