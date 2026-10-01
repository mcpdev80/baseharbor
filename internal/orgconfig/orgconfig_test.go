package orgconfig

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
    - reference: policy:company-security
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
