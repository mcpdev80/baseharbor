package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// artifactResolutionDigest builds a syntactically valid sha256 digest from a
// short label so tests can pin distinct digests deterministically.
func artifactResolutionDigest(label string) string {
	digits := "0123456789abcdef"
	var body strings.Builder
	for i := 0; i < 64; i++ {
		body.WriteByte(digits[(int(label[i%len(label)])+i)%16])
	}
	return "sha256:" + body.String()
}

const artifactRepo = "registry.example.invalid/org/app"

var (
	artifactDigestA = artifactResolutionDigest("a")
	artifactDigestB = artifactResolutionDigest("b")
)

// renderedArtifactService is one service entry of the rendered-Compose fixture.
type renderedArtifactService struct {
	Image string
	Build string // raw JSON for the build stanza; "" means none
}

func imageOnlyArtifact(image string) renderedArtifactService {
	return renderedArtifactService{Image: image}
}

func buildArtifact(contextDir, dockerfile string) renderedArtifactService {
	return renderedArtifactService{Build: fmt.Sprintf(`{"context":%q,"dockerfile":%q}`, contextDir, dockerfile)}
}

func buildWithImage(contextDir, image string) renderedArtifactService {
	return renderedArtifactService{Image: image, Build: fmt.Sprintf(`{"context":%q}`, contextDir)}
}

// renderedArtifact builds the rendered-Compose fixture for a set of services.
func renderedArtifact(services map[string]renderedArtifactService) string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	var builder strings.Builder
	builder.WriteString(`{"services":{`)
	for i, name := range names {
		if i > 0 {
			builder.WriteString(",")
		}
		entry := services[name]
		fmt.Fprintf(&builder, "%q:{", name)
		shown := false
		if entry.Image != "" {
			fmt.Fprintf(&builder, "%q:%q", "image", entry.Image)
			shown = true
		}
		if entry.Build != "" {
			if shown {
				builder.WriteString(",")
			}
			fmt.Fprintf(&builder, "%q:%s", "build", entry.Build)
		}
		builder.WriteString("}")
	}
	builder.WriteString("}}")
	out := builder.String()
	if !json.Valid([]byte(out)) {
		panic("fixture is not valid JSON: " + out)
	}
	return out
}

func artifactServiceNames(services map[string]renderedArtifactService) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// scriptedRunner records each native invocation. The registry seam
// (resolveRegistrySource) is driven by the same script: a "resolve" call that
// is not an oras call consults s.resolve; the native image tool answers
// `exists` and `inspect` from the local store script.
type scriptedRunner struct {
	calls   []string
	resolve map[string]string
	deny    map[string]error
	store   map[string]string // ref -> digest (comma-separated for a conflicted store); absent means missing
}

func (s *scriptedRunner) fn(_ context.Context, name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	s.calls = append(s.calls, call)
	if name == "oras" {
		if err, ok := s.deny[call]; ok {
			return "", err
		}
		if out, ok := s.resolve[call]; ok {
			return out, nil
		}
		return "", errors.New("oras: command failed")
	}
	ref := args[len(args)-1]
	// A non-oras "resolve" verb is the seam probe installed by
	// withArtifactRegistry. Scripts are keyed by the full call string or by
	// the bare reference.
	if name == "resolve" {
		ref := args[len(args)-1]
		for _, key := range []string{call, "resolve " + ref, ref, strings.TrimPrefix(ref, artifactRepo+":"), strings.TrimPrefix(ref, artifactRepo)} {
			if key == "" {
				continue
			}
			if err, ok := s.deny[key]; ok {
				return "", err
			}
			if out, ok := s.resolve[key]; ok {
				return out, nil
			}
		}
		return "", errors.New("resolve: endpoint unreachable")
	}
	switch {
	case strings.Contains(call, "image exists"):
		if digest, ok := s.store[ref]; ok && digest != "" {
			return "", nil
		}
		return "", errors.New(name + ": image not known to local store")
	case strings.Contains(call, "image inspect"):
		if strings.Contains(call, "Architecture") {
			if _, ok := s.store[ref]; ok {
				return "amd64|linux", nil
			}
			return "", errors.New(name + ": no such image")
		}
		digest, ok := s.store[ref]
		if !ok || digest == "" {
			return "", errors.New(name + ": no such image")
		}
		if strings.Contains(call, "range") {
			// The probe format: one repository@digest line per repo digest.
			var lines strings.Builder
			for _, d := range strings.Split(digest, ",") {
				lines.WriteString(imageArtifactRepository(ref) + "@" + d + "\n")
			}
			return lines.String(), nil
		}
		return imageArtifactRepository(ref) + "@" + strings.Split(digest, ",")[0], nil
	}
	return "", nil
}

