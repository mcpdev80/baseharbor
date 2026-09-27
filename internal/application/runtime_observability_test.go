package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

func TestManagedRuntimeObservabilityUsesCanonicalServiceIdentities(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	m := New("demo", "dev", true, true, false)
	m = WithLogsCollection(m, "application")
	m = WithOTLPTelemetry(m, "traces")

	project := "baseharbor-local-demo-dev"
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
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
		if target := got[capability.ProviderPostgreSQL]; target != "runtime://"+project+"/postgres" {
			t.Fatalf("%s PostgreSQL target = %q, want target-scoped %q", kind, target, "runtime://"+project+"/postgres")
		}
		if target := got[capability.ProviderValkey]; target != "runtime://"+project+"/valkey" {
			t.Fatalf("%s Valkey target = %q, want target-scoped %q", kind, target, "runtime://"+project+"/valkey")
		}
	}

	assertTargets("logs", logs)
	assertTargets("traces", traces)
}


func TestManagedRuntimeObservabilityExcludesSharedBackendServices(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), "shared")

	m := New("demo", "dev", true, true, false)
	m = WithLogsCollection(m, "application-provider")

	if got := ManagedRuntimeProviderServiceNames(m); len(got) != 0 {
		t.Fatalf("shared backend services leaked into application runtime observability: %v", got)
	}

	project := "baseharbor-local-demo-dev"
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
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
	for _, source := range logs {
		if source.Provider == capability.ProviderPostgreSQL || source.Provider == capability.ProviderValkey {
			t.Fatalf("shared backend registered as application-scoped runtime source: %+v", source)
		}
	}
}
