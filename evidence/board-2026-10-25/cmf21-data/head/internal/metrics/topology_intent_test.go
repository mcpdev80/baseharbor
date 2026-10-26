package metrics

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"os"
	"testing"
)

func TestProviderExplicitIntentDoesNotRewriteRetainedDataTopology(t *testing.T) {
	no, yes := false, true
	for _, scenario := range []struct {
		name     string
		global   bool
		override availability.Override
		count    int
	}{
		{name: "omitted", count: 1}, {name: "false", override: availability.Override{HA: &no}, count: 1},
		{name: "global-ha", global: true, count: 2},
		{name: "global-ha-single-override", global: true, override: availability.Override{HA: &no}, count: 1},
		{name: "component-ha", override: availability.Override{HA: &yes}, count: 2},
		{name: "explicit-members", override: availability.Override{HA: &yes, Instances: 5}, count: 5},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			issuer := serviceissuer.New(t)
			m := application.Manifest{Name: "demo", Environment: "dev", HA: scenario.global, Availability: map[string]availability.Override{"metrics": scenario.override}}
			files, err := EnsureProviderFilesWithRuntimeCAAt(ctx, issuer, root, "audit", m, "")
			if err != nil {
				t.Fatal(err)
			}
			actual, err := providertopology.ExistingMembers(files.Compose, "prometheus")
			if err != nil || actual != scenario.count {
				t.Fatalf("members=%d want=%d error=%v", actual, scenario.count, err)
			}
			before, err := os.ReadFile(files.Compose)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := EnsureProviderFilesWithRuntimeCAAt(ctx, issuer, root, "audit", m, ""); err != nil {
				t.Fatalf("same intent reconciliation failed: %v", err)
			}
			m.HA = !scenario.global
			flip := scenario.count == 1
			m.Availability = map[string]availability.Override{"metrics": {HA: &flip}}
			if _, err := EnsureProviderFilesWithRuntimeCAAt(ctx, issuer, root, "audit", m, ""); err == nil {
				t.Fatal("retained datastore topology silently changed")
			}
			after, err := os.ReadFile(files.Compose)
			if err != nil || string(after) != string(before) {
				t.Fatal("rejected migration rewrote retained Compose")
			}
		})
	}
}