func (s *scriptedRunner) mustHaveCall(t *testing.T, needle string) {
	t.Helper()
	for _, call := range s.calls {
		if strings.Contains(call, needle) {
			return
		}
	}
	t.Fatalf("no recorded call contains %q; calls: %v", needle, s.calls)
}

func (s *scriptedRunner) mustNotHaveCall(t *testing.T, needles ...string) {
	t.Helper()
	for _, call := range s.calls {
		for _, needle := range needles {
			if strings.Contains(call, needle) {
				t.Fatalf("call %q must not have happened (matched %q); calls: %v", call, needle, s.calls)
			}
		}
	}
}

func dockerArtifactCLI() imageTransferCLI {
	cli, err := imageTransferCLIFor("docker")
	if err != nil {
		panic(err)
	}
	return cli
}

func withArtifactRegistry(t *testing.T, runner *scriptedRunner) {
	t.Helper()
	previous := registrySourceExec
	registrySourceExec = func(ctx context.Context, name string, args ...string) (string, error) {
		if name == "oras" && len(args) > 0 && args[0] == "resolve" {
			return runner.fn(ctx, "resolve", args[len(args)-1])
		}
		return runner.fn(ctx, name, args...)
	}
	t.Cleanup(func() { registrySourceExec = previous })
}

func TestArtifactResolutionSourceLocation(t *testing.T) {
	cases := []struct {
		image, location, requested, digest string
	}{
		{"nginx:1.27", "nginx", "1.27", ""},
		{artifactRepo + ":stable", artifactRepo, "stable", ""},
		{artifactRepo + "@" + artifactDigestA, artifactRepo, "", artifactDigestA},
		{artifactRepo + ":stable@" + artifactDigestB, artifactRepo, "stable", artifactDigestB},
		{"redis", "redis", "", ""},
		{"  ", "", "", ""},
	}
	for _, tc := range cases {
		location, requested, digest := artifactSourceLocation(tc.image)
		if location != tc.location || requested != tc.requested || digest != tc.digest {
			t.Errorf("artifactSourceLocation(%q) = %q,%q,%q want %q,%q,%q", tc.image, location, requested, digest, tc.location, tc.requested, tc.digest)
		}
	}
}

func TestSplitArtifactResolutionPin(t *testing.T) {
	repository, digest := splitDigestReference(artifactRepo + "@" + artifactDigestA)
	if repository != artifactRepo || digest != artifactDigestA {
		t.Fatalf("split = %q,%q", repository, digest)
	}
}

func TestNativeArtifactImageTool(t *testing.T) {
	docker, err := imageTransferCLIFor("docker")
	if err != nil {
		t.Fatal(err)
	}
	if docker.name != "docker" || docker.repoDigestFormat != `{{index .RepoDigests 0}}` {
		t.Fatalf("unexpected docker CLI: %+v", docker)
	}
	podman, err := imageTransferCLIFor("podman")
	if err != nil {
		t.Fatal(err)
	}
	if podman.name != "podman" || podman.repoDigestFormat != `{{(index .RepoDigests 0)}}` {
		t.Fatalf("unexpected podman CLI: %+v", podman)
	}
	if _, err := imageTransferCLIFor("containerd"); err == nil {
		t.Fatal("unsupported runtime must fail closed")
	}
}

