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
// unwrapping the signal returned by the deployment listing. When an inspector
// is supplied it additionally verifies THIS deployment's retained-identity
// workload network and enriches the error with the network half only when that
// network is genuinely absent for this app (never fabricated from an unrelated
// or failed inspect). A nil inspector leaves the error record-only, matching
// the pre-registry-fix behaviour of the listing path.
func DamagedDeploymentState(err error, inspector NetworkInspector) (*DamagedDeploymentError, bool) {
	if err == nil {
		return nil, false
	}
	var damaged *DamagedDeploymentError
	if errors.As(err, &damaged) {
		damaged.enrichNetworkForOwnIdentity(inspector)
		return damaged, true
	}
	return nil, false
}

// EnrichNetworkForOwnIdentity re-derives and, when genuinely absent for THIS
// deployment's retained identity, attaches the workload network half. The
// registry path returns a damaged error whose Identity is missing the retained
// Application/Environment (the deploy directory is a UUID with no record), so
// the caller populates those from the request parameters before calling this.
// Any non-absent or unverifiable outcome leaves the error record-only.
func (e *DamagedDeploymentError) EnrichNetworkForOwnIdentity(inspector NetworkInspector) {
	e.enrichNetworkForOwnIdentity(inspector)
}

// enrichNetworkForOwnIdentity verifies THIS deployment's retained-identity
// workload network and, when it is genuinely ABSENT for this app alone, records
// the network half. It is the registry/listing-path counterpart of
// DamagedDeploymentNamespaceScoped's app-scoped inspect: the deploy directory is
// a per-deploy UUID with no name, so the network name is derived from the
// identity's retained app/env (never from the directory), and the absent state
// is attributed to THIS app only when the inspector confirms it for that exact
// network. Any other outcome — present, not-managed, inspect error, missing
// identity, or a nil inspector — leaves the error record-only so a missing
// network is never inferred from an unverified or foreign inspect.
func (e *DamagedDeploymentError) enrichNetworkForOwnIdentity(inspector NetworkInspector) {
	if e == nil || inspector == nil {
		return
	}
	if strings.TrimSpace(e.Identity.Application) == "" || strings.TrimSpace(e.Identity.Environment) == "" {
		return
	}
	networkName := workloadNetworkNameForIdentity(e.Identity.Application, e.Identity.Environment)
	if networkName == "" {
		return
	}
	state, netErr := inspector.InspectNetwork(context.Background(), networkName)
	if netErr != nil {
		// An inspect failure is not proof of absence: surface it so the CLI can
		// fail closed, but never attribute a fabricated absent network to this
		// app. The record half is preserved.
		e.Network = bhruntime.RuntimeNetworkStateInspectError
		e.NetworkName = networkName
		return
	}
	if state != bhruntime.RuntimeNetworkStateAbsent {
		// Present (or not-managed): this app's network is not missing, so only
		// the record half is damage. Leave the network omitted.
		return
	}
	e.Network = state
	e.NetworkName = networkName
}

// Ownerless reports whether the damaged deployment's identity is unknown: the
// deploy directory is a per-deploy UUID with no retained deployment.json, so its
// owning application/environment cannot be read from any trusted source. Such a
// damaged directory is a target-level damaged-state conflict, never the
// requested application's record — the CLI must report the record half without
// attributing the requested app's record or network to a foreign/unknown-owner
// orphan. This is the bug-hunt BF-03 no-false-attribution invariant: a damaged
// entry is bound to the requested app ONLY when a trusted retained identity
// source proves that binding.
func (e *DamagedDeploymentError) Ownerless() bool {
	if e == nil {
		return false
	}
	return strings.TrimSpace(e.Identity.Application) == "" &&
		strings.TrimSpace(e.Identity.ApplicationID) == "" &&
		strings.TrimSpace(e.Identity.Environment) == ""
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

// RetainedOwnershipBandName is the protected file inside a damaged deploy
// directory that proves WHICH application a truncated deployment belonged to.
// The authoritative deployment.json is the record that BF-03 damage truncates;
// the band is the durable ownership marker that survives that deletion, so the
// up/edit resolver can still attribute the orphaned directory to a specific
// application instead of inventing one from the requesting manifest.
const RetainedOwnershipBandName = ".owner"

// LoadRetainedOwnerBand reads the retained ownership band from a damaged deploy
// directory and returns the application identity that production recorded when
// the deployment was materialized. It returns (application, environment, true)
// when a readable band is present, ("", "", false) when the directory carries
// no band (an unbound orphan), and an error only for an unreadable band.
//
// This is the durable ownership binding the bug-hunt r5 recipe requires: the
// deploy directory is a per-deploy UUID with no deployment.json, so its identity
// cannot be read from the authoritative record — the band is the source of
// truth, and a malformed/foreign orphan that never recorded a band yields false
// so the caller reports an explicit unknown-owner, record-only error.
func LoadRetainedOwnerBand(root string) (application, environment string, ok bool, err error) {
	data, readErr := os.ReadFile(filepath.Join(root, RetainedOwnershipBandName))
	if errors.Is(readErr, os.ErrNotExist) {
		return "", "", false, nil
	}
	if readErr != nil {
		return "", "", false, fmt.Errorf("inspect retained ownership band: %w", readErr)
	}
	// The band is two lines: the retained application identity on the first
	// line, the retained environment on the second. Anything malformed is not a
	// trusted ownership source.
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
		return "", "", false, nil
	}
	return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1]), true, nil
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
	// The scoped scan is the namespace-scope half of BF-03 / 0427-159: given the
	// requested app identity it flags the orphaned deploy directory and derives
	// THAT app's workload network name (source of truth: the retained manifest
	// identity passed in, never the UUID deploy directory). Deriving the network
	// name here is a namespace-scoped claim, not an ownership attribution, so it
	// does NOT require the durable ownership band. The band-gated ownership
	// attribution (the trust boundary that withholds any identity/network from an
	// unbound orphan) lives in the caller resolvedRepositoryApplication, which
	// gates on LoadRetainedOwnerBand before it names the requested app. Keeping
	// the band check OUT of the scoped scan preserves the namespace-scope
	// contract: the scan flags the damage and names the network for whatever
	// identity the caller resolved, and the CLI decides whether to attribute it.
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
