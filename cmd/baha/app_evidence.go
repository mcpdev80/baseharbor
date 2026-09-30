package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/evidence"
)

func appEvidenceCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "evidence",
		Summary: "Export secret-safe application lifecycle and verification evidence",
		Usage:   "baha app evidence [NAME] [-e ENV|--environment ENV] [-o json|--output json]",
		Long:    "Builds one deterministic, secret-safe evidence bundle from desired state, effective policy, observed runtime state, verification results, recovery metadata and bounded lifecycle audit records. JSON output is the generic export boundary; BaseHarbor does not provision a SIEM or vendor-specific evidence sink.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, environment, err := extractApplicationEnvironment(args, "evidence")
			if err != nil {
				return err
			}
			filtered, format, err := parseReadOutputArgs(filtered, "app evidence")
			if err != nil {
				return err
			}
			bundle, err := collectApplicationEvidence(ctx, store, filtered, environment)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, bundle)
			}
			renderApplicationEvidence(out, bundle)
			return nil
		},
	}
}

func collectApplicationEvidence(ctx context.Context, store application.Store, appArgs []string, environment string) (evidence.Bundle, error) {
	resolved, err := resolveApplicationEnvironment(ctx, store, appArgs, "evidence", environment)
	if err != nil {
		return evidence.Bundle{}, err
	}
	m := resolved.Manifest
	bundle := evidence.Bundle{
		Target:      resolved.Target.Name,
		Application: m.Name,
		Environment: m.Environment,
	}

	plan, err := application.BuildPlan(m)
	if err != nil {
		return evidence.Bundle{}, err
	}
	for _, action := range plan.Actions {
		bundle.Desired = append(bundle.Desired, evidence.Record{
			Kind: evidence.StateDesired, ID: action.Kind + ":" + action.Resource,
			Status: action.Kind, Resource: action.Resource, Detail: action.Description,
		})
	}

	policyResult, err := collectApplicationPolicy(ctx, store, appArgs, m.Environment)
	if err != nil {
		return evidence.Bundle{}, err
	}
	for _, rule := range policyResult.Rules {
		bundle.Enforced = append(bundle.Enforced, evidence.Record{
			Kind: evidence.StateEnforced, ID: "policy:" + rule.Code,
			Status: string(rule.Decision), Resource: rule.Code, Detail: rule.Description,
		})
	}
	for _, finding := range policyResult.Findings {
		bundle.Enforced = append(bundle.Enforced, evidence.Record{
			Kind: evidence.StateEnforced, ID: "finding:" + finding.Code + ":" + finding.Subject,
			Status: string(finding.Decision), Resource: finding.Subject, Detail: finding.Message,
		})
	}
	for _, override := range policyResult.Overrides {
		bundle.Exceptions = append(bundle.Exceptions, evidence.Record{
			Kind: evidence.StateException, ID: "policy-override:" + override,
			Status: "acknowledged", Resource: override, Detail: "bounded operator override is active",
		})
	}

	status, err := collectApplicationStatusResult(ctx, store, machineApplicationArgs(m.Name, m.Environment))
	if err != nil {
		return evidence.Bundle{}, err
	}
	bundle.Observed = append(bundle.Observed, evidence.Record{
		Kind: evidence.StateObserved, ID: "application", Status: status.State,
		Resource: status.Application, Detail: status.Project,
	})
	for _, check := range status.Checks {
		state := "failed"
		if check.OK {
			state = "ready"
		}
		bundle.Observed = append(bundle.Observed, evidence.Record{
			Kind: evidence.StateObserved, ID: "status:" + check.Name,
			Status: state, Resource: check.Name, Detail: check.Detail,
		})
	}
	authResource := status.OperatorAuth.Mode
	if status.OperatorAuth.Provider != "" {
		authResource += "/" + status.OperatorAuth.Provider
	}
	authDetail := "status=" + status.OperatorAuth.Status + "; session=" + status.OperatorAuth.Session
	if status.OperatorAuth.Principal != "" {
		authDetail += "; principal=" + status.OperatorAuth.Principal
	}
	bundle.Observed = append(bundle.Observed, evidence.Record{
		Kind: evidence.StateObserved, ID: "operator-auth",
		Status: strings.ToLower(status.OperatorAuth.Status), Resource: authResource, Detail: authDetail,
	})
	if status.RuntimeArtifact != nil {
		identity := status.RuntimeArtifact.Digest
		if identity == "" {
			identity = status.RuntimeArtifact.ImageID
		}
		bundle.Observed = append(bundle.Observed, evidence.Record{
			Kind: evidence.StateObserved, ID: "artifact:runtime-broker",
			Status: "observed", Resource: status.RuntimeArtifact.Reference, Detail: identity,
		})
	}

	doctor, err := collectApplicationDoctor(ctx, store, machineApplicationArgs(m.Name, m.Environment))
	if err != nil {
		return evidence.Bundle{}, err
	}
	for _, check := range doctor.Checks {
		state := "failed"
		if check.OK {
			state = "verified"
		}
		bundle.Verified = append(bundle.Verified, evidence.Record{
			Kind: evidence.StateVerified, ID: "doctor:" + check.Name,
			Status: state, Resource: check.Name, Detail: check.Detail,
		})
	}
	for _, workload := range doctor.Workload {
		state := "failed"
		if workload.Ready {
			state = "verified"
		}
		bundle.Verified = append(bundle.Verified, evidence.Record{
			Kind: evidence.StateVerified, ID: "workload:" + workload.Service,
			Status: state, Resource: workload.Service, Detail: workload.Detail,
		})
	}

	recovery, unsupported, exceptions, err := collectRecoveryEvidence(resolved)
	if err != nil {
		return evidence.Bundle{}, err
	}
	bundle.Recovery = recovery
	bundle.Unsupported = append(bundle.Unsupported, unsupported...)
	bundle.Exceptions = append(bundle.Exceptions, exceptions...)

	audit, err := evidence.Load(resolved.TargetStateRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return evidence.Bundle{}, err
	}
	bundle.Audit = evidence.Filter(audit, resolved.Target.Name, m.Name, m.Environment)
	return evidence.Seal(bundle)
}