func TestResolveArtifactServiceDecisionMatrix(t *testing.T) {
	digestIdentity := ArtifactIdentity{Repository: artifactRepo, Digest: artifactDigestA, Requested: "stable"}
	mutableIdentity := ArtifactIdentity{Repository: artifactRepo, Requested: "stable"}
	cases := []struct {
		name          string
		mode          ArtifactResolutionMode
		declaredBuild bool
		resolved      ArtifactIdentity
		want          ArtifactResolutionAction
		wantErr       error
	}{
		{"existing with digest", ArtifactResolutionExisting, false, digestIdentity, ArtifactActionUseExisting, nil},
		{"existing with build and digest stays existing", ArtifactResolutionExisting, true, digestIdentity, ArtifactActionUseExisting, nil},
		{"existing without digest", ArtifactResolutionExisting, false, mutableIdentity, "", ErrArtifactResolutionUnavailable},
		{"existing pure build", ArtifactResolutionExisting, true, ArtifactIdentity{}, "", ErrArtifactResolutionUnavailable},
		{"checkout build routes native", ArtifactResolutionCheckoutBuild, true, ArtifactIdentity{}, ArtifactActionNativeBuild, nil},
		{"checkout image resolves existing", ArtifactResolutionCheckoutBuild, false, digestIdentity, ArtifactActionUseExisting, nil},
		{"checkout nothing declared", ArtifactResolutionCheckoutBuild, false, ArtifactIdentity{}, "", ErrArtifactResolutionUnavailable},
		{"unknown mode", ArtifactResolutionMode("guess"), false, digestIdentity, "", ErrArtifactResolutionAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveArtifactServiceDecision(tc.mode, tc.declaredBuild, tc.resolved)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("action = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveLocalRegistryArtifact(t *testing.T) {
	t.Run("carries immutable digest", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{artifactRepo + ":stable": artifactDigestA}}
		identity, err := resolveLocalRegistryArtifact(context.Background(), runner.fn, dockerArtifactCLI(), artifactRepo+":stable")
		if err != nil {
			t.Fatal(err)
		}
		if identity.Repository != artifactRepo || identity.Digest != artifactDigestA {
			t.Fatalf("identity = %+v", identity)
		}
		if identity.ImmutableIdentity() != artifactRepo+"@"+artifactDigestA {
			t.Fatalf("immutable identity = %q", identity.ImmutableIdentity())
		}
		runner.mustHaveCall(t, "docker image inspect")
	})
	t.Run("missing image fails closed", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{}}
		_, err := resolveLocalRegistryArtifact(context.Background(), runner.fn, dockerArtifactCLI(), artifactRepo+":stable")
		if !errors.Is(err, ErrArtifactResolutionUnavailable) {
			t.Fatalf("err = %v, want unavailable sentinel", err)
		}
	})
	t.Run("mutable tag reply is not an identity", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{artifactRepo + ":stable": "@"}}
		_, err := resolveLocalRegistryArtifact(context.Background(), runner.fn, dockerArtifactCLI(), artifactRepo+":stable")
		if !errors.Is(err, ErrArtifactResolutionUnavailable) {
			t.Fatalf("err = %v, want unavailable sentinel", err)
		}
	})
}

func TestLocalArtifactAvailable(t *testing.T) {
	t.Run("present image with digest", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{artifactRepo + ":stable": artifactDigestA}}
		available, err := localArtifactAvailable(context.Background(), runner.fn, dockerArtifactCLI(), artifactRepo+":stable")
		if err != nil || !available {
			t.Fatalf("available = %v, err = %v", available, err)
		}
		runner.mustHaveCall(t, "docker image exists")
	})
	t.Run("absent image", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{}}
		available, err := localArtifactAvailable(context.Background(), runner.fn, dockerArtifactCLI(), artifactRepo+":stable")
		if err != nil || available {
			t.Fatalf("available = %v, err = %v, want false,nil", available, err)
		}
	})
	t.Run("conflicting digests are ambiguous", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{artifactRepo + ":stable": artifactDigestA}}
		exec := func(ctx context.Context, name string, args ...string) (string, error) {
			out, err := runner.fn(ctx, name, args...)
			if strings.Contains(strings.Join(args, " "), "range") {
				// append a second conflicting digest to the probe reply
				out += artifactRepo + "@" + artifactDigestB + "\n"
			}
			return out, err
		}
		_, err := localArtifactAvailable(context.Background(), exec, dockerArtifactCLI(), artifactRepo+":stable")
		if !errors.Is(err, ErrArtifactResolutionAmbiguous) {
			t.Fatalf("err = %v, want ambiguity sentinel", err)
		}
	})
	t.Run("exists but uninspectable is absent", func(t *testing.T) {
		exec := func(_ context.Context, name string, args ...string) (string, error) {
			if strings.Contains(strings.Join(args, " "), "image exists") {
				return "", nil
			}
			return "", errors.New(name + ": no such image")
		}
		available, err := localArtifactAvailable(context.Background(), exec, dockerArtifactCLI(), artifactRepo+":stable")
		if err != nil || available {
			t.Fatalf("available = %v, err = %v, want false,nil", available, err)
		}
	})
}

