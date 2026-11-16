package metrics

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

func TestRuntimeSourceRejectsCallerControlledNetworkTargets(t *testing.T) {
	executor, err := NewRuntimeSourceExecutor("dev", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, parameters := range []map[string]any{
		{"port": 8080, "url": "http://169.254.169.254/"},
		{"port": 8080, "host": "internal-admin"},
		{"port": 8080, "service": "other"},
	} {
		_, err := executor.Execute(context.Background(), runtimeoperation.Request{
			Application:   "alpha",
			CallerService: "api",
			Capability:    RuntimeCapabilityV1,
			Operation:     "runtime.create",
			ResourceName:  "application",
			Parameters:    parameters,
		})
		if err == nil {
			t.Fatalf("unsafe runtime metrics parameters were accepted: %#v", parameters)
		}
	}
}

func TestRuntimeSourceUsesAuthenticatedServiceIdentityAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	executor, err := NewRuntimeSourceExecutor("dev", dir)
	if err != nil {
		t.Fatal(err)
	}
	request := runtimeoperation.Request{
		Application:   "alpha",
		CallerService: "api",
		Capability:    RuntimeCapabilityV1,
		Operation:     "runtime.create",
		ResourceName:  "application",
		Parameters:    map[string]any{"port": 8080, "path": "/metrics"},
	}
	created, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ResourceID, "res-metrics-") {
		t.Fatalf("resource id = %q", created.ResourceID)
	}
	if created.Binding["service"] != "api" || created.Binding["source_class"] != "application" {
		t.Fatalf("binding = %#v", created.Binding)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("target files = %d, want 1", len(entries))
	}
	data, err := os.ReadFile(dir + "/" + entries[0].Name())
	if err != nil {
		t.Fatal(err)
	}
	var groups []targetGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		t.Fatal(err)
	}
	wantTarget := application.MetricsTargetAliasFor("alpha", "dev", "api") + ":8080"
	if len(groups) != 1 || len(groups[0].Targets) != 1 || groups[0].Targets[0] != wantTarget {
		t.Fatalf("target groups = %#v, want target %q", groups, wantTarget)
	}

	get := request
	get.Operation = "runtime.get"
	got, err := executor.Execute(context.Background(), get)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResourceID != created.ResourceID || got.Binding["path"] != "/metrics" {
		t.Fatalf("get = %#v, created = %#v", got, created)
	}

	del := request
	del.Operation = "runtime.delete"
	if _, err := executor.Execute(context.Background(), del); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("runtime target not removed: entries=%#v err=%v", entries, err)
	}
}
