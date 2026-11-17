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
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
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
	canonical := record.Applied.Intent
	if record.Observed.State == "runtime-retired" {
		canonical = json.RawMessage(record.Applied.GeneratedState["retired_intent"])
	}
	if json.Unmarshal(canonical, &intent) != nil || intent.ApplicationID != resolved.Manifest.ApplicationID || intent.Name != resolved.Manifest.Name || intent.Environment != resolved.Manifest.Environment {
		return nil, application.Manifest{}, errors.New("protected remote Application intent differs from selected deployment")
	}
	intent, err = retainedBackendPlacementProjection(resolved, intent, record.Applied.GeneratedState["backend_placements"])
	if err != nil {
		return nil, application.Manifest{}, err
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
	if err := deployment.EvaluateDeploymentAdmission(resolved.Target.DeploymentPolicy, nil); err != nil {
		return err
	}
	unlock, err := lockApplicationTargetLifecycle(resolved, false)
	if err != nil {
		return err
	}
	defer unlock()
	if err := authorizeApplicationProviderChanges(ctx, resolved, applicationInput(ctx, runtimeInput), out); err != nil {
		return err
	}
	// Technical limits do not suppress the current consequence/consent flow.
	// Unsupported intent still stops before transport, material or pending writes.
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
	var previousRuntime *application.RemoteManagedRuntime
	var previousIntent application.Manifest
	var previousRecord *deployment.DeploymentRecord
	if previous != nil {
		previousRuntime, previousIntent, err = restoreRemoteApplication(ctx, resolved)
		if err != nil {
			return err
		}
		old, loadErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
		if loadErr != nil {
			return loadErr
		}
		desiredDigest, fingerprintErr := repositoryDesiredStateFingerprint(ctx, resolved)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		if reflect.DeepEqual(previousIntent, resolved.Manifest) && old.Source.Digest == desiredDigest {
			if err := previousRuntime.ApplyApplication(ctx, previousIntent, true); err != nil {
				return err
			}
			return recordVerifiedRemoteReactivation(resolved, previousIntent)
		}
		// An altered source needs a new immutable remote publication. Reusing
		// the old project would falsely claim READY for bytes never delivered.
		previousRecord = &old
	}
	// A pure-client remote Application never bootstraps or binds a local Core.
	// Only an intent that needs a managed runtime broker (secrets, identity,
	// object storage or telemetry) requires the startup-bound Core's PKI; for
	// such an intent the issuer is reached through the connector route of the
	// bound target (the transport/scope bound at the head gate). A broker-less
	// workload never enters a local Core, so no issuer is taken and no Core
	// bootstrap is touched. This is the Dev-Remote pure-client boundary: the
	// bound target's connector route is the only Core handle this path may use.
	if err := runApplicationMemoryPreflight(ctx, applicationInput(ctx, runtimeInput), out, resolved, hostresource.EstimateApplication(resolved.Manifest), true); err != nil {
		return err
	}
	var issuer serviceaccess.Issuer
	if requiresManagedServiceIssuer(resolved.Manifest) {
		brokerIssuer, err := resolveApplicationServiceIssuer(ctx, resolved, nil)
		if err != nil {
			return err
		}
		status, err := brokerIssuer.Status(ctx)
		if err != nil {
			return err
		}
		if !status.Ready {
			return errors.New("bound Core service PKI is not ready")
		}
		issuer = brokerIssuer
	}
	// Operator confirmation before any candidate submission is persisted (same
	// fail-closed choke point as the local apply path).
	promptConfirmCandidateSubmission(ctx, resolved, out)
	pending, err := recordDeploymentBeforeMutation(ctx, resolved, "applying", candidateConsentOverride(ctx))
	if err != nil {
		return err
	}
	resolved.DeploymentRecord = &pending
	if err := application.RetainPendingProviderOwnersAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		return err
	}
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
		if previousRecord != nil {
			if err := runtime.SaveSnapshotRevision(resolved.stateRoot(), project); err != nil {
				return err
			}
		} else if err := runtime.SaveSnapshot(resolved.stateRoot(), project); err != nil {
			return err
		}
		pending.Applied.RemoteProject = &project
		return deployment.SaveDeploymentRecord(pending)
	}); err != nil {
		return err
	}
	if err := runtime.ApplyApplication(ctx, resolved.Manifest, false); err != nil {
		if previousRecord != nil {
			// The old publication is immutable and still retained. Attempt a
			// bounded, verified restoration before recovering the old receipt.
			if restoreErr := previousRuntime.ApplyApplication(ctx, previousIntent, true); restoreErr != nil {
				return errors.Join(err, fmt.Errorf("previous remote publication rollback failed: %w", restoreErr))
			}
			if recordErr := deployment.SaveDeploymentRecord(*previousRecord); recordErr != nil {
				return errors.Join(err, fmt.Errorf("restore previous Core deployment receipt: %w", recordErr))
			}
			return fmt.Errorf("new remote publication failed, previous deployment restored: %w", err)
		}
		return errors.Join(err, recordObservedDeployment(resolved, "failed", false))
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		return err
	}
	if err := recordAppliedDeployment(ctx, resolved, files); err != nil {
		return err
	}
	fmt.Fprintln(out, "[OK] remote Application converged and verified")
	return recordApplicationAudit(ctx, resolved, "apply", "success", "verified", "protected remote Application publication converged and verified")
}