func TestResolveRenderedWorkloadArtifactsExistingMode(t *testing.T) {
	model := map[string]renderedArtifactService{
		"api": imageOnlyArtifact(artifactRepo + ":stable"),
		"web": imageOnlyArtifact("ghcr.io/org/web@" + artifactDigestB),
	}
	rendered := renderedArtifact(model)
	t.Run("registry first, no local probe", func(t *testing.T) {
		runner := &scriptedRunner{
			resolve: map[string]string{"resolve " + artifactRepo + ":stable": artifactRepo + "@" + artifactDigestA},
		}
		withArtifactRegistry(t, runner)
		decisions, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(), rendered, artifactServiceNames(model), ArtifactResolutionExisting)
		if err != nil {
			t.Fatal(err)
		}
		if decisions["api"].Action != ArtifactActionUseExisting || decisions["api"].Identity.Digest != artifactDigestA {
			t.Fatalf("api decision = %+v", decisions["api"])
		}
		if decisions["web"].Action != ArtifactActionUseExisting || decisions["web"].Identity.Digest != artifactDigestB {
			t.Fatalf("web decision = %+v", decisions["web"])
		}
		runner.mustHaveCall(t, "resolve "+artifactRepo+":stable")
		runner.mustNotHaveCall(t, "image exists", "image inspect")
	})
	t.Run("registry unavailable falls to local store, build stays untouched", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{artifactRepo + ":stable": artifactDigestA}}
		withArtifactRegistry(t, runner)
		decisions, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": buildWithImage(".", artifactRepo+":stable")}),
			[]string{"api"}, ArtifactResolutionExisting)
		if err != nil {
			t.Fatal(err)
		}
		if decisions["api"].Action != ArtifactActionUseExisting || decisions["api"].Identity.Digest != artifactDigestA {
			t.Fatalf("existing image must win over the declared build: %+v", decisions["api"])
		}
		runner.mustHaveCall(t, "image exists")
	})
	t.Run("neither registry nor store fails closed", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{}}
		withArtifactRegistry(t, runner)
		_, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(), rendered, []string{"api"}, ArtifactResolutionExisting)
		if !errors.Is(err, ErrArtifactResolutionUnavailable) {
			t.Fatalf("err = %v, want unavailable sentinel", err)
		}
	})
	t.Run("denied endpoint is never papered over", func(t *testing.T) {
		runner := &scriptedRunner{
			deny:  map[string]error{"resolve " + artifactRepo + ":stable": errors.New("oras: denied: unauthorized")},
			store: map[string]string{artifactRepo + ":stable": artifactDigestA},
		}
		withArtifactRegistry(t, runner)
		_, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(), rendered, []string{"api"}, ArtifactResolutionExisting)
		if !errors.Is(err, ErrRegistrySourceUnavailable) || !errors.Is(err, errClassDenied) {
			t.Fatalf("err = %v, want denial class", err)
		}
		runner.mustNotHaveCall(t, "image exists")
	})
	t.Run("public default fails closed", func(t *testing.T) {
		runner := &scriptedRunner{}
		withArtifactRegistry(t, runner)
		_, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": imageOnlyArtifact("docker.io/library/nginx:1.27")}),
			[]string{"api"}, ArtifactResolutionExisting)
		if !errors.Is(err, ErrRegistrySourcePublicDefault) {
			t.Fatalf("err = %v, want public-default rejection", err)
		}
	})
	t.Run("checkout build in existing mode is rejected", func(t *testing.T) {
		runner := &scriptedRunner{}
		withArtifactRegistry(t, runner)
		_, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": buildArtifact(".", "Dockerfile")}),
			[]string{"api"}, ArtifactResolutionExisting)
		if !errors.Is(err, ErrArtifactResolutionUnavailable) {
			t.Fatalf("err = %v, want unavailable sentinel", err)
		}
	})
	t.Run("missing service in model is ambiguous", func(t *testing.T) {
		runner := &scriptedRunner{resolve: map[string]string{"resolve " + artifactRepo + ":stable": artifactRepo + "@" + artifactDigestA, "resolve ghcr.io/org/web:latest": "ghcr.io/org/web@" + artifactDigestB}}
		withArtifactRegistry(t, runner)
		_, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(), rendered, []string{"api", "ghost"}, ArtifactResolutionExisting)
		if !errors.Is(err, ErrArtifactResolutionAmbiguous) {
			t.Fatalf("err = %v, want ambiguity sentinel", err)
		}
	})
}

