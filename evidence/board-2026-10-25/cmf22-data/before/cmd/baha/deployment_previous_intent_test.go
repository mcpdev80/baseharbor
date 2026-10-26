package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestInterruptedApplyRetainsOriginalIntentAcrossRetries(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(httpsAdoptionFixture(t, false))
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m.Services.Identity = true
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	original, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	// The resolver snapshot predates the last successful record. Mutation must
	// reload the authoritative record, not discard the prior Identity consumer.
	resolved.DeploymentRecord = nil
	resolved.Manifest.Services.Identity = false
	for attempt := 0; attempt < 2; attempt++ {
		pending, err := recordDeploymentBeforeMutation(context.Background(), resolved, "applying")
		if err != nil {
			t.Fatal(err)
		}
		var prior application.Manifest
		if err := json.Unmarshal([]byte(pending.Applied.GeneratedState["previous_intent"]), &prior); err != nil || !reflect.DeepEqual(prior, m) {
			t.Fatalf("retry %d lost original intent: %#v %v", attempt, prior, err)
		}
		var desired application.Manifest
		if err := json.Unmarshal(pending.Applied.Intent, &desired); err != nil || desired.Services.Identity {
			t.Fatalf("retry %d lost desired intent: %#v %v", attempt, desired, err)
		}
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	verified, err := deployment.LoadDeploymentRecord(original.Identity)
	if err != nil || verified.Applied.GeneratedState["previous_intent"] != "" {
		t.Fatalf("verified convergence retained stale previous intent: %#v %v", verified, err)
	}
}
