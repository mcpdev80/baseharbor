package application

import (
	"context"
	"errors"
	"testing"
)

type recordingBackendProbeExecutor struct {
	probes []BackendProbe
	out    string
	err    error
}

func (e *recordingBackendProbeExecutor) ProbeBackend(_ context.Context, probe BackendProbe) (string, error) {
	e.probes = append(e.probes, probe)
	return e.out, e.err
}

func TestVerifyPostgresProviderUsesRuntimeNeutralSemanticProbe(t *testing.T) {
	m := New("demo", "dev", true, false, false)
	exec := &recordingBackendProbeExecutor{out: "1\n"}

	if err := VerifyPostgresProvider(context.Background(), exec, m); err != nil {
		t.Fatal(err)
	}
	if len(exec.probes) != 1 {
		t.Fatalf("probes = %d, want 1", len(exec.probes))
	}
	got := exec.probes[0]
	if got.Kind != BackendProbeSQLSelectOne || got.Instance != defaultServiceInstance || got.Database == "" {
		t.Fatalf("unexpected probe: %#v", got)
	}
}

func TestVerifyValkeyProviderUsesRuntimeNeutralSemanticProbe(t *testing.T) {
	m := New("demo", "dev", false, true, false)
	exec := &recordingBackendProbeExecutor{out: "PONG\n"}

	if err := VerifyValkeyProvider(context.Background(), exec, m); err != nil {
		t.Fatal(err)
	}
	if len(exec.probes) != 1 {
		t.Fatalf("probes = %d, want 1", len(exec.probes))
	}
	got := exec.probes[0]
	if got.Kind != BackendProbeCachePing || got.Instance != defaultServiceInstance {
		t.Fatalf("unexpected probe: %#v", got)
	}
}

func TestVerifyBackendProviderPropagatesSemanticProbeFailure(t *testing.T) {
	m := New("demo", "dev", true, false, false)
	exec := &recordingBackendProbeExecutor{err: errors.New("transport unavailable")}

	if err := VerifyPostgresProvider(context.Background(), exec, m); err == nil {
		t.Fatal("expected verification failure")
	}
}