func TestResolveRenderedWorkloadArtifactsCheckoutMode(t *testing.T) {
	t.Run("pure build routes native", func(t *testing.T) {
		runner := &scriptedRunner{}
		withArtifactRegistry(t, runner)
		decisions, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": buildArtifact(".", "Dockerfile")}),
			[]string{"api"}, ArtifactResolutionCheckoutBuild)
		if err != nil {
			t.Fatal(err)
		}
		if decisions["api"].Action != ArtifactActionNativeBuild {
			t.Fatalf("decision = %+v", decisions["api"])
		}
	})
	t.Run("image with build still prefers available artifact", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{artifactRepo + ":stable": artifactDigestA}}
		withArtifactRegistry(t, runner)
		decisions, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": buildWithImage(".", artifactRepo+":stable")}),
			[]string{"api"}, ArtifactResolutionCheckoutBuild)
		if err != nil {
			t.Fatal(err)
		}
		if decisions["api"].Action != ArtifactActionUseExisting || decisions["api"].Identity.Digest != artifactDigestA {
			t.Fatalf("an available existing image must win over the checkout build: %+v", decisions["api"])
		}
	})
	t.Run("unavailable image with build routes native", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{}}
		withArtifactRegistry(t, runner)
		decisions, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": buildWithImage(".", artifactRepo+":stable")}),
			[]string{"api"}, ArtifactResolutionCheckoutBuild)
		if err != nil {
			t.Fatal(err)
		}
		if decisions["api"].Action != ArtifactActionNativeBuild {
			t.Fatalf("decision = %+v", decisions["api"])
		}
	})
	t.Run("bare local image name probes the store without the registry", func(t *testing.T) {
		runner := &scriptedRunner{store: map[string]string{"local-tool:1.0": artifactDigestA}}
		withArtifactRegistry(t, runner)
		decisions, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"tool": imageOnlyArtifact("local-tool:1.0")}),
			[]string{"tool"}, ArtifactResolutionCheckoutBuild)
		if err != nil {
			t.Fatal(err)
		}
		if decisions["tool"].Identity.Digest != artifactDigestA || decisions["tool"].Action != ArtifactActionUseExisting {
			t.Fatalf("decision = %+v", decisions["tool"])
		}
		runner.mustHaveCall(t, "docker image exists local-tool:1.0")
	})
	t.Run("imageless buildless service is ambiguous", func(t *testing.T) {
		runner := &scriptedRunner{}
		withArtifactRegistry(t, runner)
		_, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": {}}),
			[]string{"api"}, ArtifactResolutionCheckoutBuild)
		if !errors.Is(err, ErrArtifactResolutionAmbiguous) {
			t.Fatalf("err = %v, want ambiguity sentinel", err)
		}
	})
}

func TestResolveWorkloadArtifact(t *testing.T) {
	t.Run("registry resolve returns the immutable artifact", func(t *testing.T) {
		runner := &scriptedRunner{resolve: map[string]string{"resolve " + artifactRepo + ":stable": artifactRepo + "@" + artifactDigestA}}
		withArtifactRegistry(t, runner)
		identity, err := ResolveWorkloadArtifact(context.Background(), artifactRepo+":stable", "stable")
		if err != nil {
			t.Fatal(err)
		}
		if identity.ImmutableIdentity() != artifactRepo+"@"+artifactDigestA {
			t.Fatalf("identity = %q", identity.ImmutableIdentity())
		}
	})
	t.Run("unresolved endpoint fails closed", func(t *testing.T) {
		runner := &scriptedRunner{}
		withArtifactRegistry(t, runner)
		_, err := ResolveWorkloadArtifact(context.Background(), artifactRepo+":stable", "stable")
		if !errors.Is(err, ErrRegistrySourceUnavailable) {
			t.Fatalf("err = %v, want registry-unavailable sentinel", err)
		}
	})
}

type pullRecordingRuntime struct {
	bhruntime.RuntimeProvider
	pulls []string
}

func (p *pullRecordingRuntime) PullImage(_ context.Context, image string) error {
	p.pulls = append(p.pulls, image)
	return nil
}

func (p *pullRecordingRuntime) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }

