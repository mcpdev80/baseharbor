package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

// fakeRunner records each invocation and returns queued outputs, modelling a
// native image tool (podman/docker) that reuses the host credential helper /
// auth file: the caller observes only success/failure, never any credential
// material.
type fakeRunner struct {
	calls  []string
	output []string
	err    error
}

func (f *fakeRunner) fn(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if f.err != nil {
		return "", f.err
	}
	if len(f.output) > 0 {
		out := f.output[0]
		f.output = f.output[1:]
		return out, nil
	}
	return "", nil
}

// fakeRuntimeTarget is a RuntimeImageTransferTarget whose pull and verify are
// fully controlled, so the test pins exactly the reference that reaches the
// transport and exactly what the verify reports.
type fakeRuntimeTarget struct {
	runner    ExecFunc
	pullErr   error
	pulledRef string
	// digest, manifestDigest, architecture, os are the values Verify reports.
	digest         string
	manifestDigest string
	architecture   string
	os             string
	verifyUnavail  bool
}

func (t *fakeRuntimeTarget) Copy(ctx context.Context, ref string) error {
	if t.pullErr != nil {
		return t.pullErr
	}
	t.pulledRef = ref
	// A real pull must address an immutable repository@digest, never a tag.
	if !strings.Contains(ref, "@sha256:") {
		return errors.New("refusing to pull mutable tag reference " + ref)
	}
	return nil
}

func (t *fakeRuntimeTarget) Verify(ctx context.Context, ref string) (ImageTransferIdentity, error) {
	if t.verifyUnavail {
		return ImageTransferIdentity{}, fmt.Errorf("%w: %v", ErrImageTransferUnavailable, errors.New("podman: inspect: no such image"))
	}
	return ImageTransferIdentity{
		Repository:     strings.TrimSuffix(ref, "@"+t.digest),
		Digest:         t.digest,
		ManifestDigest: t.manifestDigest,
		Architecture:   t.architecture,
		OS:             t.os,
	}, nil
}

var sha256a = "sha256:" + strings.Repeat("a", 64)
var sha256b = "sha256:" + strings.Repeat("b", 64)

func TestImageTransferPreservesDigestAndReturnsImmutableIdentity(t *testing.T) {
	// Positive cell: a digest-preserving copy to an immutable repository@digest
	// returns the repository@digest pin, never a mutable tag.
	runner := &fakeRunner{output: []string{
		"registry.example.invalid/org/app@" + sha256a, // source RepoDigests
		"amd64|linux", // source platform
	}}
	target := &fakeRuntimeTarget{
		runner:       runner.fn,
		digest:       sha256a,
		architecture: "amd64",
		os:           "linux",
	}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app:stable", Exec: runner.fn}

	id, err := ImageTransfer(context.Background(), source, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.ImmutableIdentity() != "registry.example.invalid/org/app@"+sha256a {
		t.Fatalf("immutable identity must be repository@digest: got %q", id.ImmutableIdentity())
	}
	if !strings.Contains(target.pulledRef, "@"+sha256a) {
		t.Fatalf("transport must pull the exact immutable pin, got %q", target.pulledRef)
	}
	if strings.Contains(target.pulledRef, ":") && !strings.Contains(target.pulledRef, "@") {
		t.Fatalf("mutable tag reference reached the transport: %q", target.pulledRef)
	}
}

func TestImageTransferFailsClosedWhenCopyTargetUnreachable(t *testing.T) {
	// The transfer-target error cell: a failing target copy must propagate as an
	// explicit error, NOT a half-success.
	runner := &fakeRunner{}
	target := &fakeRuntimeTarget{runner: runner.fn, pullErr: errors.New("podman: pull: connection refused")}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}

	_, err := ImageTransfer(context.Background(), source, target)
	if err == nil {
		t.Fatal("a failing transfer target must return an explicit error, not half-success")
	}
	if !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("transfer-target failure must be explicit, got %v", err)
	}
	if target.pulledRef != "" {
		t.Fatalf("no copy must be reported when the target fails: got %q", target.pulledRef)
	}
}

