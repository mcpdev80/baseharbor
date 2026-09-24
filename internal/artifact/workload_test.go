package artifact

import (
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/workload"
)

func TestResolveWorkloadSeparatesExistingImageAndSourceBuild(t *testing.T) {
	root := t.TempDir()
	model := workload.Model{Services: []workload.Service{
		{
			Name:  "demo-app",
			Build: &workload.Build{Context: "./demo-app"},
		},
		{
			Name:  "worker",
			Image: "ghcr.io/example/worker@sha256:deadbeef",
		},
	}}

	got, err := ResolveWorkload(root, model)
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete() {
		t.Fatal("source-backed workload unexpectedly reported complete")
	}
	if len(got.Builds) != 1 || got.Builds[0].Service != "demo-app" {
		t.Fatalf("build requests = %#v", got.Builds)
	}
	wantContext := filepath.Join(root, "demo-app")
	if got.Builds[0].Context != wantContext {
		t.Fatalf("build context = %q, want %q", got.Builds[0].Context, wantContext)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0].Reference != "ghcr.io/example/worker@sha256:deadbeef" {
		t.Fatalf("artifacts = %#v", got.Artifacts)
	}
	if got.Images()["worker"] == "" {
		t.Fatalf("image map = %#v", got.Images())
	}
}

func TestResolveWorkloadRejectsBuildContextOutsideRepository(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveWorkload(root, workload.Model{Services: []workload.Service{{
		Name:  "app",
		Build: &workload.Build{Context: "../outside"},
	}}})
	if err == nil {
		t.Fatal("escaping build context unexpectedly accepted")
	}
}