func TestResolveAndPullWorkloadArtifact(t *testing.T) {
	t.Run("pulls exactly the immutable pin", func(t *testing.T) {
		runner := &scriptedRunner{resolve: map[string]string{"resolve " + artifactRepo + ":edge": artifactRepo + "@" + artifactDigestA}}
		withArtifactRegistry(t, runner)
		compose := &pullRecordingRuntime{}
		identity, err := ResolveAndPullWorkloadArtifact(context.Background(), compose, artifactRepo+":edge", "edge")
		if err != nil {
			t.Fatal(err)
		}
		if len(compose.pulls) != 1 || compose.pulls[0] != artifactRepo+"@"+artifactDigestA {
			t.Fatalf("pulls = %v, want exactly the immutable pin", compose.pulls)
		}
		if identity.Digest != artifactDigestA {
			t.Fatalf("identity = %+v", identity)
		}
	})
	t.Run("resolution failure means zero pulls", func(t *testing.T) {
		runner := &scriptedRunner{}
		withArtifactRegistry(t, runner)
		compose := &pullRecordingRuntime{}
		_, err := ResolveAndPullWorkloadArtifact(context.Background(), compose, artifactRepo+":edge", "edge")
		if err == nil || !errors.Is(err, ErrRegistrySourceUnavailable) {
			t.Fatalf("err = %v, fail-closed expected", err)
		}
		if len(compose.pulls) != 0 {
			t.Fatalf("a failed resolve must pull nothing, pulls = %v", compose.pulls)
		}
	})
	t.Run("development opt-in pulls the tag only for unavailable class", func(t *testing.T) {
		t.Setenv(envAllowMutableDevelopmentRuntimeImage, "1")
		runner := &scriptedRunner{}
		withArtifactRegistry(t, runner)
		compose := &pullRecordingRuntime{}
		if _, err := ResolveAndPullWorkloadArtifact(context.Background(), compose, artifactRepo+":edge", "edge"); err != nil {
			t.Fatal(err)
		}
		if len(compose.pulls) != 1 || compose.pulls[0] != artifactRepo+":edge" {
			t.Fatalf("opt-in pulls = %v", compose.pulls)
		}
	})
	t.Run("development opt-in cannot bypass a denial", func(t *testing.T) {
		t.Setenv(envAllowMutableDevelopmentRuntimeImage, "1")
		runner := &scriptedRunner{deny: map[string]error{"resolve " + artifactRepo + ":edge": errors.New("oras: denied: unauthorized")}}
		withArtifactRegistry(t, runner)
		compose := &pullRecordingRuntime{}
		if _, err := ResolveAndPullWorkloadArtifact(context.Background(), compose, artifactRepo+":edge", "edge"); err == nil {
			t.Fatal("a denied endpoint must fail closed even behind the opt-in")
		}
		if len(compose.pulls) != 0 {
			t.Fatalf("denial must pull nothing, pulls = %v", compose.pulls)
		}
	})
}

func TestRepositoryWorkloadArtifactStatePersistence(t *testing.T) {
	files := application.RuntimeFiles{Dir: t.TempDir()}
	secret := "dont-persist-this-registry-token"
	plan := map[string]ArtifactResolution{
		"api": {
			Service:   "api",
			Action:    ArtifactActionUseExisting,
			Identity:  ArtifactIdentity{Repository: artifactRepo, Digest: artifactDigestA, Requested: secret},
			Requested: "stable",
		},
		"worker": {Service: "worker", Action: ArtifactActionNativeBuild},
	}
	if err := persistRepositoryWorkloadArtifactState(files, plan); err != nil {
		t.Fatal(err)
	}
	path := repositoryWorkloadArtifactStatePath(files)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact state mode = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The record retains only the needed owner/provenance metadata: the
	// immutable identity plus the requested channel. The Requested field of the
	// source identity (which could carry an endpoint secret) is never written.
	if strings.Contains(string(data), secret) {
		t.Fatalf("artifact state persisted a secret value: %s", data)
	}
	state, err := loadRepositoryWorkloadArtifactState(files)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 1 {
		t.Fatalf("state version = %d", state.Version)
	}
	if state.Services["api"].Identity != artifactRepo+"@"+artifactDigestA || state.Services["api"].Action != string(ArtifactActionUseExisting) {
		t.Fatalf("api entry = %+v", state.Services["api"])
	}
	if state.Services["worker"].Identity != "" || state.Services["worker"].Action != string(ArtifactActionNativeBuild) {
		t.Fatalf("worker entry = %+v", state.Services["worker"])
	}
}

func TestLoadRepositoryWorkloadArtifactStateAbsentFile(t *testing.T) {
	state, err := loadRepositoryWorkloadArtifactState(application.RuntimeFiles{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 1 || len(state.Services) != 0 {
		t.Fatalf("absent state = %+v", state)
	}
}

func TestValidateWorkloadArtifactBuildContext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "service"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "service", "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workload := application.WorkloadFiles{RepositoryRoot: root}
	if err := validateWorkloadArtifactBuildContext(workload, "api", json.RawMessage(`{"context":"./service","dockerfile":"Dockerfile"}`)); err != nil {
		t.Fatalf("valid build context rejected: %v", err)
	}
	if err := validateWorkloadArtifactBuildContext(workload, "api", json.RawMessage(`{"context":"../outside"}`)); err == nil {
		t.Fatal("a build context outside the repository must fail closed")
	} else if !strings.Contains(err.Error(), "escapes") && !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("unexpected escape error: %v", err)
	}
	if err := validateWorkloadArtifactBuildContext(workload, "api", nil); !errors.Is(err, ErrArtifactResolutionAmbiguous) {
		t.Fatalf("no build definition err = %v, want ambiguity sentinel", err)
	}
	if err := validateWorkloadArtifactBuildContext(workload, "api", json.RawMessage(`{"context":"./missing"}`)); err == nil {
		t.Fatal("a missing build context must fail closed")
	}
	if err := validateWorkloadArtifactBuildContext(workload, "api", json.RawMessage(`{"context":"./service","dockerfile":"Containerfile"}`)); err == nil {
		t.Fatal("a missing Dockerfile must fail closed")
	}
}

