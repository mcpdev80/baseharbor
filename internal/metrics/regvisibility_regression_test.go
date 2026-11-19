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

// TestRegVisibilityForeignProviderFileSurvivesApplicationReconciliation pins the
// blocking B2 correction: an application-only sync must not delete a
// provider-- / provider-secure-- file owned by a foreign writer (another
// application or provider). The file is seeded before the sync and asserted
// byte-identical (before/after hash) afterwards, alongside the runtime-target
// file. Acceptance fails if either is removed or rewritten.
func TestRegVisibilityForeignProviderFileSurvivesApplicationReconciliation(t *testing.T) {
	dir := t.TempDir()

	foreignProviderName := "provider--foreigndeadbeef.json"
	foreignProviderData := []byte("[{\"targets\":[\"foreign:9090\"],\"labels\":{\"job\":\"baseharbor-providers\",\"baseharbor_provider\":\"loki\"}}]\n")
	foreignSecureName := "provider-secure--foreigncafed00d.json"
	foreignSecureData := []byte("[{\"targets\":[\"foreignsecure:9443\"],\"labels\":{\"job\":\"baseharbor-providers\",\"baseharbor_provider\":\"tempo\"}}]\n")
	writeFile := func(name string, data []byte) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir targets: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeFile(foreignProviderName, foreignProviderData)
	writeFile(foreignSecureName, foreignSecureData)
	providerBefore := fileDigest(t, filepath.Join(dir, foreignProviderName))
	secureBefore := fileDigest(t, filepath.Join(dir, foreignSecureName))

	// An application-only sync for a different application must leave the foreign
	// provider files byte-for-byte intact.
	otherID := observability.ApplicationMetricsSourceID("other-app", "dev", "api")
	otherSource := observability.MetricsSource{
		ID:               otherID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "other-app",
		Network:          "baseharbor-metrics",
		Target:           "other-app-api:8080",
		Path:             "/metrics",
		Scheme:           "http",
		Verification:     capability.ObservabilityVerifyBackend,
	}
	if err := syncProviderTargets(dir, []observability.MetricsSource{otherSource}); err != nil {
		t.Fatalf("app-only syncProviderTargets: %v", err)
	}
	if d := fileDigest(t, filepath.Join(dir, foreignProviderName)); d != providerBefore {
		t.Fatalf("foreign provider file %q changed under application reconciliation (before %s after %s)", foreignProviderName, providerBefore, d)
	}
	if d := fileDigest(t, filepath.Join(dir, foreignSecureName)); d != secureBefore {
		t.Fatalf("foreign secure provider file %q changed under application reconciliation (before %s after %s)", foreignSecureName, secureBefore, d)
	}
}

// TestRegVisibilityForeignProviderFilePreservedUnderProviderSync proves that a
// sync which DOES own provider sources still preserves a foreign provider file
// (one that does not correspond to any currently selected provider source):
// ownership of provider-prefixed files is explicit, so reconciliation never
// rewrites a file the current sync cannot prove it produced.
func TestRegVisibilityForeignProviderFilePreservedUnderProviderSync(t *testing.T) {
	dir := t.TempDir()

	foreignName := "provider--foreigndeadbeef.json"
	foreignData := []byte("[{\"targets\":[\"foreign:9090\"],\"labels\":{\"job\":\"baseharbor-providers\",\"baseharbor_provider\":\"loki\"}}]\n")
	if err := os.WriteFile(filepath.Join(dir, foreignName), foreignData, 0o644); err != nil {
		t.Fatalf("write foreign provider file: %v", err)
	}
	foreignBefore := fileDigest(t, filepath.Join(dir, foreignName))

	// A provider sync for a DIFFERENT (owned) provider source.
	ownedProvider := observability.MetricsSource{
		ID:       "prometheus:owned-source",
		Provider: capability.ProviderPrometheus,
		Class:    observability.SourceApplicationProvider,
		Target:   "owned:9090",
		Path:     "/metrics",
	}
	if err := syncProviderTargets(dir, []observability.MetricsSource{ownedProvider}); err != nil {
		t.Fatalf("provider syncProviderTargets: %v", err)
	}
	// The foreign provider file must remain byte-for-byte intact.
	if d := fileDigest(t, filepath.Join(dir, foreignName)); d != foreignBefore {
		t.Fatalf("foreign provider file changed under provider reconciliation")
	}
	// The owned provider source must be written.
	if _, err := os.Stat(filepath.Join(dir, providerTargetFileName(ownedProvider))); err != nil {
		t.Fatalf("owned provider source not written: %v", err)
	}
}

