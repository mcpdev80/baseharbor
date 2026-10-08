package main

import (
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"io"
	"testing"
)

type coreBrokerReconcileFixture struct {
	bhruntime.RuntimeProvider
	upProjects []string
	stop       error
}

func (r *coreBrokerReconcileFixture) UpProject(_ context.Context, project, _, _ string) error {
	r.upProjects = append(r.upProjects, project)
	return r.stop
}

func TestRuntimeBrokerReconcilesCoreOnInstallationProvider(t *testing.T) {
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")
	stop := errors.New("stop after Core reconciliation")
	core := &coreBrokerReconcileFixture{stop: stop}
	workload := &coreBrokerReconcileFixture{stop: errors.New("workload must not receive Core reconciliation")}
	m := application.New("core-bound-broker", "dev", false, false, true)
	if !application.RequiresRuntimeBroker(m) {
		t.Fatal("fixture needs runtime broker")
	}
	files := bhruntime.Files{Project: "installation-core", Compose: "/core/compose.yaml", Env: "/core/runtime.env"}
	err := ensureAndStartRuntimeBrokerWithCore(context.Background(), io.Discard, workload, core, files, m, application.RuntimeFiles{})
	if !errors.Is(err, stop) || len(core.upProjects) != 1 || core.upProjects[0] != files.Project || len(workload.upProjects) != 0 {
		t.Fatal("Core project was forwarded to Application provider", err, core.upProjects, workload.upProjects)
	}
	if err := ensureAndStartRuntimeBrokerWithCore(context.Background(), io.Discard, workload, nil, files, m, application.RuntimeFiles{}); err == nil || len(workload.upProjects) != 0 {
		t.Fatal("missing Core authority fell back to Application provider", err)
	}
}