func TestImageTransferFailsClosedWhenVerifyReportsWrongDigest(t *testing.T) {
	// Tag drift / swapped-payload cell: the target verifies a different digest
	// than the source pinned. This is a failed transfer, never a success.
	runner := &fakeRunner{output: []string{
		"registry.example.invalid/org/app@" + sha256a, // source resolves sha256a
		"amd64|linux",
	}}
	target := &fakeRuntimeTarget{
		runner:       runner.fn,
		digest:       sha256b, // target reports sha256b
		architecture: "amd64",
	}
	source := RuntimeImageTransferSource{
		Runtime:   "podman",
		Location:  "registry.example.invalid/org/app",
		Requested: "stable",
		Exec:      runner.fn,
	}

	_, err := ImageTransfer(context.Background(), source, target)
	if err == nil {
		t.Fatal("digest mismatch must fail the transfer, not return a success")
	}
	if !errors.Is(err, ErrImageTransferDigestMismatch) {
		t.Fatalf("digest mismatch must surface as digest mismatch, got %v", err)
	}
}

func TestImageTransferFailsClosedOnArchitectureMismatch(t *testing.T) {
	// Architecture drift cell: same digest, different architecture. The runtime
	// payload would differ even at the same digest scope, so it is rejected.
	runner := &fakeRunner{output: []string{
		"registry.example.invalid/org/app@" + sha256a, // source resolves sha256a
		"amd64|linux", // source verified amd64
	}}
	target := &fakeRuntimeTarget{
		runner:       runner.fn,
		digest:       sha256a,
		architecture: "arm64", // target reports arm64
	}
	source := RuntimeImageTransferSource{
		Runtime:   "podman",
		Location:  "registry.example.invalid/org/app",
		Requested: "stable",
		Exec:      runner.fn,
	}

	_, err := ImageTransfer(context.Background(), source, target)
	if err == nil {
		t.Fatal("architecture mismatch must fail the transfer")
	}
	if !errors.Is(err, ErrImageTransferArchitectureMismatch) {
		t.Fatalf("architecture mismatch must surface explicitly, got %v", err)
	}
}

func TestImageTransferFailsClosedWhenVerifyUnavailable(t *testing.T) {
	// A target that cannot prove the transferred identity fails closed; no
	// half-success.
	runner := &fakeRunner{output: []string{
		"registry.example.invalid/org/app@" + sha256a,
		"amd64|linux",
	}}
	target := &fakeRuntimeTarget{runner: runner.fn, digest: sha256a, architecture: "amd64", os: "linux", verifyUnavail: true}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}

	if _, err := ImageTransfer(context.Background(), source, target); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("unverifiable transfer must fail closed, got %v", err)
	}
}

// platformInspectRunner models the native tool cleanly resolving the digest but
// failing, returning empty, or returning malformed platform data.
type platformInspectRunner struct {
	calls      []string
	output     []string
	inspectErr error
	emptyOut   bool
	malformed  bool
}

func (p *platformInspectRunner) fn(_ context.Context, name string, args ...string) (string, error) {
	p.calls = append(p.calls, name+" "+strings.Join(args, " "))
	if strings.Contains(strings.Join(args, " "), ".Architecture") {
		// platform inspect call
		if p.inspectErr != nil {
			return "", p.inspectErr
		}
		if p.emptyOut {
			return "", nil
		}
		if p.malformed {
			return "amd64", nil // missing the |OS separator
		}
		return "", nil
	}
	if len(p.output) > 0 {
		out := p.output[0]
		p.output = p.output[1:]
		return out, nil
	}
	return "", nil
}