func collectRecoveryEvidence(resolved resolvedApplication) (*evidence.Recovery, []evidence.Record, []evidence.Record, error) {
	var contributors []application.RecoveryContributorMetadata
	recovery := &evidence.Recovery{}

	backup, backupErr := resolved.Store.LastBackup(resolved.Manifest.Name)
	if backupErr == nil && backup.Environment == resolved.Manifest.Environment {
		created := backup.CreatedAt
		recovery.LastBackup = &created
		contributors = append([]application.RecoveryContributorMetadata(nil), backup.Recovery...)
	} else if backupErr != nil && !errors.Is(backupErr, application.ErrNoBackupMetadata) {
		return nil, nil, nil, backupErr
	}

	restored, restoreErr := resolved.Store.LastRecovery(resolved.Manifest.Name)
	if restoreErr == nil && restored.Environment == resolved.Manifest.Environment {
		at := restored.RestoredAt
		recovery.LastRestore = &at
		if len(restored.Recovery) > 0 {
			contributors = append([]application.RecoveryContributorMetadata(nil), restored.Recovery...)
		}
	} else if restoreErr != nil && !errors.Is(restoreErr, application.ErrNoRecoveryMetadata) {
		return nil, nil, nil, restoreErr
	}

	if len(contributors) == 0 {
		selection, err := applicationbackup.DiscoverManifestRecovery(resolved.Manifest)
		if err != nil {
			return nil, nil, nil, err
		}
		contributors = recoveryContributorMetadata(applicationbackup.RecoveryManifest{
			Version: applicationbackup.RecoveryManifestVersion, Contributors: selection.Contributors,
		}, false)
	}

	var unsupported []evidence.Record
	var exceptions []evidence.Record
	for _, contributor := range contributors {
		recovery.Contributors = append(recovery.Contributors, evidence.RecoveryContributor{
			StateClass: contributor.StateClass, LogicalResource: contributor.LogicalResource,
			Ownership: contributor.Ownership, Support: contributor.Support, Selected: contributor.Selected,
			Durable: contributor.Durable, ExplicitlyExcluded: contributor.ExplicitlyExcluded,
			Verified: contributor.Verified, Reason: contributor.Reason,
		})
		id := contributor.StateClass + ":" + contributor.LogicalResource
		switch contributor.Support {
		case string(applicationbackup.RecoveryUnsupported):
			unsupported = append(unsupported, evidence.Record{
				Kind: evidence.StateUnsupported, ID: "recovery:" + id,
				Status: "unsupported", Resource: contributor.LogicalResource,
				Capability: contributor.StateClass, Ownership: contributor.Ownership, Detail: contributor.Reason,
			})
		case string(applicationbackup.RecoveryExternal):
			exceptions = append(exceptions, evidence.Record{
				Kind: evidence.StateException, ID: "recovery:" + id,
				Status: "external", Resource: contributor.LogicalResource,
				Capability: contributor.StateClass, Ownership: contributor.Ownership, Detail: contributor.Reason,
			})
		default:
			if contributor.ExplicitlyExcluded {
				exceptions = append(exceptions, evidence.Record{
					Kind: evidence.StateException, ID: "recovery-excluded:" + id,
					Status: "excluded", Resource: contributor.LogicalResource,
					Capability: contributor.StateClass, Ownership: contributor.Ownership,
					Detail: "explicitly excluded from the selected recovery unit",
				})
			}
		}
	}
	return recovery, unsupported, exceptions, nil
}

func renderApplicationEvidence(out io.Writer, bundle evidence.Bundle) {
	fmt.Fprintf(out, "Evidence for %s / %s / %s\n", bundle.Target, bundle.Application, bundle.Environment)
	fmt.Fprintf(out, "  desired:      %d\n", len(bundle.Desired))
	fmt.Fprintf(out, "  enforced:     %d\n", len(bundle.Enforced))
	fmt.Fprintf(out, "  observed:     %d\n", len(bundle.Observed))
	fmt.Fprintf(out, "  verified:     %d\n", len(bundle.Verified))
	fmt.Fprintf(out, "  exceptions:   %d\n", len(bundle.Exceptions))
	fmt.Fprintf(out, "  unsupported:  %d\n", len(bundle.Unsupported))
	fmt.Fprintf(out, "  audit events: %d\n", len(bundle.Audit))
	if bundle.Recovery != nil {
		selected := 0
		verified := 0
		for _, contributor := range bundle.Recovery.Contributors {
			if contributor.Selected {
				selected++
			}
			if contributor.Verified {
				verified++
			}
		}
		fmt.Fprintf(out, "  recovery:     %d selected / %d verified\n", selected, verified)
	}
	fmt.Fprintf(out, "Integrity: %s:%s\n", strings.ToUpper(bundle.Integrity.Algorithm), bundle.Integrity.Digest)
	fmt.Fprintln(out, "Use '-o json' for the deterministic generic evidence export.")
}
