package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeExec records each invocation and returns queued outputs. It models the
// native artifact tool (oras) that reuses the host credential helper / auth
// file transparently: the caller only observes whether resolution succeeded,
// never any credential material.
type fakeExec struct {
	calls  []string
	output []string
	err    error
}

func (f *fakeExec) fn(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if len(f.output) > 0 {
		out := f.output[0]
		f.output = f.output[1:]
		return out, f.err
	}
	return "", f.err
}

func TestResolveReadsEndpointFromConfigNotHardcoded(t *testing.T) {
	// Red-first guarantee for the configured-endpoint cell: the endpoint comes
	// from the configured Location, never a hardcoded host. Two different
	// locations must resolve two different endpoints.
	digest := "sha256:" + strings.Repeat("a", 64)
	enterprise := "registry.example.invalid/org/app"
	other := "harbor.corp.internal/team/svc"

	fake1 := &fakeExec{output: []string{"Resolved " + enterprise + "@" + digest}}
	id1, err := (RegistrySource{
		Kind:      SourceOCIAddressed,
		Location:  enterprise,
		Requested: "stable",
		Exec:      fake1.fn,
	}).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id1.Repository != "registry.example.invalid/org/app" {
		t.Fatalf("endpoint not taken from configuration: got %q", id1.Repository)
	}
	if id1.ImmutableIdentity() != "registry.example.invalid/org/app@sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("unexpected immutable identity: %q", id1.ImmutableIdentity())
	}

	fake2 := &fakeExec{output: []string{"Resolved " + other + "@" + digest}}
	id2, err := (RegistrySource{
		Kind:      SourceOCIAddressed,
		Location:  other,
		Requested: "stable",
		Exec:      fake2.fn,
	}).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id2.Repository == id1.Repository {
		t.Fatalf("configured endpoint must drive resolution: both resolved to %q", id1.Repository)
	}
}

func TestResolvePinsImmutableDigestFromNativeTool(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	fake := &fakeExec{output: []string{"Resolved registry.example.invalid/org/app@" + digest}}
	id, err := RegistrySource{
		Kind:      SourceOCIAddressed,
		Location:  "registry.example.invalid/org/app",
		Requested: "stable",
		Exec:      fake.fn,
	}.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id.Digest != digest {
		t.Fatalf("resolved digest=%q want %q", id.Digest, digest)
	}
	if id.Requested != "stable" {
		t.Fatalf("requested channel was not retained: %+v", id)
	}
	if id.ImmutableIdentity() != "registry.example.invalid/org/app@"+digest {
		t.Fatalf("immutable identity must be repository@digest: %q", id.ImmutableIdentity())
	}
}

func TestResolveDefaultsRequestToLatestWhenUnset(t *testing.T) {
	digest := "sha256:" + strings.Repeat("0", 64)
	fake := &fakeExec{output: []string{"Resolved registry.example.invalid/org/app@" + digest}}
	if _, err := (RegistrySource{
		Kind:     SourceOCIAddressed,
		Location: "registry.example.invalid/org/app",
		Exec:     fake.fn,
	}).Resolve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) == 0 || !strings.Contains(fake.calls[0], "registry.example.invalid/org/app:latest") {
		t.Fatalf("empty requested must default to :latest, calls=%v", fake.calls)
	}
}

func TestResolveReusesNativeCredentialHelperAndNeverClonesToken(t *testing.T) {
	// Native-credential cell: the adapter addresses the host through the native
	// tool, which reuses the configured Docker credential helper / Podman auth.
	// It never clones a token into its own file and never surfaces a secret.
	digest := "sha256:" + strings.Repeat("1", 64)
	fake := &fakeExec{output: []string{"Resolved registry.example.invalid/org/app@" + digest}}
	_, err := RegistrySource{
		Kind:      SourceOCIAddressed,
		Location:  "registry.example.invalid/org/app",
		Requested: "stable",
		Exec:      fake.fn,
	}.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The only native call is the resolve action against the configured host;
	// no file write, no token cloning.
	if len(fake.calls) != 1 || !strings.HasPrefix(fake.calls[0], "oras resolve") {
		t.Fatalf("expected a single native resolve call, got %v", fake.calls)
	}
}

func TestResolveFailsClosedWhenEndpointUnreachable(t *testing.T) {
	fake := &fakeExec{err: errors.New("oras: resolve: lookup failed: no such host")}
	if _, err := (RegistrySource{
		Kind:     SourceOCIAddressed,
		Location: "registry.example.invalid/org/app",
		Exec:     fake.fn,
	}).Resolve(context.Background()); !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("unreachable endpoint must fail closed: got %v", err)
	}
}

