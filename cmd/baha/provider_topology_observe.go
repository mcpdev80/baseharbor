package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

type topologySource struct {
	name, project, compose, env string
	err                         error
}

// The same read-only observation is projected into status and doctor. Provider
// projects remain separate from application workloads and helpers keep roles.
func collectProviderTopologyChecks(ctx context.Context, runtime bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles) []application.StatusCheck {
	m := resolved.Manifest
	root, namespace := resolved.TargetStateRoot, resolved.Target.Name
	sources := []topologySource{{name: "application", project: files.Project, compose: files.Compose, env: files.Env}}
	if application.HasSharedBackends(m) {
		f := application.SharedBackendFilesAt(root, namespace, m.Environment)
		sources = append(sources, topologySource{name: "shared-backends", project: f.Project, compose: f.Compose, env: f.Env})
	}
	if application.HasObjectStorage(m) {
		f, e := objectstorage.ExistingProviderFilesAt(root, namespace)
		sources = append(sources, topologySource{"object-storage", f.Project, f.Compose, f.Env, e})
	}
	if application.HasOTLPTelemetry(m) {
		f, e := telemetry.ExistingProviderFilesAt(root, namespace)
		sources = append(sources, topologySource{"telemetry", f.Project, f.Compose, f.Env, e})
	}
	if application.HasMetricsSources(m) || application.HasRuntimeMetricsPermissions(m) {
		f, e := metricsprovider.ExistingProviderFilesAt(root, namespace, m)
		p, pe := metricsprovider.PlacementForAt(root, namespace, m)
		sources = append(sources, topologySource{"metrics", p.Project, f.Compose, f.Env, errors.Join(e, pe)})
	}
	if application.HasLogsCollection(m) {
		f, e := logsprovider.ExistingProviderFilesAt(root, namespace, m)
		p, pe := logsprovider.PlacementForAt(root, namespace, m)
		sources = append(sources, topologySource{"logs", p.Project, f.Compose, f.Env, errors.Join(e, pe)})
	}
	if application.HasTraceSignal(m) {
		f, p, e := traces.ExistingProviderFilesAt(root, namespace, m)
		sources = append(sources, topologySource{"traces", p.Project, f.Compose, f.Env, e})
	}
	if application.HasIdentity(m) {
		f, e := identityprovider.ExistingCoreRuntimeFiles(root, namespace)
		sources = append(sources, topologySource{"identity", f.Project, f.Compose, f.Env, e})
	}
	var result []application.StatusCheck
	for _, source := range sources {
		if errors.Is(source.err, os.ErrNotExist) {
			continue
		} // External/unmaterialized providers have their own semantic checks.
		if source.err != nil {
			result = append(result, application.StatusCheck{Name: source.name + "-topology", Detail: source.err.Error()})
			continue
		}
		running, err := runtime.RunningServicesProject(ctx, source.project, source.compose, source.env)
		if err != nil {
			result = append(result, application.StatusCheck{Name: source.name + "-topology", Detail: err.Error()})
			continue
		}
		observations, err := providertopology.Observe(source.compose, running, application.AvailabilityIntent(m))
		if err != nil {
			result = append(result, application.StatusCheck{Name: source.name + "-topology", Detail: err.Error()})
			continue
		}
		for _, o := range observations {
			result = append(result, application.StatusCheck{Name: fmt.Sprintf("%s/%s-topology", source.name, o.Provider), OK: o.Members > 0, Detail: o.Detail()})
		}
	}
	return result
}
