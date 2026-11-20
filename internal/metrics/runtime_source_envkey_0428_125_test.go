package metrics

import (
	"context"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

// TestRuntimeSourceSeparatesTargetFilesPerEnvironment is the 0428-125 regression for
// hunt-918 Befund 7: the runtime-metrics target file key omitted the environment while
// the stored group labels it, so two environments of one application that share the
// broker's global targets directory collided on one file. The dev executor's source
// became unreadable through no action of its own (hard "runtime metrics target
// ownership mismatch" error), and one environment's resource identifier was
// indistinguishable from another environment's deployment.
func TestRuntimeSourceSeparatesTargetFilesPerEnvironment(t *testing.T) {
	dir := t.TempDir()
	request := func(operation string) runtimeoperation.Request {
		return runtimeoperation.Request{
			Application:   "alpha",
			CallerService: "api",
			Capability:    RuntimeCapabilityV1,
			Operation:     operation,
			ResourceName:  "application",
			Parameters:    map[string]any{"port": 8080, "path": "/metrics"},
		}
	}
	dev, err := NewRuntimeSourceExecutor("dev", dir)
	if err != nil {
		t.Fatal(err)
	}
	prod, err := NewRuntimeSourceExecutor("prod", dir)
	if err != nil {
		t.Fatal(err)
	}

	devCreate := request("runtime.create")
	created, err := dev.Execute(context.Background(), devCreate)
	if err != nil {
		t.Fatal(err)
	}
	devPath := dev.targetPath(devCreate)

	prodCreate := devCreate
	prodResource, err := prod.Execute(context.Background(), prodCreate)
	if err != nil {
		t.Fatal(err)
	}
	prodPath := prod.targetPath(prodCreate)
	if devPath == prodPath {
		t.Fatalf("environment key asymmetry: dev and prod share target file %q", devPath)
	}
	if created.ResourceID == prodResource.ResourceID {
		t.Fatalf("environment key asymmetry: identical resource id %q for dev and prod", created.ResourceID)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("target files = %d, want 2 (one per environment)", len(entries))
	}

	get := devCreate
	get.Operation = "runtime.get"
	got, err := dev.Execute(context.Background(), get)
	if err != nil {
		t.Fatalf("dev read after unrelated prod write: %v", err)
	}
	if got.ResourceID != created.ResourceID {
		t.Fatalf("dev get = %q, created = %q", got.ResourceID, created.ResourceID)
	}

	del := devCreate
	del.Operation = "runtime.delete"
	if _, err := dev.Execute(context.Background(), del); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(devPath); !os.IsNotExist(err) {
		t.Fatalf("dev target file still present after dev delete: %v", err)
	}
	if _, err := os.Stat(prodPath); err != nil {
		t.Fatalf("prod target file must survive the dev delete: %v", err)
	}
}
