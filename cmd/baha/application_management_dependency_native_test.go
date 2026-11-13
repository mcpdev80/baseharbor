package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Extend the existing real Core bootstrap receipt, without creating an app,
// reseeding data, provisioning a registry offer or claiming login continuity.
func verifyIndependentManagementProviderUnion(t *testing.T, ctx context.Context, target deployment.ResolvedTarget, state coreinstallation.State) {
	t.Helper()
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	m := application.WithWorkloadComponents(application.New("union-probe", "dev", false, false, false), "api")
	resolved := resolvedApplication{Target: target, TargetStateRoot: root, Manifest: m}
	snapshot, preferences, err := applicationLifecycleSnapshot(resolved, observeApplicationProviderFacts(ctx, resolved))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveApplicationProviderPlanFromSnapshot(resolved, snapshot, preferences)
	if err != nil || !plan.Complete {
		t.Fatalf("actual management union incomplete: %+v %v", plan, err)
	}
	wantRequired := 0
	if snapshot.ProtectCoreForConnection {
		wantRequired = 3 // SQL registry, certificate authority and Core Identity serve the connected workstation.
	}
	if len(plan.Required) != wantRequired || len(snapshot.Registry.Instances) != 0 || len(snapshot.Registry.Bindings) != 0 {
		t.Fatalf("actual management dependencies or registry unexpected: protected=%t plan=%+v", snapshot.ProtectCoreForConnection, plan)
	}
	for _, change := range plan.Changes {
		fact := snapshot.Facts[change.InstanceID]
		if snapshot.ProtectCoreForConnection && (change.Action != "reuse" || fact.State != application.ProviderRunning || fact.Containers.Value == nil || *fact.Containers.Value == 0 || len(change.Consumers) != 1 || change.Consumers[0] != "installation/"+state.ID) {
			t.Fatalf("actual management module lacks owned running evidence: %+v %+v", change, fact)
		}
		if !snapshot.ProtectCoreForConnection && change.Action != "stop" && change.Action != "preserve" {
			t.Fatalf("unconsumed management module was not released: %+v", change)
		}
	}
	if plan.Consumers["application/"+m.ApplicationID+"/dev"].Core != application.CoreNotRequired {
		t.Fatal("plain app acquired management dependencies")
	}
	t.Logf("Independent management union: connection-protected=%t; empty app registry; zero-capability app remains Core-free.", snapshot.ProtectCoreForConnection)
}
