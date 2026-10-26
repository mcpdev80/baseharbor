package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type sharedProviderResumeFixture struct {
	bhruntime.RuntimeProvider
	started []string
}

func (r *sharedProviderResumeFixture) ConfigProject(context.Context, string, string, string) error {
	return nil
}
func (r *sharedProviderResumeFixture) UpProject(_ context.Context, project, _, _ string) error {
	r.started = append(r.started, project)
	return nil
}

func TestControlPlaneDependencyStartDoesNotResumeRetiredSharedProviders(t *testing.T) {
	for _, only := range []bool{true, false} {
		t.Run(map[bool]string{true: "dependency-only", false: "explicit-platform"}[only], func(t *testing.T) {
			target := configureTestTarget(t)
			root, err := targetDataRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			files, err := telemetry.EnsureProviderFilesWithTraceBackendForEnvironmentAt(t.Context(), serviceissuer.New(t), "", "", "dev", root, target.Name)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(files.Env)
			if err != nil {
				t.Fatal(err)
			}
			runtime := &sharedProviderResumeFixture{}
			if err := resumeSharedPlatformRuntimeForSelection(t.Context(), runtime, &bytes.Buffer{}, only); err != nil {
				t.Fatal(err)
			}
			if only && len(runtime.started) != 0 {
				t.Fatal("broker dependency start resurrected retained unrequested Collector")
			}
			if !only && (len(runtime.started) != 1 || runtime.started[0] != files.Project) {
				t.Fatal("explicit platform resume no longer starts its retained module")
			}
			after, err := os.ReadFile(files.Env)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("provider restart rewrote original private material")
			}
		})
	}
}