func TestResolveRenderedWorkloadArtifactsAmbiguousLocalStore(t *testing.T) {
	// A local store that reports two conflicting digests behind one reference
	// is ambiguous: no arbitrary winner may realize the selection, in any mode.
	runner := &scriptedRunner{store: map[string]string{artifactRepo + ":stable": artifactDigestA + "," + artifactDigestB}}
	withArtifactRegistry(t, runner)
	for _, mode := range []ArtifactResolutionMode{ArtifactResolutionExisting, ArtifactResolutionCheckoutBuild} {
		_, err := resolveRenderedWorkloadArtifacts(context.Background(), runner.fn, dockerArtifactCLI(),
			renderedArtifact(map[string]renderedArtifactService{"api": imageOnlyArtifact(artifactRepo + ":stable")}),
			[]string{"api"}, mode)
		if !errors.Is(err, ErrArtifactResolutionAmbiguous) {
			t.Fatalf("mode %q: err = %v, want ambiguity sentinel", mode, err)
		}
	}
}

func TestPrepareRepositoryWorkloadArtifactResolution(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  api:\n    build: ./service\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "service"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "service", "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workload := application.WorkloadFiles{RepositoryRoot: root, Compose: composePath, Project: "artifact"}

	t.Run("buildless model needs no plan", func(t *testing.T) {
		compose := &renderedWorkloadRuntime{rendered: renderedArtifact(map[string]renderedArtifactService{
			"api": imageOnlyArtifact("alpine:3.22"),
		})}
		plan, err := prepareRepositoryWorkloadArtifactResolution(context.Background(), compose, workload, map[string]string{}, []string{composePath}, []string{"api"})
		if err != nil {
			t.Fatal(err)
		}
		if plan != nil {
			t.Fatalf("no build means no artifact plan, got %+v", plan)
		}
	})
	t.Run("checkout build plan routes and proves the context", func(t *testing.T) {
		compose := &renderedWorkloadRuntime{rendered: renderedArtifact(map[string]renderedArtifactService{
			"api": buildArtifact("./service", "Dockerfile"),
		})}
		plan, err := prepareRepositoryWorkloadArtifactResolution(context.Background(), compose, workload, map[string]string{}, []string{composePath}, []string{"api"})
		if err != nil {
			t.Fatal(err)
		}
		if plan["api"].Action != ArtifactActionNativeBuild {
			t.Fatalf("plan = %+v", plan["api"])
		}
	})
	t.Run("imagined build context fails closed", func(t *testing.T) {
		compose := &renderedWorkloadRuntime{rendered: renderedArtifact(map[string]renderedArtifactService{
			"api": buildArtifact("./ghost", "Dockerfile"),
		})}
		_, err := prepareRepositoryWorkloadArtifactResolution(context.Background(), compose, workload, map[string]string{}, []string{composePath}, []string{"api"})
		if err == nil || !strings.Contains(err.Error(), "build context") {
			t.Fatalf("err = %v, want build-context failure", err)
		}
	})
	t.Run("broken compose file set fails closed", func(t *testing.T) {
		compose := &renderedWorkloadRuntime{rendered: renderedArtifact(map[string]renderedArtifactService{
			"api": buildArtifact("./service", "Dockerfile"),
		})}
		_, err := prepareRepositoryWorkloadArtifactResolution(context.Background(), compose, workload, map[string]string{}, []string{filepath.Join(root, "other.yaml")}, []string{"api"})
		if !errors.Is(err, ErrArtifactResolutionAmbiguous) && !strings.Contains(err.Error(), "Compose file") {
			t.Fatalf("err = %v, want compose-file validation failure", err)
		}
	})
}

// renderedWorkloadRuntime is the minimal RuntimeProvider seam the prepare path
// touches: it renders a scripted Compose JSON model and reports Docker.
type renderedWorkloadRuntime struct {
	bhruntime.RuntimeProvider
	rendered string
}

