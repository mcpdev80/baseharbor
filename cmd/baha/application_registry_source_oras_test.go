package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// notInstalledRunnerError mirrors what the production runner returns for a
// host without the native tool: the exec package's own not-found identity,
// which the redaction must preserve for classification.
func notInstalledRunnerError() error {
	return &exec.Error{Name: "oras", Err: exec.ErrNotFound}
}

// TestExecArtifactReportsMissingBinaryIdentity proves the production runner
// keeps the executable-not-found identity instead of collapsing it into the
// generic redacted "oras: command failed" class. A host without the tool must
// be classifiable, and the message must never leak helper payload.
func TestExecArtifactReportsMissingBinaryIdentity(t *testing.T) {
	_, err := execArtifact(context.Background(), "baseharbor-missing-tool-probe", "resolve", "example.invalid/app:edge")
	if err == nil {
		t.Fatal("expected an error for a binary that is not installed")
	}
	if !orgconfig.IsNativeToolNotFound(err) {
		t.Fatalf("missing binary must preserve exec.ErrNotFound, got %v", err)
	}
}

// TestResolveClassifiesMissingOras proves the adapter reports the actionable
// missing-tool identity when the native runner cannot find oras: the error is
// never the generic "command failed" text, it names the binary and the
// remediation, and it still unwraps to ErrRegistrySourceUnavailable so every
// existing class gate keeps its semantics.
func TestResolveClassifiesMissingOras(t *testing.T) {
	installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
		return "", notInstalledRunnerError()
	})
	_, err := resolveRegistrySourceImpl(context.Background(), "registry.example.invalid/org/app", "edge")
	if err == nil {
		t.Fatal("a missing native tool must fail closed")
	}
	if !errors.Is(err, ErrRegistryNativeToolMissing) {
		t.Fatalf("missing oras must classify as ErrRegistryNativeToolMissing, got %v", err)
	}
	if !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("the missing-tool identity must unwrap to the unavailable class, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "oras: command failed") {
		t.Fatalf("the generic command-failed message must never surface for a missing tool: %s", msg)
	}
	if !strings.Contains(msg, "oras") || !strings.Contains(msg, "not installed") {
		t.Fatalf("the message must name the binary and the remediation: %s", msg)
	}
}

// TestResolveKeepsGenericClassForOtherToolFailures proves the classification
// is narrower than message matching: a tool that exists but fails (e.g. a
// network error) must stay in the generic unavailable class without the
// missing-tool identity.
func TestResolveKeepsGenericClassForOtherToolFailures(t *testing.T) {
	installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
		return "", errors.New("oras: command failed")
	})
	_, err := resolveRegistrySourceImpl(context.Background(), "registry.example.invalid/org/app", "edge")
	if err == nil {
		t.Fatal("a failing native tool must fail closed")
	}
	if errors.Is(err, ErrRegistryNativeToolMissing) {
		t.Fatalf("a generic tool failure must not claim the binary is missing: %v", err)
	}
	if !errors.Is(err, ErrRegistrySourceUnavailable) {
		t.Fatalf("tool failures must stay in the unavailable class, got %v", err)
	}
}

// TestPullWithMissingOrasStatesCauseAndZeroPulls proves
// pullResolvedRuntimeImage reports the actionable missing-oras cause instead
// of an opaque "resolve development runtime image: oras: command failed",
// fail-closed with zero pulls, both with the mutable development opt-in off
// and on.
func TestPullWithMissingOrasStatesCauseAndZeroPulls(t *testing.T) {
	for _, optIn := range []string{"", "true"} {
		t.Setenv("BASEHARBOR_ALLOW_MUTABLE_DEVELOPMENT_IMAGE", optIn)
		installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
			return "", notInstalledRunnerError()
		})
		compose := &callerRecordingCompose{}
		err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge")
		if err == nil {
			t.Fatalf("opt-in %q: a missing native tool must propagate fail-closed", optIn)
		}
		if !errors.Is(err, ErrRegistryNativeToolMissing) {
			t.Fatalf("opt-in %q: the actionable missing-tool identity must surface, got %v", optIn, err)
		}
		if strings.Contains(err.Error(), "command failed") {
			t.Fatalf("opt-in %q: the generic command-failed class must never surface: %v", optIn, err)
		}
		if len(compose.pulled) != 0 {
			t.Fatalf("opt-in %q: a missing tool must cause NULL pulls, got %d (%v)", optIn, len(compose.pulled), compose.pulled)
		}
	}
}

// installNativeRunner replaces the native artifact-tool runner for the
// duration of the test.
func installNativeRunner(t *testing.T, fn ExecFunc) {
	t.Helper()
	prev := registrySourceExec
	registrySourceExec = fn
	t.Cleanup(func() { registrySourceExec = prev })
}
