package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestFreshSecretsPlanAcknowledgementsRemainBoundBeforeMutation(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	for path, data := range map[string]string{"compose.yaml": corelessHTTPWorkload, "baseharbor.yaml": "version: 1\napp:\n  name: selective-secrets\n  environment: dev\nworkload:\n  components:\n    - api\nservices:\n  secrets: true\nsecrets:\n  required:\n    - name: API_TOKEN\n      generate:\n        type: hex\n        bytes: 32\n"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := machineNoninteractiveContext(t.Context())
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	var first string
	records, err := deployment.ListDeployments(resolved.Target.Name)
	if err != nil || len(records) != 0 {
		t.Fatal("fresh initialization unexpectedly has a deployment", err)
	}
	var firstPlan any
	var firstManifest any
	for i := 0; i < 40; i++ {
		resolved, err = resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildResolvedApplicationPlan(ctx, resolved)
		if err != nil || plan.ProviderPlan.Impact == nil {
			t.Fatal("fresh Secrets plan unavailable", err)
		}
		id := plan.ProviderPlan.Impact.PlanID
		if i == 0 {
			first = id
			raw, _ := json.Marshal(plan)
			json.Unmarshal(raw, &firstPlan)
			raw, _ = json.Marshal(resolved.Manifest)
			json.Unmarshal(raw, &firstManifest)
		} else if id != first {
			raw, _ := json.Marshal(plan)
			var current any
			json.Unmarshal(raw, &current)
			t.Log("changed plan paths", changedSecretsPlanPaths("plan", firstPlan, current))
			raw, _ = json.Marshal(resolved.Manifest)
			json.Unmarshal(raw, &current)
			t.Log("changed intent paths", changedSecretsPlanPaths("intent", firstManifest, current))
			t.Fatalf("unchanged read-only Secrets plan changed acknowledgement ID at iteration %d: %s -> %s", i, first, id)
		}
	}
	records, err = deployment.ListDeployments(resolved.Target.Name)
	if err != nil || len(records) != 0 {
		t.Fatal("read-only plan allocated a deployment owner", err)
	}
	execution, err := newApplicationApplyExecution(ctx, application.Store{}, nil, &output, &output)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := lockApplicationTargetLifecycle(execution.resolved)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := requireResolvedApplicationCore(ctx, execution.resolved, nil, &output); err != nil {
		t.Fatal(err)
	}
	plan, err := buildResolvedApplicationPlan(ctx, execution.resolved)
	if err != nil || plan.ProviderPlan.Impact.PlanID != first {
		t.Fatal("apply preparation changed the acknowledged plan", err)
	}
	consent := application.ProviderChangeConsent{PlanID: first, First: true, Second: true}
	if err := authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &consent), execution.resolved, nil, &output); err != nil {
		t.Fatal("unchanged plan acknowledgements were rejected", err)
	}
	modified := resolved
	modified.Manifest.Name = "changed-consumer"
	changed, err := buildResolvedApplicationPlan(ctx, modified)
	if err != nil || changed.ProviderPlan.Impact == nil {
		t.Fatal("changed consumer plan unavailable", err)
	}
	if err := application.ValidateProviderChangeConsent(*changed.ProviderPlan.Impact, consent); err == nil {
		t.Fatal("stable owner fix accepted consent for a changed consumer")
	}
}

func changedSecretsPlanPaths(path string, before, after any) []string {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	b, bok := before.(map[string]any)
	a, aok := after.(map[string]any)
	if bok && aok {
		out := []string{}
		seen := map[string]bool{}
		for k := range b {
			seen[k] = true
		}
		for k := range a {
			seen[k] = true
		}
		for k := range seen {
			out = append(out, changedSecretsPlanPaths(path+"."+k, b[k], a[k])...)
		}
		sort.Strings(out)
		return out
	}
	bs, bok := before.([]any)
	as, aok := after.([]any)
	if bok && aok && len(bs) == len(as) {
		out := []string{}
		for i := range bs {
			out = append(out, changedSecretsPlanPaths(path+"[]", bs[i], as[i])...)
		}
		return out
	}
	return []string{path}
}
