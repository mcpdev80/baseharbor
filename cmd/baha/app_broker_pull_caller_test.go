package main

import (
	"context"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// callerPullRecordsCompose is the production runtime provider used by the
// caller-discriminator cell. It records every pull reference the production
// caller (ensureAndStartRuntimeBroker -> pullResolvedRuntimeImage ->
// compose.PullImage) performs, and fails OpenBao readiness so the caller
// returns before the workload path. The pull assertion is the discriminator:
// the real production caller must pull the resolved digest identity, not the
// mutable tag.
type callerPullRecordsCompose struct {
	bhruntime.RuntimeProvider
	pulled []string
}

func (c *callerPullRecordsCompose) PullImage(_ context.Context, image string) error {
	c.pulled = append(c.pulled, image)
	return nil
}

// failReadinessExecProject fails OpenBao manager readiness so the production
// caller returns right after the (already-recorded) pull. It never reaches the
// workload path.
func (c *callerPullRecordsCompose) ExecProjectInput(_ context.Context, _ string, _ string, _ string, _ []byte, _ string, _ ...string) (string, error) {
	return "", io.EOF
}

// callerResolveRecordsExec is the native artifact-tool seam
// (resolveRegistrySourceImpl) that resolves the configured development image
// endpoint to the immutable digest identity the production caller must pull.
// The test installs it so resolveRegistrySourceImpl returns the digest; the
// production caller then pulls exactly that identity.
type callerResolveRecordsExec struct {
	ran bool
}

func (e *callerResolveRecordsExec) fn(_ context.Context, name string, args ...string) (string, error) {
	e.ran = true
	return callerDiscriminatorDigest, nil
}

const callerDiscriminatorImage = "ghcr.io/mcpdev80/baseharbor-runtime:edge"

const callerDiscriminatorDigest = "ghcr.io/mcpdev80/baseharbor-runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// TestProductionCallerPinsResolvedDigestNotMutableTag is the discriminator cell
// for review point 3 round 3. It drives the REAL production caller
// (ensureAndStartRuntimeBroker) with a mutable development image configured via
// BASEHARBOR_RUNTIME_IMAGE. The production caller resolves the configured
// endpoint through RegistrySource.Resolve and pulls exactly the immutable
// repository@digest identity.
//
// Discriminator (mutant lock): with the real production caller in place the
// recorded pull is the resolved digest identity (GREEN). The mutant that
// removes/neutralizes the real production caller body and pulls the raw mutable
// tag (compose.PullImage(ctx, runtimeImage)) makes this assertion fail (RED).
// Re-installing the mutation-free path pulls the digest identity (GREEN) again.
//
// The cell proves the caller is wired into the production broker flow (the
// gap that left the round-2 cell GREEN when the caller was removed), not only
// that the resolve adapter exists.
func TestProductionCallerPinsResolvedDigestNotMutableTag(t *testing.T) {
	resolve := &callerResolveRecordsExec{}
	prev := registrySourceExec
	registrySourceExec = resolve.fn
	t.Cleanup(func() { registrySourceExec = prev })

	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", callerDiscriminatorImage)
	m := application.New("registry-caller-discriminator", "dev", false, false, true)
	if !application.RequiresRuntimeBroker(m) {
		t.Fatal("fixture needs a runtime broker")
	}

	compose := &callerPullRecordsCompose{}
	platformFiles := bhruntime.Files{}
	appFiles := application.RuntimeFiles{}

	err := ensureAndStartRuntimeBroker(context.Background(), io.Discard, compose, platformFiles, m, appFiles)
	if err == nil {
		t.Fatal("production caller expected to fail on OpenBao readiness after the pull")
	}
	if !resolve.ran {
		t.Fatal("production caller did not drive the artifact resolve invocation through resolveRegistrySourceImpl")
	}
	if len(compose.pulled) != 1 {
		t.Fatalf("production caller must perform exactly one pull, got %d (%v)", len(compose.pulled), compose.pulled)
	}
	if compose.pulled[0] != callerDiscriminatorDigest {
		t.Fatalf("production caller must pull the resolved immutable digest identity %q, got %q (pulling the mutable tag is the RED mutant)", callerDiscriminatorDigest, compose.pulled[0])
	}
}
