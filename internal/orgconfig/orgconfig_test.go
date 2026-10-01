package orgconfig

import (
	"context"
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
		"company-go-api": "apiVersion: baseharbor.dev/v1\nkind: StackProfile\nmetadata:\n  name: go-api\ncomponents:\n  - id: app\n    role: application\n    adapter: go\n",
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
