// Package deployment contains the damaged-state signal and namespace scanner
// for bug-hunt BF-03. DamagedDeploymentError and DamagedDeploymentState report
// a retained deploy directory whose authoritative deployment.json is missing
// with orphaned protected runtime state; DamagedDeploymentNamespace scans the
// deployments namespace so the repository up/edit path can detect the damage
// even when it re-derives a fresh per-deploy UUID.
package deployment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// workloadNetworkNameForIdentity derives the workload runtime network name from
// the retained application identity alone. The deploy directory is a per-deploy
// UUID carrying no application name, so the manifest/record identity is the only
// source of truth for the network name (never datum-rerei against disk):
// baseharbor-dev-workload-<project> where project is baseharbor-<app>-<env>.
func workloadNetworkNameForIdentity(applicationName, environment string) string {
	if strings.TrimSpace(applicationName) == "" || strings.TrimSpace(environment) == "" {
		return ""
	}
	return "baseharbor-dev-workload-baseharbor-" + applicationName + "-" + environment
}

// DamagedDeploymentError is returned when the protected deploy directory exists
// but its authoritative deployment.json is missing. This is the "damaged
// deployment state" from bug-hunt BF-03: the runtime directory tree is retained
// (networks, volumes, state files) while the record that proves ownership and
// recovery intent has been truncated or deleted. Recovery is explicit and never
// invented by a generic repository inspection.
type DamagedDeploymentError struct {
	Identity    DeploymentIdentity
	Root        string
	Network     bhruntime.RuntimeNetworkState
	NetworkName string
}

func (e *DamagedDeploymentError) Error() string {
	if e == nil {
		return "damaged deployment state error"
	}
	msg := fmt.Sprintf(
		"damaged deployment state: protected deploy directory %s exists but deployment.json is missing; "+
			"the record that proves ownership for %s/%s/%s cannot be verified",
		e.Root, e.Identity.Target, e.Identity.Application, e.Identity.Environment)
	// Name the runtime network half of the observed BF-03 damage when it can be
	// verified (network absent for THIS app's identity). An absent network is
	// reported together with the record; a present/not-managed network is not
	// attributed to this app and is omitted from the message.
	if e.Network == bhruntime.RuntimeNetworkStateAbsent && strings.TrimSpace(e.NetworkName) != "" {
		msg += fmt.Sprintf("; the workload runtime network %s for %s/%s/%s is also absent",
			e.NetworkName, e.Identity.Target, e.Identity.Application, e.Identity.Environment)
	}
	msg += "; recover it explicitly — " + e.NextAction()
	return msg
}

// NextAction is consumed by the CLI error classifier so the damaged-state
// error surfaces as a concrete internal_error machine error with the concrete
// recovery, before any generic repository-inspection budget check runs.
func (e *DamagedDeploymentError) NextAction() string {
	if e == nil {
		return ""
	}
	return "Recover it explicitly: re-up from the source repository so BaseHarbor re-establishes the record, or destroy-and-recreate with 'baha destroy --yes' then 'baha app up'. Do not delete the deploy directory to force a generic repository inspection."
}

// (DamagedDeploymentError).Unwrap keeps damaged-state detection compatible with
// errors.Is / errors.As helpers used by the CLI error layer.
func (e *DamagedDeploymentError) Unwrap() error { return nil }

// DamagedDeploymentState reports whether err is a damaged deployment record,
// unwrapping the signal returned by the deployment listing.
func DamagedDeploymentState(err error) (*DamagedDeploymentError, bool) {
	if err == nil {
		return nil, false
	}
	var damaged *DamagedDeploymentError
	if errors.As(err, &damaged) {
		return damaged, true
	}
	return nil, false
}

// DamagedDirectoryState is the directory-level primitive behind
// DamagedDeploymentState: it returns true when the deploy directory holds
// retained protected runtime state while deployment.json is missing.
//
// The presence of protected runtime artifacts (application.env, bindings,
// runtime.env, applied-*.sha256, etc.) alongside a deleted deployment.json is
// the bug-hunt BF-03 "damaged state": the runtime directory tree was
// materialized and ownership was recorded, then the record that proves recovery
// intent was truncated or deleted. A purely empty deploy directory is instead
// the pre-existing "incomplete" case (a partially initialized deployment that
// never recorded intent), so it is deliberately NOT flagged damaged and keeps
// the existing INCOMPLETE_DEPLOYMENT_STATE recovery path intact.
func DamagedDirectoryState(root string) (bool, error) {
	info, statErr := os.Stat(root)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect deploy directory for damaged state: %w", statErr)
	}
	if !info.IsDir() {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(root, "deployment.json")); !errors.Is(err, os.ErrNotExist) {
		// Record present (or unreadable) — not the truncated-state signature.
		return false, nil
	}
	// Damaged only when protected runtime state was already materialized and is
	// now orphaned without its record. An empty directory stays "incomplete".
	for _, name := range []string{"application.env", "bindings", "runtime.env", "applied-desired-state.sha256", "applied-control-state.sha256", "baseharbor.yaml"} {
		if _, err := os.Stat(filepath.Join(root, name)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return false, fmt.Errorf("inspect deploy directory for damaged state: %w", err)
		} else {
			return true, nil
		}
	}
	return false, nil
}