func TestImageTransferFailsClosedOnPlatformInspectCommandError(t *testing.T) {
	// FAIL-CLOSED: a failing platform inspect on the source must fail the
	// transfer closed, never return a transfer with unpinned architecture/OS.
	runner := &platformInspectRunner{
		output:     []string{"registry.example.invalid/org/app@" + sha256a},
		inspectErr: errors.New("podman: inspect: error occurred"),
	}
	target := &fakeRuntimeTarget{runner: runner.fn, digest: sha256a, architecture: "amd64", os: "linux"}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}

	if _, err := ImageTransfer(context.Background(), source, target); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("platform inspect command error must fail closed, got %v", err)
	}
}

func TestImageTransferFailsClosedOnPlatformInspectEmptyOutput(t *testing.T) {
	// FAIL-CLOSED: an empty platform inspect report must fail the transfer, not
	// pass with an empty architecture/OS.
	runner := &platformInspectRunner{
		output:   []string{"registry.example.invalid/org/app@" + sha256a},
		emptyOut: true,
	}
	target := &fakeRuntimeTarget{runner: runner.fn, digest: sha256a, architecture: "amd64", os: "linux"}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}

	if _, err := ImageTransfer(context.Background(), source, target); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("empty platform inspect output must fail closed, got %v", err)
	}
}

func TestImageTransferFailsClosedOnPlatformInspectMalformedOutput(t *testing.T) {
	// FAIL-CLOSED: a platform inspect report missing the |OS separator must fail
	// the transfer closed, not pass with an empty OS.
	runner := &platformInspectRunner{
		output:    []string{"registry.example.invalid/org/app@" + sha256a},
		malformed: true,
	}
	target := &fakeRuntimeTarget{runner: runner.fn, digest: sha256a, architecture: "amd64", os: "linux"}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}

	if _, err := ImageTransfer(context.Background(), source, target); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("malformed platform inspect output must fail closed, got %v", err)
	}
}

func TestImageTransferFailsClosedOnOsDrift(t *testing.T) {
	// OS-drift cell: same digest, same architecture, but a different OS. The
	// transferred OS must match the source-pinned OS exactly.
	runner := &platformInspectRunner{output: []string{"registry.example.invalid/org/app@" + sha256a}}
	target := &fakeRuntimeTarget{runner: runner.fn, digest: sha256a, architecture: "amd64", os: "windows"}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}

	if _, err := ImageTransfer(context.Background(), source, target); err == nil {
		t.Fatal("OS drift must fail the transfer")
	}
}

func TestImageTransferSourceFailsClosedOnPublicDefault(t *testing.T) {
	// No-copy cell: a public-default source must be rejected before any copy, so
	// a copy can never silently fall back to docker.io.
	runner := &fakeRunner{}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "docker.io/library/postgres:15", Exec: runner.fn}

	if _, err := source.Resolve(context.Background()); !errors.Is(err, ErrImageTransferTargetRefused) {
		t.Fatalf("public-default source must fail closed with target-refused, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("a public-default source must not invoke the native tool at all, calls=%v", runner.calls)
	}
}

func TestImageTransferSourceFailsClosedOnMutableTagIdentity(t *testing.T) {
	// The resolved source must carry an immutable sha256 digest identity. A
	// reply without a digest (mutable tag) is rejected rather than accepted.
	runner := &fakeRunner{output: []string{}}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}
	runner.output = append(runner.output, "registry.example.invalid/orgapp") // no digest

	if _, err := source.Resolve(context.Background()); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("mutable-tag source identity must fail closed, got %v", err)
	}
}

func TestImageTransferSourceFailsClosedOnUnreachableHost(t *testing.T) {
	runner := &fakeRunner{err: errors.New("podman: inspect: lookup failed: no such host")}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app", Exec: runner.fn}

	if _, err := source.Resolve(context.Background()); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("unreachable host must fail closed, got %v", err)
	}
}

