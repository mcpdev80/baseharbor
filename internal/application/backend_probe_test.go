package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	useApplicationScopedDataProviders(t)
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
	useApplicationScopedDataProviders(t)
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
	useApplicationScopedDataProviders(t)
	m := New("demo", "dev", true, false, false)
	exec := &recordingBackendProbeExecutor{err: errors.New("transport unavailable")}

	if err := VerifyPostgresProvider(context.Background(), exec, m); err == nil {
		t.Fatal("expected verification failure")
	}
}

func TestObserveSQLConsumerInventoryRequiresActualCredentialBoundCatalog(t *testing.T) {
	useApplicationScopedDataProviders(t)
	m := New("demo", "dev", true, false, false)
	m.ApplicationID = MustNewApplicationID()
	files := RuntimeFiles{Env: filepath.Join(t.TempDir(), "runtime.env")}
	if err := os.WriteFile(files.Env, []byte("POSTGRES_DB=demo_dev\nPOSTGRES_USER=baseharbor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exec := &recordingBackendProbeExecutor{out: "baseharbor|demo_dev|0|0"}
	observed, err := ObserveSQLConsumerInventory(context.Background(), exec, files, m, "default")
	if err != nil || !observed.Empty || observed.Consumer != "application/"+m.ApplicationID+"/dev" {
		t.Fatalf("actual SQL consumer was not identified: %+v %v", observed, err)
	}
	if len(exec.probes) != 1 || exec.probes[0].Kind != BackendProbeSQLConsumerInventory ||
		exec.probes[0].Database != "demo_dev" {
		t.Fatalf("native SQL probe missing: %+v", exec.probes)
	}
	for _, tc := range []struct{out string; wantErr, wantEmpty bool}{
		{"baseharbor|demo_dev|1|0", false, false},
		{"baseharbor|demo_dev|0|2", false, false},
		{"postgres|demo_dev|0|0", true, false},
		{"baseharbor|other|0|0", true, false},
		{"baseharbor|demo_dev|invalid|0", true, false},
	} {
		exec.out = tc.out
		got, err := ObserveSQLConsumerInventory(context.Background(), exec, files, m, "default")
		if (err != nil) != tc.wantErr || got.Empty != tc.wantEmpty {
			t.Fatalf("query %q yielded %+v, err %v", tc.out, got, err)
		}
	}
	exec.err = errors.New("secret-password must not be logged")
	if _, err := ObserveSQLConsumerInventory(context.Background(), exec, files, m, "default"); err == nil ||
		strings.Contains(err.Error(), "secret-password") {
		t.Fatalf("probe failure leaked a credential: %v", err)
	}
}
