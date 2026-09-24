package application

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

func TestManagedRuntimeObservabilityUsesCanonicalServiceIdentities(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	m := New("demo", "dev", true, true, false)
	m = WithLogsCollection(m, "application")
	m = WithOTLPTelemetry(m, "traces")

	if err := reconcileManagedRuntimeObservability(m); err != nil {
		t.Fatal(err)
	}

	logs, err := observability.ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeApplication},
		[]string{m.Name},
		true,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	traces, err := observability.ListTraces(
		capability.ProviderPlacement{Scope: capability.ScopeApplication},
		[]string{m.Name},
		true,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	assertTargets := func(kind string, sources []observability.SignalSource) {
		t.Helper()
		got := map[capability.ProviderKind]string{}
		for _, source := range sources {
			got[source.Provider] = source.Target
		}
		if target := got[capability.ProviderPostgreSQL]; !strings.HasSuffix(target, "/postgres") {
			t.Fatalf("%s PostgreSQL target = %q, want canonical /postgres service", kind, target)
		}
		if target := got[capability.ProviderValkey]; !strings.HasSuffix(target, "/valkey") {
			t.Fatalf("%s Valkey target = %q, want canonical /valkey service", kind, target)
		}
	}

	assertTargets("logs", logs)
	assertTargets("traces", traces)
}
