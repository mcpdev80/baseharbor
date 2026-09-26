package applicationbackup

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestDiscoverManifestRecoveryUsesPortableStateClasses(t *testing.T) {
	m := application.New("demo", "dev", false, false, true)
	m = application.WithSQLInstances(m, "primary", "reporting")
	m = application.WithObjectStorageBuckets(m, "uploads")
	m = application.WithLogsCollection(m, "api")
	m = application.WithMetricsSource(m, "api", "api", 9090, "/metrics")
	m = application.WithOTLPTelemetry(m, "metrics", "traces")

	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatal(err)
	}

	want := map[RecoveryStateClass]int{
		StateApplicationMetadata: 1,
		StateSecrets:             1,
		StateSQL:                 2,
		StateObjectStorage:       1,
		StateLogs:                1,
		StateMetrics:             1,
		StateTraces:              1,
	}
	got := map[RecoveryStateClass]int{}
	for _, c := range selection.Contributors {
		got[c.StateClass]++
		if c.StateClass == StateObjectStorage && c.Support != RecoveryUnsupported {
			t.Fatalf("object storage support = %q, want unsupported until capture/restore lands", c.Support)
		}
	}
	for class, count := range want {
		if got[class] != count {
			t.Fatalf("%s contributors = %d, want %d", class, got[class], count)
		}
	}
}

func TestDiscoverManifestRecoveryOmitsUndeclaredHistoryClasses(t *testing.T) {
	m := application.New("demo", "dev", true, false, false)
	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range selection.Contributors {
		switch c.StateClass {
		case StateLogs, StateMetrics, StateTraces, StateObjectStorage:
			t.Fatalf("unexpected undeclared recovery contributor: %#v", c)
		}
	}
}
