package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestResolveUsesPlainHTTPOnlyForLocalhostRegistry(t *testing.T) {
	digest := "sha256:" + strings.Repeat("3", 64)
	local := &fakeExec{output: []string{"Resolved localhost:5000/baseharbor-runtime@" + digest}}
	if _, err := (RegistrySource{
		Kind:      SourceOCIAddressed,
		Location:  "localhost:5000/baseharbor-runtime",
		Requested: "pre-release",
		Exec:      local.fn,
	}).Resolve(context.Background()); err != nil {
		t.Fatalf("resolve local HTTP registry: %v", err)
	}
	if got, want := local.calls[0], "oras resolve --full-reference --plain-http --resolve localhost:5000:127.0.0.1 localhost:5000/baseharbor-runtime:pre-release"; got != want {
		t.Fatalf("local registry resolve args = %q, want %q", got, want)
	}

	remote := &fakeExec{output: []string{"Resolved registry.example.invalid/org/app@" + digest}}
	if _, err := (RegistrySource{
		Kind:      SourceOCIAddressed,
		Location:  "registry.example.invalid/org/app",
		Requested: "stable",
		Exec:      remote.fn,
	}).Resolve(context.Background()); err != nil {
		t.Fatalf("resolve HTTPS registry: %v", err)
	}
	if got, want := remote.calls[0], "oras resolve --full-reference registry.example.invalid/org/app:stable"; got != want {
		t.Fatalf("remote registry resolve args = %q, want %q", got, want)
	}
}

func TestLocalRuntimeImageResolveOverridePinsLoopbackRegistry(t *testing.T) {
	cases := map[string]string{
		"localhost/team/app:dev":                   "localhost:80:127.0.0.1",
		"localhost:5000/team/app:pre-release":      "localhost:5000:127.0.0.1",
		"registry.example.invalid/team/app:stable": "",
		"127.0.0.1:5000/team/app:dev":              "",
	}
	for reference, want := range cases {
		t.Run(reference, func(t *testing.T) {
			if got := localRuntimeImageResolveOverride(reference); got != want {
				t.Fatalf("localRuntimeImageResolveOverride(%q)=%q want %q", reference, got, want)
			}
		})
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

func TestIsLocalRuntimeImageReference(t *testing.T) {
	cases := []struct {
		reference string
		want      bool
	}{
		{reference: "localhost/team/app:dev", want: true},
		{reference: "localhost:5000/team/app:pre-release", want: true},
		{reference: "registry.example.invalid/team/app:dev", want: false},
		{reference: "127.0.0.1:5000/team/app:dev", want: false},
		{reference: "localhost.invalid/team/app:dev", want: false},
		{reference: "localhost:invalid/team/app:dev", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.reference, func(t *testing.T) {
			if got := isLocalRuntimeImageReference(tc.reference); got != tc.want {
				t.Fatalf("isLocalRuntimeImageReference(%q)=%t want %t", tc.reference, got, tc.want)
			}
		})
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

func TestPodmanLocalRegistryConfigIsScopedAndCleanedUp(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source file")
	}
	script := filepath.Join(filepath.Dir(sourceFile), "..", "..", "scripts", "ci-local-oci-registry.sh")
	root := t.TempDir()
	home := filepath.Join(root, "home")
	fakeBin := filepath.Join(root, "bin")
	for _, dir := range []string{home, fakeBin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	commands := map[string]string{
		"podman": "#!/bin/sh\nexit 0\n",
		"curl":   "#!/bin/sh\nexit 0\n",
		"oras":   "#!/bin/sh\necho sha256:" + strings.Repeat("a", 64) + "\n",
	}
	for name, body := range commands {
		path := filepath.Join(fakeBin, name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	overrides := map[string]string{
		"BASEHARBOR_TEST_RUNTIME": "podman",
		"GITHUB_ENV":              filepath.Join(root, "github-env"),
		"GITHUB_STEP_SUMMARY":     filepath.Join(root, "github-summary"),
		"HOME":                    home,
		"PATH":                    fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			env = append(env, entry)
		}
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}

	run := func(args ...string) {
		t.Helper()
		commandArgs := append([]string{script}, args...)
		command := exec.Command("bash", commandArgs...)
		command.Env = env
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("ci-local-oci-registry.sh %v failed: %v\n%s", args, err, output)
		}
	}

	configDir := filepath.Join(home, ".config", "containers", "registries.conf.d")
	config := filepath.Join(configDir, "99-baseharbor-pre-release-registry.conf")
	run("publish", "localhost/baseharbor-runtime:pre-release", "localhost:5000/baseharbor-runtime:pre-release")

	got, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	want := "[[registry]]\nlocation = \"localhost:5000\"\ninsecure = true\n"
	if string(got) != want {
		t.Fatalf("Podman registry drop-in = %q, want only the localhost:5000 insecure registry entry %q", got, want)
	}
	entries, err := os.ReadDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(config) {
		t.Fatalf("unexpected Podman registry drop-ins: %v", entries)
	}

	run("cleanup")
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("Podman registry drop-in still exists after cleanup: stat error=%v", err)
	}
}
