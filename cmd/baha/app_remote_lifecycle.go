package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/hostresource"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func isRemoteApplication(resolved resolvedApplication) bool {
	return hasRemoteApplicationTarget(resolved)
}

// Until a provider has remote protocol readiness and cleanup adapters, reject
// its intent before creating state or staging artifacts. This is not permission
// to omit its release qualification or to route it through the Core host.
func checkRemoteApplicationLifecycle(manifest application.Manifest) error {
	remaining := manifest.Services
	remaining.SQL, remaining.Cache, remaining.KeyValue = false, false, false
	remaining.SQLInstances, remaining.CacheInstances, remaining.KeyValueInstances = nil, nil, nil
	if manifest.HA || !reflect.DeepEqual(remaining, application.Services{}) || application.HasSharedBackends(manifest) ||
		len(manifest.Exposures) != 0 || len(manifest.Secrets.Required) != 0 || len(manifest.Secrets.Optional) != 0 ||
		len(manifest.Runtime.Permissions) != 0 || len(manifest.Logs.Collect) != 0 || len(manifest.Metrics.Sources) != 0 || manifest.Telemetry.OTLP != nil {
		return machine.NewError(machine.ErrorCapabilityMissing, "Selected remote Application requires provider lifecycle adapters that are not yet qualified.", "Keep this deployment pending until its complete remote provider and workload contracts are supported.", false)
	}
	return manifest.Validate()
}

func restoreRemoteApplication(ctx context.Context, resolved resolvedApplication) (*application.RemoteManagedRuntime, application.Manifest, error) {
	transport, scope, err := remoteApplicationTransport(ctx, resolved, targetsession.PoolFromContext(ctx))
	if err != nil {
		return nil, application.Manifest{}, err
	}
	if _, err := targetsession.NewProjectRuntime(transport, scope); err != nil {
		return nil, application.Manifest{}, err
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return nil, application.Manifest{}, err
	}
	project, err := retainedRemoteProject(resolved)
	if err != nil {
		return nil, application.Manifest{}, err
	}
	if project == nil {
		return nil, application.Manifest{}, application.ErrRuntimeNotApplied
	}
	var intent application.Manifest
	if json.Unmarshal(record.Applied.Intent, &intent) != nil || intent.ApplicationID != resolved.Manifest.ApplicationID || intent.Name != resolved.Manifest.Name || intent.Environment != resolved.Manifest.Environment {
		return nil, application.Manifest{}, errors.New("protected remote Application intent differs from selected deployment")
	}
	if err := checkRemoteApplicationLifecycle(intent); err != nil {
		return nil, application.Manifest{}, err
	}
	runtime, err := application.RestoreRemoteManagedSnapshot(transport, scope, resolved.stateRoot(), *project)
	if err != nil {
		return nil, application.Manifest{}, err
	}
	if _, _, err := runtime.ApplicationPhases(intent); err != nil {
		return nil, application.Manifest{}, err
	}
	return runtime, intent, nil
}

func executeRemoteApplicationApply(ctx context.Context, resolved resolvedApplication, out io.Writer) error {
	if err := checkRemoteApplicationLifecycle(resolved.Manifest); err != nil {
		return err
	}
	transport, scope, err := remoteApplicationTransport(ctx, resolved, targetsession.PoolFromContext(ctx))
	if err != nil {
		return err
	}
	previous, err := retainedRemoteProject(resolved)
	if err != nil {
		return err
	}
	if previous != nil {
		runtime, intent, err := restoreRemoteApplication(ctx, resolved)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(intent, resolved.Manifest) {
			return errors.New("remote Application desired state differs from retained publication; inspect before changing its lifecycle")
		}
		if err := runtime.ApplyApplication(ctx, intent, true); err != nil {
			return err
		}
		return recordObservedDeployment(resolved, "ready", true)
	}
	if err := applicationCorePrerequisite(withTargetOverride(ctx, resolved.Target.Name), applicationInput(ctx, runtimeInput), out); err != nil {
		return err
	}
	if err := runApplicationMemoryPreflight(ctx, runtimeInput, out, resolved, hostresource.EstimateApplication(resolved.Manifest), true); err != nil {
		return err
	}
	coreRuntime, coreFiles, err := resolveApplicationCoreRuntime(ctx, resolved, nil)
	if err != nil {
		return err
	}
	issuer := openbao.NewServiceIssuer(coreRuntime, coreFiles)
	status, err := issuer.Status(ctx)
	if err != nil {
		return err
	}
	if !status.Ready {
		return errors.New("bound Core service PKI is not ready")
	}
	pending, err := recordDeploymentBeforeMutation(ctx, resolved, "applying")
	if err != nil {
		return err
	}
	resolved.DeploymentRecord = &pending
	files, err := application.EnsureRuntime(ctx, issuer, resolved.Store, resolved.Manifest)
	if err != nil {
		return err
	}
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil {
		return err
	}
	var selected *application.WorkloadFiles
	if found {
		selected = &workload
	}
	environment, err := application.RuntimeEnvironment(files)
	if err != nil {
		return err
	}
	runtime, err := application.NewRemoteApplicationRuntime(transport, scope, files, resolved.Manifest, selected, environment)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(resolved.stateRoot(), 0700); err != nil {
		return err
	}
	if err := runtime.Publish(ctx, func(project targetsession.ProjectRecord) error {
		if err := runtime.SaveSnapshot(resolved.stateRoot(), project); err != nil {
			return err
		}
		pending.Applied.RemoteProject = &project
		return deployment.SaveDeploymentRecord(pending)
	}); err != nil {
		return err
	}
	if err := runtime.ApplyApplication(ctx, resolved.Manifest, false); err != nil {
		return errors.Join(err, recordObservedDeployment(resolved, "failed", false))
	}
	if err := recordAppliedDeployment(ctx, resolved, files); err != nil {
		return err
	}
	fmt.Fprintln(out, "[OK] remote Application converged and verified")
	return recordApplicationAudit(ctx, resolved, "apply", "success", "verified", "protected remote Application publication converged and verified")
}