// DamagedDeploymentNamespace reports whether the deployments namespace of a
// Target holds any directory whose deployment.json is missing while protected
// runtime state was already materialized - the bug-hunt BF-03 "damaged
// deployment state". It scans the namespace rather than a single deployment
// UUID because the up/edit repository path (resolvedRepositoryApplication)
// re-derives a fresh per-deploy UUID when the authoritative record is gone, so
// the per-deploy DamagedDirectoryState can no longer locate the retained
// directory. applicationID and environment identify the repository app being
// resolved so a matching never-delivered one-shot proof (the legitimate absence
// of a record on a fresh init) can be excluded. It returns true when at least
// one damaged directory exists, along with that directory's path.
func DamagedDeploymentNamespace(targetName, applicationID, environment string) (bool, string, error) {
	deployDir, err := deploymentDirForTarget(targetName)
	if err != nil {
		return false, "", err
	}
	entries, readErr := os.ReadDir(deployDir)
	if errors.Is(readErr, os.ErrNotExist) {
		return false, "", nil
	}
	if readErr != nil {
		return false, "", fmt.Errorf("inspect deployment namespace for damaged state: %w", readErr)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		damaged, damageErr := DamagedDirectoryState(filepath.Join(deployDir, entry.Name()))
		if damageErr != nil {
			return false, "", damageErr
		}
		if !damaged {
			continue
		}
		// The never-delivered one-shot proof is stored at
		// never-delivered/<applicationID>/<environment>.json, keyed by the
		// application identity rather than the per-deploy directory name. When a
		// proof for this repository app exists, the missing record is a
		// legitimate fresh-init, not damage, so it is not flagged.
		proofPath, proofErr := neverDeliveredProofPath(targetName, applicationID, environment)
		if proofErr == nil {
			if _, statErr := os.Stat(proofPath); !errors.Is(statErr, os.ErrNotExist) {
				continue
			}
		}
		return true, filepath.Join(deployDir, entry.Name()), nil
	}
	return false, "", nil
}

// NetworkInspector is the minimal runtime surface the damaged-state scan needs:
// inspect a single runtime network and report its managed-owner state. It is
// satisfied by bhruntime.RuntimeProvider but declared here so the deployment
// package never imports the full runtime contract and tests can fake it.
type NetworkInspector interface {
	InspectNetwork(ctx context.Context, network string) (bhruntime.RuntimeNetworkState, error)
}

// DamagedDeploymentNamespaceScoped is the app-scoped variant of
// DamagedDeploymentNamespace used by resolvedRepositoryApplication: it flags the
// orphaned deploy directory exactly as DamagedDeploymentNamespace does, but when
// an app identity is supplied it additionally inspects THAT app's workload
// runtime network and reports the network-absent damage in addition to the
// record-absent damage.
//
// The deploy directory is a per-deploy UUID and carries no application name, so
// the workload network name (baseharbor-dev-workload-<project>) cannot be
// derived from the directory alone — the manifest/record that proves ownership
// is the source of truth. When runtime is nil OR the manifest identity is empty
// the caller falls back to the namespace-only scan (unchanged behaviour), so a
// runtime that cannot be queried never suppresses the record damage. When the
// network for THIS app is present (RuntimeNetworkStatePresent) the damaged
// directory is still flagged — the record damage alone is enough — but the
// returned damage carries only the record half. When the network for THIS app
// is absent the returned damage names BOTH halves. A network that is
// NotManaged (owned by another project) is not this app's damage and is not
// attributed to this app.
func DamagedDeploymentNamespaceScoped(
	targetName string,
	applicationID, environment string,
	appName string,
	runtime NetworkInspector,
) (damaged bool, root string, networkName string, networkDamage bhruntime.RuntimeNetworkState, inspectErr error) {
	damaged, root, err := DamagedDeploymentNamespace(targetName, applicationID, environment)
	if err != nil {
		return false, "", "", "", err
	}
	if !damaged || runtime == nil || strings.TrimSpace(appName) == "" {
		return damaged, root, "", "", nil
	}
	// Derive the workload project name from the retained app identity, not from
	// the UUID deploy directory (datumrei is forbidden).
	networkName = workloadNetworkNameForIdentity(appName, environment)
	if networkName == "" {
		// Could not derive a network name: report the record damage unchanged
		// and never fabricate a missing network.
		return damaged, root, "", "", nil
	}
	state, netErr := runtime.InspectNetwork(context.Background(), networkName)
	if netErr != nil {
		// Inspect failure on the resolved app's network must not be inferred as
		// absent and must not suppress the record damage; surface it so the CLI
		// can fail closed without misattributing.
		return damaged, root, networkName, bhruntime.RuntimeNetworkStateInspectError, fmt.Errorf(
			"inspect workload network %s for damaged deployment state: %w", networkName, netErr)
	}
	return damaged, root, networkName, state, nil
}

// DetectDamagedDeploymentState reports whether the protected deploy directory
// for id exists on disk with retained state while deployment.json is missing.
// A freshly unregistered deployment removes the whole directory (not damaged);
// a healthy deployment always carries deployment.json (not damaged).
func DetectDamagedDeploymentState(id DeploymentIdentity) (bool, error) {
	root, err := DeploymentRoot(id)
	if err != nil {
		return false, err
	}
	return DamagedDirectoryState(root)
}
