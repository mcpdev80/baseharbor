package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// callerRecordingCompose records every PullImage reference the production
// caller performs; the negative cells assert this record stays NULL.
type callerRecordingCompose struct {
	bhruntime.RuntimeProvider
	pulled []string
}

func (c *callerRecordingCompose) PullImage(_ context.Context, image string) error {
	c.pulled = append(c.pulled, image)
	return nil
}

// installFailingNativeResolve replaces the native artifact-tool seam for the
// duration of the test and returns the error the real adapter surfaces when
// the tool itself fails (the wrapped ErrRegistrySourceUnavailable form that
// resolve produces for a denied/unreachable endpoint).
func installFailingNativeResolve(t *testing.T, reason string) {
	t.Helper()
	prev := registrySourceExec
	registrySourceExec = func(context.Context, string, ...string) (string, error) {
		return "", errors.New(reason)
	}
	t.Cleanup(func() { registrySourceExec = prev })
}

// TestFailedResolvePropagatesWithZeroPulls proves the resolution failure
// (unavailable tooling: no native binary / unreachable host) propagates out of
// pullResolvedRuntimeImage and the caller performs NULL pulls.
func TestFailedResolvePropagatesWithZeroPulls(t *testing.T) {
	installFailingNativeResolve(t, "oras: command failed")

	compose := &callerRecordingCompose{}
	err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge")
	if err == nil {
		t.Fatal("failed resolve must propagate, got nil error")
	}
	if !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("failed resolve must propagate as ErrRegistrySourceUnavailable, got %v", err)
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("failed resolve must cause NULL pulls, got %d (%v)", len(compose.pulled), compose.pulled)
	}
}

// TestDeniedEndpointResolveCausesNoPull proves that an endpoint refusal (HTTP
// denial from the configured enterprise registry) propagates fail-closed with
// NULL pulls: no mutable reference is pulled behind a denied endpoint.
func TestDeniedEndpointResolveCausesNoPull(t *testing.T) {
	// Opt-in ON: a denial must fail closed regardless of the fallback setting.
	t.Setenv(envAllowMutableDevelopmentRuntimeImage, "true")
	installFailingNativeResolve(t, "denied: access forbidden")

	compose := &callerRecordingCompose{}
	err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge")
	if err == nil {
		t.Fatal("denied endpoint must fail closed, got nil error")
	}
	if !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("denied endpoint must propagate ErrRegistrySourceUnavailable, got %v", err)
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("denied endpoint must cause NULL pulls, got %d (%v)", len(compose.pulled), compose.pulled)
	}
}

// TestMissingDigestResolveCausesNoPull proves a resolve that returns no
// immutable sha256 digest fails closed with NULL pulls.
func TestMissingDigestResolveCausesNoPull(t *testing.T) {
	prev := registrySourceExec
	registrySourceExec = func(context.Context, string, ...string) (string, error) {
		return "Resolved registry.example.invalid/org/app:edge", nil
	}
	t.Cleanup(func() { registrySourceExec = prev })

	compose := &callerRecordingCompose{}
	err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge")
	if err == nil {
		t.Fatal("missing-digest resolve must fail closed, got nil error")
	}
	if !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("missing-digest must propagate ErrRegistrySourceUnavailable, got %v", err)
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("missing-digest must cause NULL pulls, got %d (%v)", len(compose.pulled), compose.pulled)
	}
}

// TestPublicDefaultDockerHubResolveCausesNoPull proves the public-default
// rejection (docker.io) survives the legacy development opt-in: the pull count
// stays NULL even with the opt-in ON. The rejection must be independent of the
// native tool: the tool seam is NEVER stubbed here, so the fail-closure is the
// adapter's own classification, not a fake's behavior.
func TestPublicDefaultDockerHubResolveCausesNoPull(t *testing.T) {
	t.Setenv(envAllowMutableDevelopmentRuntimeImage, "true")

	compose := &callerRecordingCompose{}
	err := pullResolvedRuntimeImage(context.Background(), compose, "docker.io/library/app:edge", "edge")
	if err == nil {
		t.Fatal("public-default docker.io endpoint must fail closed, got nil error")
	}
	if !errors.Is(err, ErrRegistrySourcePublicDefault) {
		t.Fatalf("public-default must propagate the ErrRegistrySourcePublicDefault hard-rejection identity, got %v", err)
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("public-default must cause NULL pulls, got %d (%v)", len(compose.pulled), compose.pulled)
	}
}

// TestPublicDefaultRejectedBeforeNativeTool proves the docker.io rejection
// happens before any native tool invocation: a tool that answers anything
// (which would otherwise "resolve" the location) must never be reached, with
// the opt-in ON.
func TestPublicDefaultRejectedBeforeNativeTool(t *testing.T) {
	t.Setenv(envAllowMutableDevelopmentRuntimeImage, "true")
	prev := registrySourceExec
	registrySourceExec = func(context.Context, string, ...string) (string, error) {
		t.Error("public-default location must be rejected before the native tool runs")
		return "Resolved docker.io/library/app@sha256:" + strings.Repeat("c", 64), nil
	}
	t.Cleanup(func() { registrySourceExec = prev })

	compose := &callerRecordingCompose{}
	err := pullResolvedRuntimeImage(context.Background(), compose, "docker.io/library/app:edge", "edge")
	if !errors.Is(err, ErrRegistrySourcePublicDefault) {
		t.Fatalf("public-default must propagate ErrRegistrySourcePublicDefault, got %v", err)
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("public-default must cause NULL pulls, got %d (%v)", len(compose.pulled), compose.pulled)
	}
}

// TestUnavailableResolveWithOptInPullsMutableTag proves the legacy development
// opt-in is the ONLY path that pulls a mutable reference, and only when the
// endpoint merely could not be resolved (not rejected as public-default):
// exactly one pull of the original location happens.
func TestUnavailableResolveWithOptInPullsMutableTag(t *testing.T) {
	t.Setenv(envAllowMutableDevelopmentRuntimeImage, "true")
	installFailingNativeResolve(t, "oras: command failed")

	compose := &callerRecordingCompose{}
	if err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge"); err != nil {
		t.Fatalf("opt-in unavailable path must pull, got %v", err)
	}
	if len(compose.pulled) != 1 || compose.pulled[0] != "registry.example.invalid/org/app:edge" {
		t.Fatalf("opt-in must pull exactly the original mutable reference, got %v", compose.pulled)
	}
}

// TestUnavailableResolveWithoutOptInFailsClosed proves that without the
// opt-in the same unavailable endpoint fails closed with NULL pulls.
func TestUnavailableResolveWithoutOptInFailsClosed(t *testing.T) {
	t.Setenv(envAllowMutableDevelopmentRuntimeImage, "")
	installFailingNativeResolve(t, "oras: command failed")

	compose := &callerRecordingCompose{}
	err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge")
	if err == nil {
		t.Fatal("without opt-in an unavailable endpoint must fail closed")
	}
	if !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("expected ErrRegistrySourceUnavailable, got %v", err)
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("without opt-in must cause NULL pulls, got %d (%v)", len(compose.pulled), compose.pulled)
	}
}