func TestImageTransferIdentityVerifyMatches(t *testing.T) {
	id := ImageTransferIdentity{
		Repository:     "registry.example.invalid/org/app",
		Digest:         sha256a,
		ManifestDigest: sha256b,
		Architecture:   "amd64",
		OS:             "linux",
	}
	if err := id.Verify(sha256a, sha256b, "amd64", "linux"); err != nil {
		t.Fatalf("matching identity must verify: %v", err)
	}
	if err := id.Verify(sha256b, sha256b, "amd64", "linux"); err == nil {
		t.Fatal("digest mismatch must fail Verify")
	}
	if err := id.Verify(sha256a, sha256a, "amd64", "linux"); err == nil {
		t.Fatal("manifest digest mismatch must fail Verify")
	}
	if err := id.Verify(sha256a, sha256b, "arm64", "linux"); err == nil {
		t.Fatal("architecture mismatch must fail Verify")
	}
	if err := id.Verify(sha256a, sha256b, "amd64", "windows"); err == nil {
		t.Fatal("os mismatch must fail Verify")
	}
	if err := id.Verify("", sha256b, "amd64", "linux"); err == nil {
		t.Fatal("missing digest must fail Verify")
	}
	// Fail-closed on empty architecture or os on the target side.
	if err := id.Verify(sha256a, sha256b, "", "linux"); err == nil {
		t.Fatal("empty architecture must fail Verify")
	}
	if err := id.Verify(sha256a, sha256b, "amd64", ""); err == nil {
		t.Fatal("empty os must fail Verify")
	}
}

func TestImageTransferIdentityImmutableIdentityIsPin(t *testing.T) {
	id := ImageTransferIdentity{Repository: "registry.example.invalid/org/app", Digest: sha256a}
	if id.ImmutableIdentity() != "registry.example.invalid/org/app@"+sha256a {
		t.Fatalf("immutable identity must be repository@digest, got %q", id.ImmutableIdentity())
	}
	// A digest-less identity returns the bare repository.
	empty := ImageTransferIdentity{Repository: "registry.example.invalid/org/app"}
	if empty.ImmutableIdentity() != "registry.example.invalid/org/app" {
		t.Fatalf("digest-less identity must return bare repository, got %q", empty.ImmutableIdentity())
	}
}

func TestSplitDigestReference(t *testing.T) {
	repo, digest := splitDigestReference("registry.example.invalid/org/app@" + sha256a)
	if repo != "registry.example.invalid/org/app" || digest != sha256a {
		t.Fatalf("splitDigestReference(repo@digest)=%q,%q want %q,%q", repo, digest, "registry.example.invalid/org/app", sha256a)
	}
	// A RepoDigests entry carries the host prefix: registry:443/org/app@digest.
	_, digest = splitDigestReference("registry:443/org/app@" + sha256a)
	if digest != sha256a {
		t.Fatalf("splitDigestReference dropped host prefix: digest=%q", digest)
	}
	repo, digest = splitDigestReference("registry.example.invalid/orgapp")
	if digest != "" || repo != "registry.example.invalid/orgapp" {
		t.Fatalf("reference without digest must yield empty digest: repo=%q digest=%q", repo, digest)
	}
}

func TestPodman57ImageInspectTemplatesResolve(t *testing.T) {
	// Positive host-contract cell: Podman 5.7 accepts the parenthesized index
	// expression and top-level platform fields, and rejects the old Docker-shaped
	// templates. This test deliberately executes the adapter's command builder.
	var calls []string
	runner := func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name != "podman" || len(args) != 5 || args[0] != "image" || args[1] != "inspect" || args[2] != "--format" {
			return "", fmt.Errorf("unexpected Podman inspect invocation: %s %v", name, args)
		}
		switch args[3] {
		case `{{(index .RepoDigests 0)}}`:
			return "registry.example.invalid/org/app@" + sha256a, nil
		case `{{.Architecture}}|{{.Os}}`:
			return "amd64|linux", nil
		default:
			return "", fmt.Errorf("Podman 5.7 rejected template %q", args[3])
		}
	}
	source := RuntimeImageTransferSource{Runtime: "podman", Location: "registry.example.invalid/org/app:stable", Exec: runner}
	identity, err := source.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Podman 5.7 adapter cell must resolve successfully: %v; calls=%v", err, calls)
	}
	if identity.ImmutableIdentity() != "registry.example.invalid/org/app@"+sha256a || identity.Architecture != "amd64" || identity.OS != "linux" {
		t.Fatalf("unexpected Podman identity: %+v", identity)
	}
}

