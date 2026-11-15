package orgconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeOrgArtifactFixtures(t *testing.T, root string) {
	t.Helper()
	for name, content := range map[string]string{
		"company-dev":    "kind: target\nname: company-dev\n",
		"company-go-api": "apiVersion: https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/development/v1\nkind: StackProfile\nmetadata:\n  name: go-api\ncomponents:\n  - id: app\n    role: application\n    adapter: go\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const testOrgYAML = `apiVersion: baseharbor.organization/v1
organization: acme
targets:
  dev:
    reference: company-dev
stacks:
  go-api:
    reference: company-go-api
providers:
  company-postgres:
    reference: external-provider:company-postgres
trust:
  corp:
    reference: trust:corp-pki
policies:
  security:
    reference: policy:company-security
defaults:
  target: dev
  stack: go-api
  providers:
    database.sql:
      provider: company-postgres
      scope: external
  trust:
    default:
      reference: trust:corp-pki
  policies:
    - policy: security
      mandatory: true
environments:
  prod:
    target: dev
    providers:
      database.sql:
        provider: company-postgres
        scope: external
`

func TestLocalActivationAndEffectiveProvenance(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv(worldWritableCheckOptOutEnv, "skip")
	sourceDir := filepath.Join(root, "org")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "organization.yaml"), []byte(testOrgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	writeOrgArtifactFixtures(t, sourceDir)
	state, err := Activate(context.Background(), Source{Kind: SourceLocal, Location: sourceDir})
	if err != nil {
		t.Fatal(err)
	}
	if state.Resolution.ResolvedDigest == "" || !strings.HasPrefix(state.Resolution.Provenance, "file://") {
		t.Fatalf("resolution is not immutable/attributed: %+v", state.Resolution)
	}
	if info, err := os.Stat(state.Resolution.CachePath); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("cache path is missing or not private: %v %v", info, err)
	}
	effective, err := ResolveEffective(state, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if effective.Target == nil || effective.Target.Name != "dev" || effective.Target.Value != "company-dev" || effective.Target.Source != "organization.environment.prod" {
		t.Fatalf("unexpected effective target: %+v", effective.Target)
	}
	p := effective.Providers["database.sql"]
	if p.Provider != "company-postgres" || p.Reference != "external-provider:company-postgres" || p.Scope != "external" || p.Source != "organization.environment.prod" {
		t.Fatalf("unexpected effective provider: %+v", p)
	}
	if len(effective.Policies) != 1 || effective.Policies[0].Policy != "security" || effective.Policies[0].Reference != "policy:company-security" || !effective.Policies[0].Mandatory {
		t.Fatalf("mandatory policy reference/provenance was not preserved: %+v", effective.Policies)
	}
}

func TestSecretMaterialIsRejected(t *testing.T) {
	var config Config
	bad := strings.Replace(testOrgYAML, "trust:corp-pki", "password=hunter2", 1)
	if _, err := parseConfig([]byte(bad)); err == nil {
		t.Fatal("expected plaintext secret-like reference to be rejected")
	}
	_ = config
}

func TestGitResolutionPinsRevisionAndCheckDoesNotActivate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv(worldWritableCheckOptOutEnv, "skip")
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.invalid")
	runGitTest(t, repo, "config", "user.name", "BaseHarbor Test")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "organization.yaml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		runGitTest(t, repo, "add", "organization.yaml")
		runGitTest(t, repo, "commit", "-q", "-m", "org config")
	}
	writeOrgArtifactFixtures(t, repo)
	runGitTest(t, repo, "add", "company-dev", "company-go-api")
	runGitTest(t, repo, "commit", "-q", "-m", "org artifacts")
	write(testOrgYAML)
	state, err := Activate(context.Background(), Source{Kind: SourceGit, Location: repo, Requested: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	first := state.Resolution.ResolvedRevision
	if len(first) != 40 {
		t.Fatalf("expected pinned git revision, got %q", first)
	}
	write(strings.Replace(testOrgYAML, "organization: acme", "organization: acme2", 1))
	status, available, err := Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Changed || status.Available.ResolvedRevision == first {
		t.Fatalf("expected available revision change: %+v", status)
	}
	active, err := LoadActive()
	if err != nil {
		t.Fatal(err)
	}
	if active.Resolution.ResolvedRevision != first || active.Config.Organization != "acme" {
		t.Fatalf("check silently changed active config: %+v", active)
	}
	if available.Organization != "acme2" {
		t.Fatalf("unexpected available config: %+v", available)
	}
}

func TestSystemSourceUsesExplicitManagedPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	path := filepath.Join(root, "managed.yaml")
	if err := os.WriteFile(path, []byte(testOrgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	writeOrgArtifactFixtures(t, root)
	resolution, config, err := Resolve(context.Background(), Source{Kind: SourceSystem, Location: path})
	if err != nil {
		t.Fatal(err)
	}
	if config.Organization != "acme" || resolution.Source.Kind != SourceSystem {
		t.Fatalf("unexpected system resolution: %+v %+v", resolution, config)
	}
}

func TestOCIDigestParsing(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	if got := firstDigest("Resolved " + digest); got != digest {
		t.Fatalf("got %q", got)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestOCIResolutionPinsDigestAndPullsImmutableReference(t *testing.T) {
	t.Setenv(OrgRegistryOptInEnv, "true")
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	fixture := filepath.Join(root, "fixture.yaml")
	if err := os.WriteFile(fixture, []byte(testOrgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	writeOrgArtifactFixtures(t, root)
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("c", 64)
	oras := filepath.Join(bin, "oras")
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"case \"$1\" in\n" +
		"  resolve) echo '" + digest + "' ;;\n" +
		"  pull)\n" +
		"    ref=\"$2\"; shift 2; out=\"\"\n" +
		"    while [ \"$#\" -gt 0 ]; do if [ \"$1\" = --output ]; then out=\"$2\"; shift 2; else shift; fi; done\n" +
		"    case \"$ref\" in *@'" + digest + "') ;; *) echo bad-ref >&2; exit 9 ;; esac\n" +
		"    mkdir -p \"$out\"; cp \"$ORG_FIXTURE\" \"$out/organization.yaml\"; cp \"$(dirname \"$ORG_FIXTURE\")/company-dev\" \"$out/company-dev\"; cp \"$(dirname \"$ORG_FIXTURE\")/company-go-api\" \"$out/company-go-api\" ;;\n" +
		"  *) exit 8 ;;\n" +
		"esac\n"
	if err := os.WriteFile(oras, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ORG_FIXTURE", fixture)

	resolution, config, err := Resolve(context.Background(), Source{
		Kind: SourceOCI, Location: "registry.example.invalid/platform/baseharbor-org", Requested: "stable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.ResolvedDigest != digest {
		t.Fatalf("resolved digest=%q want=%q", resolution.ResolvedDigest, digest)
	}
	if resolution.Source.Requested != "stable" {
		t.Fatalf("requested channel was not retained: %+v", resolution.Source)
	}
	if resolution.Provenance != "registry.example.invalid/platform/baseharbor-org@"+digest {
		t.Fatalf("unexpected immutable provenance: %q", resolution.Provenance)
	}
	if config.Organization != "acme" {
		t.Fatalf("unexpected resolved organization: %+v", config)
	}
	if _, err := os.Stat(resolution.CachePath); err != nil {
		t.Fatalf("immutable OCI cache missing: %v", err)
	}
}

func TestTypedOrganizationSourceErrors(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	if _, err := LoadActive(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("LoadActive error = %v, want ErrNotConfigured", err)
	}

	_, _, err := Resolve(context.Background(), Source{Kind: SourceKind("unsupported"), Location: "ignored"})
	if !errors.Is(err, ErrUnsupportedSource) {
		t.Fatalf("Resolve unsupported error = %v, want ErrUnsupportedSource", err)
	}

	_, _, err = Resolve(context.Background(), Source{Kind: SourceGit, Location: filepath.Join(root, "missing.git"), Requested: "HEAD"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("Resolve unavailable Git error = %v, want ErrSourceUnavailable", err)
	}
}

func TestCanonicalLocationUsesUserConfigDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", root)

	path, err := ActivePath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "baseharbor", "organization", "active.json"); filepath.Clean(path) != want {
		t.Fatalf("ActivePath = %q, want %q (canonical user-state location)", path, want)
	}
	// The canonical location is not world-writable, so the guard must pass.
	if info, err := os.Stat(root); err != nil || info.Mode()&0o002 != 0 {
		t.Fatalf("test config root is not private: %v %v", info, err)
	}
}

func TestWorldWritableConfigRootIsRejected(t *testing.T) {
	// Isolate the private fallback: HOME points at a test-private dir that does
	// not yet contain a selection, so the migration fallback can never
	// accidentally satisfy the write against the real home directory.
	privateHome := t.TempDir()
	t.Setenv("HOME", privateHome)

	root := t.TempDir() // t.TempDir() is 0777 world-writable
	cfgDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	// A fresh selection into a world-writable XDG_CONFIG_HOME must be rejected
	// outright: there is no legacy file to adopt, so writing into a
	// world-writable location is refused.
	state := ActiveState{
		Resolution: Resolution{ResolvedRevision: "deadbeef", Source: Source{Kind: SourceLocal, Location: "ignored"}},
		Config:     parseConfigForTest(t),
	}
	err := SaveActive(state)
	if err == nil {
		t.Fatal("SaveActive into a world-writable XDG_CONFIG_HOME with no legacy file must be rejected")
	}
}

func TestWorldWritableMigrationAdoptsOnceThenFreezes(t *testing.T) {
	// Isolate the private fallback: HOME points at a test-private dir so the
	// migration target (~/.config) never touches the real home directory.
	privateHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(privateHome, ".config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", privateHome)

	// The resolved selection location is world-writable (broken XDG_CONFIG_HOME).
	root := t.TempDir() // 0777 world-writable
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	// Seed a stale /tmp selection at the (world-writable) resolved path.
	seed := `{"resolution":{"resolved_revision":"mig-1","source":{"kind":"local","location":"x"}},"config":` + testConfigJSON(t) + `}`
	seedPath, err := ActivePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(seedPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(seedPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if !isWorldWritableDir(filepath.Dir(seedPath)) {
		t.Fatalf("test did not create a world-writable selection directory")
	}
	if err := os.WriteFile(seedPath, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	// LoadActive adopts the legacy /tmp selection into the private HOME fallback.
	loaded, err := LoadActive()
	if err != nil {
		t.Fatalf("LoadActive after migration = %v", err)
	}
	if loaded.Resolution.ResolvedRevision != "mig-1" {
		t.Fatalf("LoadActive did not return the adopted selection: %+v", loaded.Resolution)
	}

	// The value now lives in the private, non-world-writable canonical path.
	canonical, err := privateCanonicalPath()
	if err != nil {
		t.Fatal(err)
	}
	if isWorldWritableDir(filepath.Dir(canonical)) {
		t.Fatalf("canonical path is still world-writable: %q", canonical)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		t.Fatalf("canonical private selection missing after migration: %v %v", info, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("canonical selection file mode = %v, want exactly 0o600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(canonical))
	if err != nil {
		t.Fatalf("stat canonical organization directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("canonical organization directory mode = %v, want exactly 0o700", dirInfo.Mode().Perm())
	}

	// The source /tmp file is frozen read-only (0400) with a tombstone; it must
	// never be written again.
	if info, err := os.Stat(seedPath); err == nil {
		if info.Mode().Perm() != 0o400 {
			t.Fatalf("legacy /tmp selection mode = %v, want exactly 0o400 after freeze", info.Mode().Perm())
		}
	}
	if !isWorldFrozenTombstone(filepath.Dir(seedPath)) {
		t.Fatal("expected tombstone marker next to the frozen legacy selection")
	}

	// A subsequent write goes to the private canonical path only; the /tmp
	// source is never re-touched.
	state := ActiveState{
		Resolution: Resolution{ResolvedRevision: "mig-2", Source: Source{Kind: SourceLocal, Location: "y"}},
		Config:     parseConfigForTest(t),
	}
	if err := SaveActive(state); err != nil {
		t.Fatalf("SaveActive after migration = %v", err)
	}
	final, err := LoadActive()
	if err != nil {
		t.Fatalf("LoadActive after write = %v", err)
	}
	if final.Resolution.ResolvedRevision != "mig-2" {
		t.Fatalf("write after migration did not land in canonical path: %+v", final.Resolution)
	}
	// The frozen /tmp source still holds the original migrated content.
	data, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read legacy /tmp selection: %v", err)
	}
	if !strings.Contains(string(data), "mig-1") {
		t.Fatalf("legacy /tmp source was re-touched by migration write: %q", string(data))
	}
}

// TestWorldWritableMigrationRejectsForeignSymlink asserts fail-closed behavior
// for a positive symlink case: the legacy selection at the world-writable
// resolved path is a symlink to a foreign file. LoadActive must refuse (the
// symlink is rejected before any read) and the foreign target must remain
// byte-for-byte and mode unchanged. This is the positive symlink regression
// the PR #1582 review R2 required.
func TestWorldWritableMigrationRejectsForeignSymlink(t *testing.T) {
	// Isolate the private fallback so nothing is written to the real home.
	privateHome := t.TempDir()
	t.Setenv("HOME", privateHome)

	// World-writable resolved location (broken XDG_CONFIG_HOME).
	root := t.TempDir() // 0777 world-writable
	cfgDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	// A foreign target that must never be altered.
	foreignDir := filepath.Join(t.TempDir(), "foreign")
	if err := os.MkdirAll(foreignDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignPath := filepath.Join(foreignDir, "target.json")
	foreignBytes := []byte(`{"resolution":{"resolved_revision":"foreign","source":{"kind":"local","location":"foreign"}},"config":` + testConfigJSON(t) + `}`)
	if err := os.WriteFile(foreignPath, foreignBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	foreignInfo, err := os.Stat(foreignPath)
	if err != nil {
		t.Fatal(err)
	}
	origForeignMode := foreignInfo.Mode().Perm()

	// Seed the legacy path as a symlink to the foreign file.
	legacyPath, err := ActivePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(legacyPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignPath, legacyPath); err != nil {
		t.Fatal(err)
	}
	// Verify the symlink is a symlink (Lstat must not follow it).
	legacyInfo, err := os.Lstat(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if legacyInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("test did not create a symlink at %q", legacyPath)
	}

	// LoadActive must fail closed: a symlink must not be adopted.
	_, err = LoadActive()
	if err == nil {
		t.Fatal("LoadActive into a world-writable XDG with a symlink legacy selection must fail closed")
	}

	// The foreign target must be completely unchanged (bytes and mode): the
	// adoption rejected before any read could touch it.
	got, err := os.ReadFile(foreignPath)
	if err != nil {
		t.Fatalf("read foreign target: %v", err)
	}
	if string(got) != string(foreignBytes) {
		t.Fatalf("foreign target was altered by symlink adoption: got %q", string(got))
	}
	afterInfo, err := os.Stat(foreignPath)
	if err != nil {
		t.Fatalf("stat foreign target: %v", err)
	}
	if afterInfo.Mode().Perm() != origForeignMode {
		t.Fatalf("foreign target mode changed: before %v after %v", origForeignMode, afterInfo.Mode().Perm())
	}
}

// TestWorldWritableMigrationFrozenWhenSourceAndTargetRemoved is the regression
// the r2/r3 reviews required: after a successful migration, if both the private
// canonical active.json and the legacy /tmp source are removed while the
// durable once-only marker in the private canonical namespace remains, no
// re-adoption may occur (the marker honors the freeze) and LoadActive must
// return the normal not-configured result — not a decode error, not a panic.
func TestWorldWritableMigrationFrozenWhenSourceAndTargetRemoved(t *testing.T) {
	privateHome := t.TempDir()
	t.Setenv("HOME", privateHome)

	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	seed := `{"resolution":{"resolved_revision":"mig-1","source":{"kind":"local","location":"x"}},"config":` + testConfigJSON(t) + `}`
	legacyPath, err := ActivePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(legacyPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	// Adopt the legacy selection into the private canonical path.
	loaded, err := LoadActive()
	if err != nil {
		t.Fatalf("LoadActive after migration = %v", err)
	}
	if loaded.Resolution.ResolvedRevision != "mig-1" {
		t.Fatalf("LoadActive did not return the adopted selection: %+v", loaded.Resolution)
	}

	canon, err := privateCanonicalPath()
	if err != nil {
		t.Fatal(err)
	}
	// The durable once-only marker must be present after a successful adopt.
	marker := filepath.Join(filepath.Dir(canon), canonicalFrozenMarker)
	if _, err := os.Lstat(marker); err != nil {
		t.Fatalf("expected durable migration marker %q: %v", marker, err)
	}
	// The legacy source must still be frozen read-only (0400).
	if info, err := os.Stat(legacyPath); err == nil {
		if info.Mode().Perm() != 0o400 {
			t.Fatalf("legacy frozen source mode = %v, want 0o400", info.Mode().Perm())
		}
	}

	// Remove ONLY the canonical active.json, leaving the frozen legacy /tmp
	// source in place. With the marker present, no re-adoption may occur.
	//
	// This deliberately keeps the legacy source: without the durable marker the
	// loader would fall through to adoptLegacySelection, re-read the retained
	// legacy selection and return it successfully. The assertion therefore
	// proves the marker — not merely the absent source — is what makes the
	// migration once-only.
	if err := os.Remove(canon); err != nil {
		t.Fatalf("remove canonical: %v", err)
	}

	// LoadActive must return ErrNotConfigured: the migration is frozen, so the
	// absent canonical state yields the normal not-configured result and the
	// legacy path is never re-read.
	_, err = LoadActive()
	if err == nil {
		t.Fatal("LoadActive with marker present but canonical missing must return ErrNotConfigured")
	}
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("LoadActive error = %v, want ErrNotConfigured", err)
	}
}

func parseConfigForTest(t *testing.T) Config {
	t.Helper()
	config, err := parseConfig([]byte(testOrgYAML))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	return config
}

func parseConfigForTestRaw() Config {
	config, err := parseConfig([]byte(testOrgYAML))
	if err != nil {
		panic(err)
	}
	return config
}

func testConfigJSON(t *testing.T) string {
	t.Helper()
	config := parseConfigForTest(t)
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return string(data)
}

// SaveActiveForTest writes a state with the default local source, exercising
// the world-writable migration path under test control.
func SaveActiveForTest() (ActiveState, error) {
	state := ActiveState{
		Resolution: Resolution{ResolvedRevision: "mig-1", Source: Source{Kind: SourceLocal, Location: "ignored"}},
		Config:     parseConfigForTestRaw(),
	}
	return state, SaveActive(state)
}
