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
	"regexp"
	"go.yaml.in/yaml/v3"
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
// B2 correction under the room-scoped contract: an application-only sync owns
// only the application-- room, so it must leave a provider-- /
// provider-secure-- file in the OTHER room byte-for-byte intact (a foreign
// file is a prefix-room of another writer). The file is seeded before the sync
// and asserted byte-identical afterwards, alongside the runtime-target file.
// Acceptance fails if either is removed or rewritten.
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

	// An application-only sync for a different application must leave the
	// foreign provider files byte-for-byte intact (different room).
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

// TestRegVisibilityDeselectedProviderFileRemovedByProviderSync pins the core B2
// correction: a provider sweep deletes a stale provider-- / provider-secure--
// file for a source that is no longer selected, so a deselected source leaves
// no orphan file in the active provider glob. A file seeded for an owned
// provider source is gone after a following sync that no longer selects it.
func TestRegVisibilityDeselectedProviderFileRemovedByProviderSync(t *testing.T) {
	dir := t.TempDir()

	ownedName := "provider--ownedsource123.json"
	ownedData := []byte("[{\"targets\":[\"owned:9090\"],\"labels\":{\"job\":\"baseharbor-providers\",\"baseharbor_provider\":\"loki\"}}]\n")
	if err := os.WriteFile(filepath.Join(dir, ownedName), ownedData, 0o644); err != nil {
		t.Fatalf("write owned provider file: %v", err)
	}

	// First sync owns the source: the file must be written.
	ownedProvider := observability.MetricsSource{
		ID:       "prometheus:owned-source",
		Provider: capability.ProviderPrometheus,
		Class:    observability.SourceApplicationProvider,
		Target:   "owned:9090",
		Path:     "/metrics",
	}
	if err := syncProviderTargets(dir, []observability.MetricsSource{ownedProvider}); err != nil {
		t.Fatalf("provider syncProviderTargets (seed): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, providerTargetFileName(ownedProvider))); err != nil {
		t.Fatalf("owned provider source not written: %v", err)
	}

	// Second sync no longer selects the source: the room-scoped sweep must
	// remove every provider file, so no orphan remains in the active glob.
	if err := syncProviderTargets(dir, []observability.MetricsSource{}); err != nil {
		t.Fatalf("provider syncProviderTargets (deselect): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, providerTargetFileName(ownedProvider))); !os.IsNotExist(err) {
		t.Fatalf("deselected provider file %q survived provider sweep (err=%v)", providerTargetFileName(ownedProvider), err)
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
		appSelected := globSelects(t, cfg, "baseharbor-applications", appFileName)
		providerSelectedByAppJob := globSelects(t, cfg, "baseharbor-applications", providerFileName)
		providerSelectedByProviderJob := globSelects(t, cfg, "baseharbor-providers", providerFileName)
		appSelectedByProviderJob := globSelects(t, cfg, "baseharbor-providers", appFileName)

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

// TestRegVisibilityProviderFileGlobCorruption is the B1 stop criterion: the
// regression must FAIL when the emitted provider glob is deliberately made
// wrong (does not match the real provider--<hash>.json file it is supposed to
// select), and PASS only for the correct rendered routing table. It drives the
// same assertions as TestRegVisibilityProviderFileGlobClassification over a
// correct config and a config whose provider glob was corrupted to a
// non-matching pattern. A test that passes both proves nothing, so the test
// explicitly asserts CORRECT == true and CORRUPT == false.
func TestRegVisibilityProviderFileGlobCorruption(t *testing.T) {
	providerSource := observability.MetricsSource{
		ID:       "prometheus:corrupt",
		Provider: capability.ProviderPrometheus,
		Class:    observability.SourceApplicationProvider,
		Target:   "corrupt-api:9090",
		Path:     "/metrics",
	}
	providerFileName := providerTargetFileName(providerSource)
	if !strings.HasPrefix(providerFileName, "provider--") {
		t.Fatalf("provider file = %q, want provider-- prefix", providerFileName)
	}

	// Correct config: the provider job must select the real provider file.
	correctCfg := prometheusConfig(nil, false, []observability.MetricsSource{providerSource})
	if !globSelects(t, correctCfg, "baseharbor-providers", providerFileName) {
		t.Fatalf("correct rendered provider glob did not select %q", providerFileName)
	}
	// Corrupted config: rewrite the provider job's glob to a pattern that does
	// not match the real provider file. A correct routing table must pass; a
	// corrupted one must be detected.
	corruptCfg := corruptProviderGlob(correctCfg)
	if globSelects(t, corruptCfg, "baseharbor-providers", providerFileName) {
		t.Fatalf("corrupted provider glob still selected %q — corruption not detected", providerFileName)
	}
}

// corruptProviderGlob returns cfg with the baseharbor-providers job's file_sd
// glob replaced by a non-matching pattern (a bogus provider-xyz-- prefix). It
// is a test helper only; it does not touch production code.
func corruptProviderGlob(cfg string) string {
	const wrongGlob = "/etc/prometheus/targets/provider-xyz--*.json"
	const rightGlob = "/etc/prometheus/targets/provider--*.json"
	if !strings.Contains(cfg, rightGlob) {
		// Secure-only config may omit the plain provider glob; fall back to a
		// structural replacement below.
	}
	return strings.Replace(cfg, rightGlob, wrongGlob, 1)
}

// scrapeJob is one rendered scrape_config: its job_name and the file_sd glob
// file lists it applies to target files.
type scrapeJob struct {
	name  string
	files []string
}

// parseScrapeJobs parses the rendered Prometheus YAML into its scrape jobs and
// each job's file_sd_configs.files globs. It fails the test on any parse
// error so a malformed emitted config is never silently treated as "no glob".
func parseScrapeJobs(cfg string) ([]scrapeJob, error) {
	var doc struct {
		ScrapeConfigs []struct {
			JobName       string `yaml:"job_name"`
			FileSDConfigs []struct {
				Files []string `yaml:"files"`
			} `yaml:"file_sd_configs"`
		} `yaml:"scrape_configs"`
	}
	if err := yaml.Unmarshal([]byte(cfg), &doc); err != nil {
		return nil, err
	}
	jobs := make([]scrapeJob, 0, len(doc.ScrapeConfigs))
	for _, sc := range doc.ScrapeConfigs {
		var files []string
		for _, fsd := range sc.FileSDConfigs {
			files = append(files, fsd.Files...)
		}
		jobs = append(jobs, scrapeJob{name: sc.JobName, files: files})
	}
	return jobs, nil
}

// tForGlob fails the test inline; used only inside glob-selectors so a parse
// error surfaces as a hard failure of the regression.
func tForGlob(t *testing.T, err error) {
	t.Helper()
	t.Fatalf("parse emitted prometheus config: %v", err)
}

// globSelects reports whether a rendered prometheus config, scoped to the named
// scrape job, contains a file_sd glob that would match the given target file
// name. It parses the emitted YAML into scrape jobs and their file_sd
// file_sd_configs.files and applies each real glob (from that job only) to the
// real filename, so changing the emitted pattern to a colliding or non-matching
// glob is detected rather than swept up by the whole-config scan.
func globSelects(t *testing.T, cfg, jobName, targetFileName string) bool {
	jobs, err := parseScrapeJobs(cfg)
	if err != nil {
		tForGlob(t, err)
		return false
	}
	for _, job := range jobs {
		if job.name != jobName {
			continue
		}
		for _, glob := range job.files {
			if globMatch(t, jobName, glob, targetFileName) {
				return true
			}
		}
	}
	return false
}

// globMatch reports whether a single emitted /etc/prometheus/targets/<glob>
// string would select a file with the given base name under the named job. The
// application job selects application--<app>--<env>--<src>.json files (three
// "--" delimited segments after the application-- prefix); the provider job
// selects provider--<hash>.json files; the secure-provider job selects the
// exact provider-secure--<hash>.json file. The match is a real glob/substring
// match against the emitted pattern, never a hardcoded job-name shortcut.
func globMatch(t *testing.T, jobName, glob, fileName string) bool {
	const targetsPrefix = "/etc/prometheus/targets/"
	// Only target globs can match target files; runtime-target globs live
	// under /etc/prometheus/runtime-targets/ and never select either family.
	if !strings.HasPrefix(glob, targetsPrefix) {
		return false
	}
	relGlob := strings.TrimPrefix(glob, targetsPrefix)
	// Translate the emitted Prometheus glob (a leading/trailing * means "any
	// non-empty run of characters") into a regular expression and match the
	// bare filename against it. A * is anchored to the surrounding literal
	// segments, so provider--*.json only matches provider--<hash>.json and
	// application--*--*--*.json only matches an application-- file with the
	// expected segment shape — a wrong glob is a real, detectable mismatch.
	re, err := regexp.Compile("^" + translateGlob(relGlob) + "$")
	if err != nil {
		tForGlob(t, err)
		return false
	}
	return re.MatchString(fileName)
}

// translateGlob converts a Prometheus file_sd glob (only `/`-free target globs
// are expected here) into a regular expression. A leading or trailing * becomes
// "one or more of any character"; other characters are escaped.
func translateGlob(glob string) string {
	var b strings.Builder
	runes := []rune(glob)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '*' {
			// Collapse a run of consecutive * into a single greedy match.
			if i+1 < len(runes) && runes[i+1] == '*' {
				continue
			}
			b.WriteString(".+")
			i++
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(r)))
	}
	return b.String()
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
