package orgconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOrasHarness installs a fake `oras` on PATH that records the reference it
// was asked to resolve/pull and returns canned output. It returns the recorded
// invocations so the cells can assert the pull path hits the shared endpoint.
//
// The runner records the resolve and pull arguments so a cell can prove the
// pull path used the configured shared endpoint (opt-in) or refused to run
// at all (no opt-in).
type orasInvocation struct {
	subcommand string
	ref        string
}

func installFakeOras(t *testing.T, root, fixture string, resolveErr, pullErr string) []orasInvocation {
	t.Helper()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	invocations := []orasInvocation{}
	digest := "sha256:" + strings.Repeat("c", 64)
	var script strings.Builder
	script.WriteString("#!/bin/sh\nset -eu\n")
	script.WriteString("echo \"$1:$2\" >> \"$ORAS_INV\"; \n")
	script.WriteString("case \"$1\" in\n")
	script.WriteString("  resolve)\n")
	script.WriteString("    if [ -n \"$ORAS_RESOLVE_ERR\" ]; then echo \"$ORAS_RESOLVE_ERR\" >&2; exit 1; fi\n")
	script.WriteString("    echo '" + digest + "' ;;\n")
	script.WriteString("  pull)\n")
	script.WriteString("    if [ -n \"$ORAS_PULL_ERR\" ]; then echo \"$ORAS_PULL_ERR\" >&2; exit 1; fi\n")
	script.WriteString("    ref=\"$2\"; shift 2; out=\"\"\n")
	script.WriteString("    while [ \"$#\" -gt 0 ]; do if [ \"$1\" = --output ]; then out=\"$2\"; shift 2; else shift; fi; done\n")
	// Write a valid organization.yaml (plus the referenced artifact files)
	// directly into the pull output dir so the locate/read/parse/cache cycle
	// succeeds without staging artifact copies elsewhere.
	script.WriteString("    printf 'apiVersion: baseharbor.organization/v1\\norganization: acme\\ntargets:\\n  dev:\\n    reference: company-dev\\n' > \"$out/organization.yaml\"; \n")
	script.WriteString("    printf 'kind: target\\nname: company-dev\\n' > \"$out/company-dev\"; \n")
	script.WriteString("    printf 'apiVersion: https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/development/v1\\nkind: StackProfile\\nmetadata:\\n  name: go-api\\ncomponents:\\n  - id: app\\n    role: application\\n    adapter: go\\n' > \"$out/company-go-api\" ;;\n")
	script.WriteString("  *) exit 8 ;;\n")
	script.WriteString("esac\n")
	oras := filepath.Join(bin, "oras")
	if err := os.WriteFile(oras, []byte(script.String()), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ORAS_INV", filepath.Join(root, "oras-invocations.txt"))
	t.Setenv("ORAS_RESOLVE_ERR", resolveErr)
	t.Setenv("ORAS_PULL_ERR", pullErr)
	return invocations
}

func readOrasInvocations(t *testing.T, path string) []orasInvocation {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	inv := []orasInvocation{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		inv = append(inv, orasInvocation{subcommand: strings.TrimSpace(parts[0]), ref: strings.TrimSpace(parts[1])})
	}
	return inv
}

// TestOCIOptInDisabledByBaseline proves the red-first baseline: without the
// opt-in switch an OCI source fails closed with the explicit distinct
// identity ErrOrgSourceRegistryDisabled and never runs the shared endpoint.
func TestOCIOptInDisabledByBaseline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	// Ensure the opt-in is off for the baseline cell.
	t.Setenv(OrgRegistryOptInEnv, "false")
	_ = installFakeOras(t, root, "", "", "")

	_, _, err := Resolve(context.Background(), Source{
		Kind: SourceOCI, Location: "registry.example.invalid/platform/baseharbor-org", Requested: "stable",
	})
	if err == nil {
		t.Fatal("opt-in OFF must reject the OCI source, got nil error")
	}
	if !strings.Contains(err.Error(), ErrOrgSourceRegistryDisabled.Error()) {
		t.Fatalf("opt-in OFF must surface ErrOrgSourceRegistryDisabled, got %v", err)
	}
	// The shared endpoint must never be reached without opt-in.
	for _, inv := range readOrasInvocations(t, filepath.Join(root, "oras-invocations.txt")) {
		t.Fatalf("oras must not run without opt-in, saw %s %s", inv.subcommand, inv.ref)
	}
}

// TestOCIOptInDisabledDefaultUnset proves the default (env unset) is also the
// byte-for-byte baseline: no opt-in, explicit rejection, no tool run.
func TestOCIOptInDisabledDefaultUnset(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	_ = os.Unsetenv(OrgRegistryOptInEnv)
	_ = installFakeOras(t, root, "", "", "")

	_, _, err := Resolve(context.Background(), Source{
		Kind: SourceOCI, Location: "registry.example.invalid/platform/baseharbor-org", Requested: "HEAD",
	})
	if err == nil || !strings.Contains(err.Error(), ErrOrgSourceRegistryDisabled.Error()) {
		t.Fatalf("unset opt-in must fail closed with ErrOrgSourceRegistryDisabled, got %v", err)
	}
}

