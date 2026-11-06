package main

import (
	"context"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// registryPullRecordingCompose records every PullImage reference so the e2e
// test can observe exactly what the production caller resolved to.
type registryPullRecordingCompose struct {
	bhruntime.RuntimeProvider
	pulled []string
}

func (c *registryPullRecordingCompose) PullImage(_ context.Context, image string) error {
	c.pulled = append(c.pulled, image)
	return nil
}

// resolveRegistrySourceExec records the native tool invocation the production
// caller performed and returns a digest-carrying resolve reply.
type resolveRegistrySourceExec struct {
	calls []string
}

func (e *resolveRegistrySourceExec) fn(_ context.Context, name string, args ...string) (string, error) {
	e.calls = append(e.calls, name+" "+strings.Join(args, " "))
	return "Resolved ghcr.io/mcpdev80/baseharbor-runtime@sha256:" + strings.Repeat("a", 64), nil
}

// TestPullResolvedRuntimeImagePinsImmutableDigest is the e2e cell for review
// point 4: the single pull site (cmd/baha/app_broker.go) resolves the
// configured development image endpoint through RegistrySource.Resolve and pulls
// the immutable repository@digest identity, not the mutable tag. It drives the
// production caller (pullResolvedRuntimeImage -> resolveRegistrySource ->
// resolveRegistrySourceImpl) with a fake native runner installed on the
// registrySourceExec seam so the native oras resolve invocation is recorded.
//
// Discriminator (mutant lock): with the caller in place the pull receives the
// resolved digest identity (GREEN). The mutant that removes the caller and
// pulls the raw mutable tag makes this assertion fail (RED). Re-installing the
// mutation-free path pulls the digest identity (GREEN) again.
func TestPullResolvedRuntimeImagePinsImmutableDigest(t *testing.T) {
	const edgeImage = "ghcr.io/mcpdev80/baseharbor-runtime:edge"
	digest := "sha256:" + strings.Repeat("a", 64)
	want := "ghcr.io/mcpdev80/baseharbor-runtime@" + digest

	exec := &resolveRegistrySourceExec{}
	prev := registrySourceExec
	registrySourceExec = exec.fn
	t.Cleanup(func() { registrySourceExec = prev })

	compose := &registryPullRecordingCompose{}
	if err := pullResolvedRuntimeImage(context.Background(), compose, edgeImage, "edge"); err != nil {
		t.Fatalf("pullResolvedRuntimeImage failed: %v", err)
	}

	if len(exec.calls) == 0 {
		t.Fatal("production caller did not drive the native resolve invocation")
	}
	if !strings.Contains(exec.calls[0], edgeImage) {
		t.Fatalf("native resolve must address the configured endpoint %q, got %q", edgeImage, exec.calls[0])
	}
	if len(compose.pulled) != 1 {
		t.Fatalf("expected exactly one pull, got %d (%v)", len(compose.pulled), compose.pulled)
	}
	if compose.pulled[0] != want {
		t.Fatalf("pull must receive the resolved immutable digest identity %q, got %q (pulling the mutable tag is the RED mutant)", want, compose.pulled[0])
	}
}
