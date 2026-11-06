package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
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
	runner    *fakeRunner
	pullErr   error
	pulledRef string
	// digest, manifestDigest, architecture are the values Verify reports.
	digest         string
	manifestDigest string
	architecture   string
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
		runner:       runner,
		digest:       sha256a,
		architecture: "amd64",
	}
	source := RuntimeImageTransferSource{Location: "registry.example.invalid/org/app:stable", Exec: runner.fn}

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
	target := &fakeRuntimeTarget{runner: runner, pullErr: errors.New("podman: pull: connection refused")}
	source := RuntimeImageTransferSource{Location: "registry.example.invalid/org/app", Exec: runner.fn}

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
		runner:       runner,
		digest:       sha256b, // target reports sha256b
		architecture: "amd64",
	}
	source := RuntimeImageTransferSource{
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
		runner:       runner,
		digest:       sha256a,
		architecture: "arm64", // target reports arm64
	}
	source := RuntimeImageTransferSource{
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
	target := &fakeRuntimeTarget{runner: runner, digest: sha256a, architecture: "amd64", verifyUnavail: true}
	source := RuntimeImageTransferSource{Location: "registry.example.invalid/org/app", Exec: runner.fn}

	if _, err := ImageTransfer(context.Background(), source, target); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("unverifiable transfer must fail closed, got %v", err)
	}
}

func TestImageTransferSourceFailsClosedOnPublicDefault(t *testing.T) {
	// No-copy cell: a public-default source must be rejected before any copy, so
	// a copy can never silently fall back to docker.io.
	runner := &fakeRunner{}
	source := RuntimeImageTransferSource{Location: "docker.io/library/postgres:15", Exec: runner.fn}

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
	source := RuntimeImageTransferSource{Location: "registry.example.invalid/org/app", Exec: runner.fn}
	runner.output = append(runner.output, "registry.example.invalid/orgapp") // no digest

	if _, err := source.Resolve(context.Background()); !errors.Is(err, ErrImageTransferUnavailable) {
		t.Fatalf("mutable-tag source identity must fail closed, got %v", err)
	}
}

func TestImageTransferSourceFailsClosedOnUnreachableHost(t *testing.T) {
	runner := &fakeRunner{err: errors.New("podman: inspect: lookup failed: no such host")}
	source := RuntimeImageTransferSource{Location: "registry.example.invalid/org/app", Exec: runner.fn}

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
	if err := id.Verify(sha256a, sha256b, "amd64"); err != nil {
		t.Fatalf("matching identity must verify: %v", err)
	}
	if err := id.Verify(sha256b, sha256b, "amd64"); err == nil {
		t.Fatal("digest mismatch must fail Verify")
	}
	if err := id.Verify(sha256a, sha256a, "amd64"); err == nil {
		t.Fatal("manifest digest mismatch must fail Verify")
	}
	if err := id.Verify(sha256a, sha256b, "arm64"); err == nil {
		t.Fatal("architecture mismatch must fail Verify")
	}
	if err := id.Verify("", sha256b, "amd64"); err == nil {
		t.Fatal("missing digest must fail Verify")
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