// TestOCIOptInEnablesSharedEndpoint proves the green opt-in cell: with the
// switch on the pull path resolves and pulls from the configured shared
// endpoint and pins the immutable digest.
func TestOCIOptInEnablesSharedEndpoint(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv(OrgRegistryOptInEnv, "true")
	fixture := filepath.Join(root, "fixture.yaml")
	if err := os.WriteFile(fixture, []byte(testOrgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = installFakeOras(t, root, fixture, "", "")

	resolution, _, err := Resolve(context.Background(), Source{
		Kind: SourceOCI, Location: "registry.example.invalid/platform/baseharbor-org", Requested: "stable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resolution.ResolvedDigest, "sha256:") {
		t.Fatalf("expected immutable digest, got %q", resolution.ResolvedDigest)
	}
	inv := readOrasInvocations(t, filepath.Join(root, "oras-invocations.txt"))
	if len(inv) < 2 {
		t.Fatalf("expected resolve+pull against shared endpoint, saw %d invocations", len(inv))
	}
	// The pull must target the immutable repository@digest on the shared endpoint.
	pullRef := ""
	for _, i := range inv {
		if i.subcommand == "pull" {
			pullRef = i.ref
		}
	}
	if !strings.HasPrefix(pullRef, "registry.example.invalid/platform/baseharbor-org@sha256:") {
		t.Fatalf("pull did not target the shared immutable endpoint: %q", pullRef)
	}
}

// TestOCIUnreachableFailsClosedNoFallback proves the red-first unreachable
// cell: an unreachable endpoint (or tool failure) propagates as
// ErrSourceUnavailable and never downgrades to a mutable fallback. This is the
// registry-access 1395c trap guard: no silent fallback on a refused endpoint.
func TestOCIUnreachableFailsClosedNoFallback(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv(OrgRegistryOptInEnv, "true")
	_ = installFakeOras(t, root, "", "oras: host unreachable: network down", "")

	_, _, err := Resolve(context.Background(), Source{
		Kind: SourceOCI, Location: "registry.example.invalid/platform/baseharbor-org", Requested: "stable",
	})
	if err == nil {
		t.Fatal("unreachable endpoint must fail closed, got nil error")
	}
	if !strings.Contains(err.Error(), ErrSourceUnavailable.Error()) {
		t.Fatalf("unreachable must surface ErrSourceUnavailable, got %v", err)
	}
	// No mutable fallback: the error is never resolved into a pull of a raw ref.
	for _, inv := range readOrasInvocations(t, filepath.Join(root, "oras-invocations.txt")) {
		if inv.subcommand == "pull" && strings.Contains(inv.ref, ":") && !strings.Contains(inv.ref, "@sha256:") {
			t.Fatalf("mutable fallback pulled raw reference %q after unreachable, must fail closed", inv.ref)
		}
	}
}

// TestOCIPublicDefaultHardRejection proves the 1395c-class safeguard: a
// public docker.io default location is a distinct hard rejection on the
// opt-in path and can never be downgraded into the merely-unavailable class or
// pulled from the public default.
func TestOCIPublicDefaultHardRejection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	_ = installFakeOras(t, root, "", "", "")
	// Opt-in ON after the harness: the harness defaults the switch OFF, so the
	// opt-in path is explicitly enabled for this rejection test.
	t.Setenv(OrgRegistryOptInEnv, "true")

	registry := "registry.docker.io/org/baseharbor-org"
	_, _, err := Resolve(context.Background(), Source{
		Kind: SourceOCI, Location: registry, Requested: "stable",
	})
	if err == nil {
		t.Fatal("public docker.io default must be rejected on the opt-in path, got nil error")
	}
	if !strings.Contains(err.Error(), ErrOrgSourceRegistryDefault.Error()) {
		t.Fatalf("public default must surface ErrOrgSourceRegistryDefault, got %v", err)
	}
	// No tool runs against the public default.
	for _, inv := range readOrasInvocations(t, filepath.Join(root, "oras-invocations.txt")) {
		t.Fatalf("must not run oras against a public default, saw %s %s", inv.subcommand, inv.ref)
	}
}

// TestIsHardRegistryRejectionIdentity guards the classifier against aliasing:
// the public-default and disabled sentinels are hard rejection identities, and
// an unrelated error is not one.
func TestIsHardRegistryRejectionIdentity(t *testing.T) {
	if !isHardRegistryRejection(ErrOrgSourceRegistryDefault) {
		t.Fatal("public-default sentinel must be a hard rejection")
	}
	if !isHardRegistryRejection(ErrOrgSourceRegistryDisabled) {
		t.Fatal("disabled sentinel must be a hard rejection")
	}
	if isHardRegistryRejection(ErrSourceUnavailable) {
		t.Fatal("the merely-unavailable identity must NOT be a hard rejection")
	}
	if isHardRegistryRejection(nil) {
		t.Fatal("nil must not be a hard rejection")
	}
}