func TestResolveFailsClosedOnEmptyReply(t *testing.T) {
	fake := &fakeExec{output: []string{"   "}}
	if _, err := (RegistrySource{
		Kind:     SourceOCIAddressed,
		Location: "registry.example.invalid/org/app",
		Exec:     fake.fn,
	}).Resolve(context.Background()); !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("empty resolve reply must fail closed: got %v", err)
	}
}

func TestResolveFailsClosedOnMutableTagIdentity(t *testing.T) {
	// The resolved reference must carry an immutable sha256 digest identity.
	// A mutable tag (no digest) is not an acceptable resolved identity.
	fake := &fakeExec{output: []string{"Resolved registry.example.invalid/org/app:stable"}}
	if _, err := (RegistrySource{
		Kind:     SourceOCIAddressed,
		Location: "registry.example.invalid/org/app",
		Exec:     fake.fn,
	}).Resolve(context.Background()); !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("mutable tag identity must fail closed: got %v", err)
	}
}

func TestResolveSucceedsWhenReferenceCarriesPinnedDigest(t *testing.T) {
	// ORAS --full-reference emits the complete immutable repository identity.
	fake := &fakeExec{output: []string{"registry.example.invalid/org/app@sha256:" + strings.Repeat("2", 64) + "\n"}}
	id, err := (RegistrySource{
		Kind:     SourceOCIAddressed,
		Location: "registry.example.invalid/org/app",
		Exec:     fake.fn,
	}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("pinned digest must resolve: got %v", err)
	}
	if id.Digest != "sha256:"+strings.Repeat("2", 64) {
		t.Fatalf("resolved digest=%q", id.Digest)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "oras resolve --full-reference registry.example.invalid/org/app:latest" {
		t.Fatalf("resolve must request ORAS's full repository reference, got %v", fake.calls)
	}
}

func TestResolveFailsClosedWhenReplyLacksDigest(t *testing.T) {
	// A resolve reply that does not carry an immutable sha256 digest at all is
	// rejected rather than accepted as a mutable identity.
	fake := &fakeExec{output: []string{"Resolved registry.example.invalid/org/app:stable"}}
	if _, err := (RegistrySource{
		Kind:     SourceOCIAddressed,
		Location: "registry.example.invalid/org/app",
		Exec:     fake.fn,
	}).Resolve(context.Background()); !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("reply without digest must fail closed: got %v", err)
	}
}

func TestResolveRejectsEmptyEndpoint(t *testing.T) {
	if _, err := (RegistrySource{
		Kind:     SourceOCIAddressed,
		Location: "  ",
		Exec:     (&fakeExec{}).fn,
	}).Resolve(context.Background()); err != ErrRegistrySourceEmpty {
		t.Fatalf("empty endpoint must fail closed: got %v", err)
	}
}

func TestResolveRejectsPublicDockerDefault(t *testing.T) {
	// No-fallback cell: the adapter refuses to treat a public docker.io default
	// as an enterprise endpoint, so a missing or denied endpoint cannot silently
	// become docker.io. This must start RED: a hardcoded/default fallback makes
	// this test fail.
	digest := "sha256:" + strings.Repeat("3", 64)
	for _, location := range []string{
		"docker.io/library/postgres",
		"index.docker.io",
		"registry-1.docker.io/app",
	} {
		fake := &fakeExec{output: []string{"Resolved " + location + "@" + digest}}
		if _, err := (RegistrySource{
			Kind:      SourceOCIAddressed,
			Location:  location,
			Requested: "15",
			Exec:      fake.fn,
		}).Resolve(context.Background()); !errors.Is(err, ErrRegistrySourceUnavailable) {
			t.Fatalf("public default %q must fail closed: got %v", location, err)
		}
	}
}

func TestCanonicalArtifactRepositoryStripsTagButKeepsHost(t *testing.T) {
	cases := map[string]string{
		"registry.example.invalid/org/app@sha256:" + strings.Repeat("a", 64): "registry.example.invalid/org/app",
		"registry.example.invalid/org/app:stable":                            "registry.example.invalid/org/app",
		"localhost:5000/team/svc":                                            "localhost:5000/team/svc",
	}
	for in, want := range cases {
		if got := canonicalArtifactRepository(in); got != want {
			t.Fatalf("canonicalArtifactRepository(%q)=%q want %q", in, got, want)
		}
	}
}

func TestExecArtifactRedactsExitErrorStderr(t *testing.T) {
	// A real failing native command must be reported with only a redacted class
	// and exit status, never its raw stderr.
	out, err := execArtifact(context.Background(), "sh", "-ec", "echo secret-token leaked >&2; exit 7")
	_ = out
	if err == nil {
		t.Fatal("expected a runner error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatal("stderr secret leaked through execArtifact")
	}
}