func TestDockerImageInspectTemplatesResolve(t *testing.T) {
	var calls []string
	runner := func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name != "docker" || len(args) != 5 || args[0] != "image" || args[1] != "inspect" || args[2] != "--format" {
			return "", fmt.Errorf("unexpected Docker inspect invocation: %s %v", name, args)
		}
		switch args[3] {
		case `{{index .RepoDigests 0}}`:
			return "registry.example.invalid/org/app@" + sha256a, nil
		case `{{.Architecture}}|{{.Os}}`:
			return "amd64|linux", nil
		default:
			return "", fmt.Errorf("unexpected Docker template %q", args[3])
		}
	}
	source := RuntimeImageTransferSource{Runtime: "docker", Location: "registry.example.invalid/org/app:stable", Exec: runner}
	identity, err := source.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Docker adapter cell must resolve successfully: %v; calls=%v", err, calls)
	}
	if identity.ImmutableIdentity() != "registry.example.invalid/org/app@"+sha256a || identity.Architecture != "amd64" || identity.OS != "linux" {
		t.Fatalf("unexpected Docker identity: %+v", identity)
	}
}

func TestImageTransferRejectsUnsupportedRuntime(t *testing.T) {
	source := RuntimeImageTransferSource{Runtime: "kubernetes", Location: "registry.example.invalid/org/app"}
	if _, err := source.Resolve(context.Background()); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("unsupported runtime must fail closed, got %v", err)
	}
}

type imageTransferRuntimeStub struct {
	bhruntime.RuntimeProvider
	kind     bhruntime.ProviderKind
	pulled   string
	pullFail error
}

func (r *imageTransferRuntimeStub) Kind() bhruntime.ProviderKind { return r.kind }

func (r *imageTransferRuntimeStub) PullImage(_ context.Context, ref string) error {
	r.pulled = ref
	return r.pullFail
}

func TestRuntimeImageTransferTargetUsesNativeInspectFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind bhruntime.ProviderKind
		tool string
		fmt  string
	}{
		{name: "Docker", kind: bhruntime.ProviderDocker, tool: "docker", fmt: `{{index .RepoDigests 0}}`},
		{name: "Podman 5.7", kind: bhruntime.ProviderPodman, tool: "podman", fmt: `{{(index .RepoDigests 0)}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &imageTransferRuntimeStub{kind: tc.kind}
			var calls []string
			runner := func(_ context.Context, name string, args ...string) (string, error) {
				calls = append(calls, name+" "+strings.Join(args, " "))
				if name != tc.tool || len(args) != 5 || args[0] != "image" || args[1] != "inspect" || args[2] != "--format" {
					return "", fmt.Errorf("unexpected target inspect invocation: %s %v", name, args)
				}
				switch args[3] {
				case tc.fmt:
					return "registry.example.invalid/org/app@" + sha256a, nil
				case `{{.Architecture}}|{{.Os}}`:
					return "amd64|linux", nil
				default:
					return "", fmt.Errorf("unexpected target template %q", args[3])
				}
			}
			ref := "registry.example.invalid/org/app@" + sha256a
			target := RuntimeImageTransferTarget{Compose: runtime, Exec: runner}
			if err := target.Copy(context.Background(), ref); err != nil {
				t.Fatalf("target copy failed: %v", err)
			}
			if runtime.pulled != ref {
				t.Fatalf("runtime must pull immutable ref %q, got %q", ref, runtime.pulled)
			}
			identity, err := target.Verify(context.Background(), ref)
			if err != nil {
				t.Fatalf("target verify failed: %v; calls=%v", err, calls)
			}
			if identity.ImmutableIdentity() != ref || identity.Architecture != "amd64" || identity.OS != "linux" {
				t.Fatalf("unexpected verified identity: %+v", identity)
			}
		})
	}
}
