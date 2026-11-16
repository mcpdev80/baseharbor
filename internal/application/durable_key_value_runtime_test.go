package application

import (
	"context"
	"strings"
	"testing"
)

type durableKeyValueProbeExecutor struct {
	probes []BackendProbe
}

func (e *durableKeyValueProbeExecutor) ProbeBackend(_ context.Context, probe BackendProbe) (string, error) {
	e.probes = append(e.probes, probe)
	switch probe.Kind {
	case BackendProbeCachePing:
		return "PONG\n", nil
	case BackendProbeDurableKeyValueRW:
		return "durable\n", nil
	default:
		return "", nil
	}
}

func TestDurableKeyValueRuntimeUsesPersistentValkeyAndWriteReadVerification(t *testing.T) {
	useApplicationScopedDataProviders(t)
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "ledger",
		Environment:   "dev",
		Services: Services{
			KeyValue:          true,
			KeyValueInstances: map[string]ServiceInstance{"sessions": {}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	rendered, err := RuntimeComposeYAML(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"valkey-sessions", "appendonly yes", "valkey-sessions-data:/data"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("durable key-value runtime missing %q:\n%s", want, rendered)
		}
	}
	exec := &durableKeyValueProbeExecutor{}
	if err := VerifyValkeyProvider(context.Background(), exec, m); err != nil {
		t.Fatal(err)
	}
	if len(exec.probes) != 1 || exec.probes[0].Kind != BackendProbeDurableKeyValueRW || exec.probes[0].Instance != "sessions" {
		t.Fatalf("durable key-value probes = %#v", exec.probes)
	}
}

func TestCacheAndDurableKeyValueCannotShareLogicalInstanceName(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "ledger",
		Environment:   "dev",
		Services: Services{
			Cache:             true,
			KeyValue:          true,
			CacheInstances:    map[string]ServiceInstance{"sessions": {}},
			KeyValueInstances: map[string]ServiceInstance{"sessions": {}},
		},
	}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "cannot be both") {
		t.Fatalf("Validate() error = %v, want cache/durable collision", err)
	}
}