func (r *renderedWorkloadRuntime) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }

func (r *renderedWorkloadRuntime) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return r.rendered, nil
}

func TestValidateWorkloadArtifactComposeFiles(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workload := application.WorkloadFiles{RepositoryRoot: dir, Compose: composePath}
	if err := validateWorkloadArtifactComposeFiles(workload, []string{composePath}); err != nil {
		t.Fatalf("valid file set rejected: %v", err)
	}
	if err := validateWorkloadArtifactComposeFiles(workload, nil); !errors.Is(err, ErrArtifactResolutionAmbiguous) {
		t.Fatalf("empty file set err = %v, want ambiguity sentinel", err)
	}
	if err := validateWorkloadArtifactComposeFiles(workload, []string{filepath.Join(dir, "gone.yaml")}); err == nil {
		t.Fatal("a missing Compose file must fail closed")
	}
	if err := validateWorkloadArtifactComposeFiles(workload, []string{filepath.Join(dir, "elsewhere.yaml")}); err == nil {
		t.Fatal("a file set that does not start at the workload Compose file must fail closed")
	}
}

// TestRebuildChangedServicesArtifactGate proves the apply-path wiring: a
// service whose artifact plan routes to an existing image must never trigger
// the native rebuild even when its source fingerprint changed.
func TestRebuildChangedServicesArtifactGate(t *testing.T) {
	files := application.RuntimeFiles{Dir: t.TempDir()}
	if err := persistRepositoryWorkloadBuildState(files, map[string]string{"api": "old"}); err != nil {
		t.Fatal(err)
	}
	var built []string
	compose := &recordingBuildRuntime{build: func(_ context.Context, _, _ string, _ map[string]string, services []string, _ func(string), _ ...string) error {
		built = append(built, services...)
		return nil
	}}
	execution := &repositoryWorkloadExecution{
		compose:           compose,
		files:             files,
		workload:          application.WorkloadFiles{Project: "artifact"},
		buildFingerprints: map[string]string{"api": "new", "worker": "new"},
		beforeServices:    map[string]struct{}{},
		buildChanged:      map[string]struct{}{},
		artifactPlan: map[string]ArtifactResolution{
			"api":    {Service: "api", Action: ArtifactActionUseExisting},
			"worker": {Service: "worker", Action: ArtifactActionNativeBuild},
		},
	}
	out := &discardWriter{}
	if err := execution.rebuildChangedServices(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 || built[0] != "worker" {
		t.Fatalf("artifact-gated rebuilds = %v, want [worker]", built)
	}
	if _, ok := execution.buildChanged["api"]; ok {
		t.Fatal("an existing-image service was recorded as rebuilt")
	}
}

// TestRebuildChangedServicesWithoutPlanIsUnchanged proves the pure-image path
// stays byte-identical to the pre-adapter apply: no plan means no gating.
func TestRebuildChangedServicesWithoutPlanIsUnchanged(t *testing.T) {
	files := application.RuntimeFiles{Dir: t.TempDir()}
	if err := persistRepositoryWorkloadBuildState(files, map[string]string{"api": "old"}); err != nil {
		t.Fatal(err)
	}
	var built []string
	compose := &recordingBuildRuntime{build: func(_ context.Context, _, _ string, _ map[string]string, services []string, _ func(string), _ ...string) error {
		built = append(built, services...)
		return nil
	}}
	execution := &repositoryWorkloadExecution{
		compose:           compose,
		files:             files,
		workload:          application.WorkloadFiles{Project: "artifact"},
		buildFingerprints: map[string]string{"api": "new"},
		beforeServices:    map[string]struct{}{},
		buildChanged:      map[string]struct{}{},
	}
	if err := execution.rebuildChangedServices(context.Background(), &discardWriter{}); err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 || built[0] != "api" {
		t.Fatalf("rebuilds = %v, want [api]", built)
	}
}

type recordingBuildRuntime struct {
	bhruntime.RuntimeProvider
	build func(context.Context, string, string, map[string]string, []string, func(string), ...string) error
}

func (r *recordingBuildRuntime) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }

func (r *recordingBuildRuntime) BuildProjectFilesSelectedProgress(ctx context.Context, project, workdir string, environment map[string]string, services []string, onProgress func(string), composeFiles ...string) error {
	return r.build(ctx, project, workdir, environment, services, onProgress, composeFiles...)
}

func (r *recordingBuildRuntime) StopProjectFilesSelected(context.Context, string, string, map[string]string, []string, ...string) error {
	return nil
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
