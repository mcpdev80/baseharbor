package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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

func TestRuntimeBrokerUsesInstallationPKIWithoutRestartingCore(t *testing.T) {
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	core := &brokerPKIReadinessRuntime{started: true, recover: true, stopPKI: cancel}
	workload := &coreBrokerReconcileFixture{stop: errors.New("workload must not receive Core reconciliation")}
	m := application.New("core-bound-broker", "dev", false, false, true)
	if !application.RequiresRuntimeBroker(m) {
		t.Fatal("fixture needs runtime broker")
	}
	files := brokerReadinessFiles(t)
	err := ensureAndStartRuntimeBrokerWithCore(ctx, io.Discard, workload, core, files, m, application.RuntimeFiles{})
	if !errors.Is(err, context.Canceled) || len(core.projects) != 4 || core.upCalls != 0 || len(workload.upProjects) != 0 {
		t.Fatal("broker restarted Core or forwarded its PKI to Application provider", err, core.upCalls, workload.upProjects)
	}
	for _, project := range core.projects {
		if project != files.Project {
			t.Fatal("PKI request used a different installation", project)
		}
	}
	if err := ensureAndStartRuntimeBrokerWithCore(context.Background(), io.Discard, workload, nil, files, m, application.RuntimeFiles{}); err == nil || len(workload.upProjects) != 0 {
		t.Fatal("missing Core authority fell back to Application provider", err)
	}
}