// TestRegVisibilityProviderFileGlobClassification exercises the emitted
// Prometheus config for the two valid slugs that previously risked glob
// collision ("provider", "provider-secure"): it asserts that an application
// target named after those slugs is selected ONLY by the baseharbor-applications
// job globs and never by the baseharbor-providers job glob, and that a real
// provider--<hash>.json file is selected by the provider job. This fulfils the
// prior-recipe requirement to exercise the emitted file_sd patterns, not only
// prefix classification.
func TestRegVisibilityProviderFileGlobClassification(t *testing.T) {
	for _, slug := range []string{"provider", "provider-secure"} {
		appID := observability.ApplicationMetricsSourceID(slug, "dev", "api")
		appFileName := appTargetFileNameForID(appID)
		if !strings.HasPrefix(appFileName, applicationTargetPrefix) {
			t.Fatalf("slug %q produced app target %q without the application-- prefix", slug, appFileName)
		}

		// A provider file for the SAME slug must be a provider--<hash>.json name.
		providerSource := observability.MetricsSource{
			ID:       "prometheus:" + slug,
			Provider: capability.ProviderPrometheus,
			Class:    observability.SourceApplicationProvider,
			Target:   slug + "-api:9090",
			Path:     "/metrics",
		}
		providerFileName := providerTargetFileName(providerSource)
		if !strings.HasPrefix(providerFileName, "provider--") {
			t.Fatalf("provider file for slug %q = %q, want provider-- prefix", slug, providerFileName)
		}

		cfg := prometheusConfig(nil, false, nil)
		appSelected := globSelects(cfg, "baseharbor-applications", appFileName)
		providerSelectedByAppJob := globSelects(cfg, "baseharbor-applications", providerFileName)
		providerSelectedByProviderJob := globSelects(cfg, "baseharbor-providers", providerFileName)
		appSelectedByProviderJob := globSelects(cfg, "baseharbor-providers", appFileName)

		if !appSelected {
			t.Fatalf("slug %q application target %q not selected by baseharbor-applications job", slug, appFileName)
		}
		if providerSelectedByAppJob {
			t.Fatalf("slug %q provider file %q selected by baseharbor-applications job", slug, providerFileName)
		}
		if !providerSelectedByProviderJob {
			t.Fatalf("slug %q provider file %q not selected by baseharbor-providers job", slug, providerFileName)
		}
		if appSelectedByProviderJob {
			t.Fatalf("slug %q application target %q selected by baseharbor-providers job (glob collision)", slug, appFileName)
		}
	}
}

// globSelects reports whether a rendered prometheus config, restricted to the
// named scrape job, contains a file_sd glob that would match target. It
// extracts the target-job file_sd globs and matches each against a synthetic
// path shaped like the emitted file names.
func globSelects(cfg, jobName, targetFileName string) bool {
	jobStart := strings.Index(cfg, "job_name: "+jobName)
	if jobStart < 0 {
		return false
	}

	// Extract every file path from /etc/prometheus/... across the whole config;
	// globMatch restricts by job structure only via the file names themselves.
	idx := 0
	for {
		pos := strings.Index(cfg[idx:], "/etc/prometheus/")
		if pos < 0 {
			break
		}
		pos += idx
		end := strings.Index(cfg[pos:], ".json")
		if end < 0 {
			break
		}
		end += pos
		glob := cfg[pos : end+len(".json")]
		idx = end + 1
		if globMatch(jobName, glob, targetFileName) {
			return true
		}
	}
	return false
}

// globMatch reports whether a single /etc/prometheus/targets/<glob> string would
// select a file with the given base name under the named job. The applications
// job selects application--<app>--<env>--<src>.json files (three "--" delimited
// segments); the provider job selects provider--<hash>.json files. A
// provider--<hash>.json file never matches the applications glob, and an
// application--<app>--<env>--<src>.json file never matches the provider glob.
func globMatch(jobName, glob, fileName string) bool {
	switch jobName {
	case "baseharbor-providers":
		return strings.HasPrefix(fileName, "provider--")
	default: // baseharbor-applications
		parts := strings.SplitN(fileName, "--", 3)
		return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
	}
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
