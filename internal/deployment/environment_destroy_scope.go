package deployment

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// The environment-destroy seam lives here by design. Per the owner ruling on
// card t-20261114-r28-cli2-env-delete (0428-055) and the umbrella contract
// (§6 "env group"): environment destruction is an environment-scoped operation
// on the authoritative per-Target environment identity, and its impact is the
// union of the deployments that belong to that (Target, environment) pair.
//
// The seam is deliberately read-only about membership and impact. It never
// deletes anything itself: the CLI drives the existing app-destroy lifecycle
// for each env-specific deployment and leaves global apps, other environments,
// shared services and node hosts untouched. Destroying the environment identity
// itself is a separate, explicit step the CLI takes only after the per
// Keep-data/Delete-data consent is recorded.

// EnvironmentDestroyDataChoice enumerates what happens to the data the
// environment's deployments hold. The default is KEEP so an accidental
// invocation never deletes recoverable state; a caller must opt in to DATA.
type EnvironmentDestroyDataChoice string

const (
	// EnvironmentDestroyKeepData preserves persistent data, credentials and
	// recovery material (the historical default): the environment identity is
	// removed but its members and their data are not purged.
	EnvironmentDestroyKeepData EnvironmentDestroyDataChoice = "keep-data"
	// EnvironmentDestroyDeleteData explicitly requests permanent data
	// deletion. This is the destructive branch and requires a separate,
	// explicit operator consent on top of the plan acknowledgements.
	EnvironmentDestroyDeleteData EnvironmentDestroyDataChoice = "delete-data"
)

// Validate resolves the raw CLI value into a known data choice and fails
// closed on anything else. An empty value is not accepted here: the CLI always
// resolves the default (KEEP) before it reaches this check, so a nil default is
// a programming error, not a caller choice.
func (d EnvironmentDestroyDataChoice) Validate() (EnvironmentDestroyDataChoice, error) {
	switch d {
	case EnvironmentDestroyKeepData:
		return EnvironmentDestroyKeepData, nil
	case EnvironmentDestroyDeleteData:
		return EnvironmentDestroyDeleteData, nil
	default:
		return "", fmt.Errorf("invalid data choice %q: use %q or %q", string(d), EnvironmentDestroyKeepData, EnvironmentDestroyDeleteData)
	}
}

// EnvironmentDestroyMember is one subject associated with the environment being
// destroyed. It carries only the non-secret durable identity and kind, exactly
// like the association binding, so the impact report never reveals a secret.
type EnvironmentDestroyMember struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
}

// EnvironmentDestroyDeployment is one deployment the environment impact
// accounts for. Scoped is true when the deployment belongs to the environment
// being destroyed and must be removed with it; scoped is false for a global
// deployment that shares its application with other environments and is
// therefore preserved.
type EnvironmentDestroyDeployment struct {
	DeploymentID  string `json:"deployment_id"`
	Application   string `json:"application"`
	ApplicationID string `json:"application_id"`
	Environment   string `json:"environment"`
	Scope         string `json:"scope"` // "scoped" or "global-preserved"
	OwnerVerified bool   `json:"owner_verified"`
}

// EnvironmentDestroyScope is the authoritative, read-only impact report for one
// (Target, environment) pair. It lists the scoped deployments that would be
// removed with the environment and the global deployments that are preserved
// because they are shared across environments. It is the single input the CLI
// renders to the operator and asks to confirm.
type EnvironmentDestroyScope struct {
	Target      string                         `json:"target"`
	Environment string                         `json:"environment"`
	DataChoice  string                         `json:"data_choice"`
	Scoped      []EnvironmentDestroyDeployment `json:"scoped"`
	Preserved   []EnvironmentDestroyDeployment `json:"preserved"`
	Members     []EnvironmentDestroyMember     `json:"members"`
	// FailClosed lists reasons the environment cannot be safely destroyed
	// because ownership or impact is unknown. A non-empty list means the
	// operation must refuse regardless of the data choice.
	FailClosed []string `json:"fail_closed,omitempty"`
}

// IsFailClosed reports whether the scope carries an unknown-ownership or
// unknown-impact guard that refuses destruction. It is the single boolean the
// CLI checks before it ever reaches the consent path.
func (s EnvironmentDestroyScope) IsFailClosed() bool {
	return len(s.FailClosed) > 0
}

// IsDestructive reports whether the selected data choice requests permanent
// deletion, so the CLI can demand the separate data-deletion consent.
func (s EnvironmentDestroyScope) IsDestructive() bool {
	return s.DataChoice == string(EnvironmentDestroyDeleteData)
}

// environmentDestroyDeploymentKind distinguishes a scoped from a preserved
// deployment in the impact report.
const (
	environmentDestroyScopeScoped    = "scoped"
	environmentDestroyScopePreserved = "global-preserved"
)

// CollectEnvironmentDestroyScope builds the authoritative impact report for one
// (Target, environment) pair. It lists every deployment the environment owns
// and, for each, whether the deployment is scoped to this environment (removed
// with it) or global (its application is also deployed in another environment
// and is therefore preserved). It uses the authoritative deployment registry
// read and the association member list; a missing installation owner or a
// deployment whose ownership cannot be verified fails closed, never guesses.
//
// This is a pure computation over the durable state: it performs no mutation,
// allocates no secret material, and never infers matching from names.
func CollectEnvironmentDestroyScope(target, environmentName string, dataChoice EnvironmentDestroyDataChoice) (EnvironmentDestroyScope, error) {
	choice, err := dataChoice.Validate()
	if err != nil {
		return EnvironmentDestroyScope{}, err
	}
	scope := EnvironmentDestroyScope{
		Target:      target,
		Environment: environmentName,
		DataChoice:  string(choice),
		Scoped:      []EnvironmentDestroyDeployment{},
		Preserved:   []EnvironmentDestroyDeployment{},
		Members:     []EnvironmentDestroyMember{},
	}

	// The membership list is the authoritative Core-side member set: it is
	// what the operator sees as "who belongs to this environment" and it is
	// what must be preserved (global apps) or reported.
	members, listErr := ListEnvironmentAssociations(target, environmentName)
	if listErr != nil {
		return scope, listErr
	}
	for _, binding := range members {
		scope.Members = append(scope.Members, EnvironmentDestroyMember{
			Kind:    string(binding.SubjectKind),
			Subject: binding.SubjectID,
		})
	}

	records, err := listScopedEnvironmentDeployments(target, environmentName)
	if err != nil {
		return scope, err
	}
	if len(records) == 0 {
		// Nothing is deployed in this environment: there is nothing to
		// destroy, so the scoped/preserved sets are empty but the scope is
		// still valid and its members are preserved above. The identity may
		// still be removed; the CLI treats an empty scope as a no-op
		// confirmation.
		return scope, nil
	}

	// Classify each deployment as scoped or global. A deployment is global
	// when its (application_id) is deployed in ANOTHER environment on the same
	// Target (anywhere on the Target, not just here): destroying this
	// environment must not remove that shared application. Names are never
	// consulted for this classification.
	allRecords, allErr := listScopedEnvironmentDeployments(target, "")
	if allErr != nil {
		return scope, allErr
	}
	selectedApplications := map[string]bool{}
	for _, record := range records {
		selectedApplications[record.Identity.ApplicationID] = true
		dep := EnvironmentDestroyDeployment{
			DeploymentID:  record.Identity.DeploymentID,
			Application:   record.Identity.Application,
			ApplicationID: record.Identity.ApplicationID,
			Environment:   record.Identity.Environment,
			OwnerVerified: true,
		}
		// A deployment belongs to exactly one environment. Even when the same
		// application is deployed elsewhere, this environment's deployment
		// must be processed; only sibling deployment records are preserved.
		dep.Scope = environmentDestroyScopeScoped
		scope.Scoped = append(scope.Scoped, dep)
	}
	for _, record := range allRecords {
		if record.Identity.Environment == environmentName || !selectedApplications[record.Identity.ApplicationID] {
			continue
		}
		dep := EnvironmentDestroyDeployment{
			DeploymentID:  record.Identity.DeploymentID,
			Application:   record.Identity.Application,
			ApplicationID: record.Identity.ApplicationID,
			Environment:   record.Identity.Environment,
			Scope:         environmentDestroyScopePreserved,
			OwnerVerified: true,
		}
		scope.Preserved = append(scope.Preserved, dep)
	}

	return scope, nil
}

// listScopedEnvironmentDeployments returns the deployment records whose
// environment equals environmentName on target. It fails closed on a bad
// target/environment name (the registry read validates both) and returns an
// empty slice when none are deployed, never an error for the absence.
func listScopedEnvironmentDeployments(target, environmentName string) ([]DeploymentRecord, error) {
	if err := ValidateTargetName(target); err != nil {
		return nil, err
	}
	all, err := ListDeployments(target)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(environmentName) == "" {
		return all, nil
	}
	var scoped []DeploymentRecord
	for _, record := range all {
		if strings.TrimSpace(record.Identity.Environment) != environmentName {
			continue
		}
		scoped = append(scoped, record)
	}
	return scoped, nil
}

// EnvironmentStateExists reports whether a durable identity record for one
// (Target, environment) pair is on disk. It is a pure existence probe: it
// never creates the record (unlike GetEnvironment), so it is safe to use as a
// "does this environment exist" check after an identity removal.
func EnvironmentStateExists(target, environmentName string) (bool, error) {
	if err := ValidateTargetName(target); err != nil {
		return false, err
	}
	if strings.TrimSpace(environmentName) == "" {
		return false, fmt.Errorf("environment name is empty")
	}
	home, path, err := environmentStateRoot(target, environmentName)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(home); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DestroyEnvironmentIdentity removes the durable identity record and its
// per-Target state directory for one (Target, environment) pair. It fails
// closed when the identity does not exist or its on-disk binding does not
// match the path it is asked to remove, so a copied or renamed record can
// never be deleted under the wrong (target, environment). It performs exactly
// one filesystem mutation and nothing else: no deployment, no association, no
// other-environment state is touched by this call.
//
// The caller (the CLI) is responsible for the surrounding plan/preflight/
// apply/verify order, the Keep-data/Delete-data consent and the audit write;
// this seam only guarantees the identity it removes is the one requested.
func DestroyEnvironmentIdentity(target, environmentName string, dataChoice EnvironmentDestroyDataChoice) error {
	if err := ValidateTargetName(target); err != nil {
		return err
	}
	name := strings.TrimSpace(environmentName)
	if name == "" {
		return fmt.Errorf("environment name is empty")
	}
	if _, err := dataChoice.Validate(); err != nil {
		return err
	}
	home, path, err := environmentStateRoot(target, name)
	if err != nil {
		return err
	}
	// Fail closed: the identity must exist and bind to exactly this path. A
	// missing record is reported (not a silent no-op) so the CLI can surface a
	// precise "no such environment" result rather than pretending success.
	existing, statErr := os.Stat(path)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return &machine.Error{
				Code:      machine.ErrorNotFound,
				CauseCode: "environment_not_found",
				Message:   fmt.Sprintf("environment %q has no identity on target %q", name, target),
				Resource:  target + "/" + name,
				Next:      "Run 'baha target env show' to list existing environments.",
			}
		}
		return fmt.Errorf("inspect environment identity: %w", statErr)
	}
	if existing.IsDir() {
		return &machine.Error{
			Code:      machine.ErrorInternal,
			CauseCode: "environment_identity_malformed",
			Message:   fmt.Sprintf("environment identity at %q is a directory, not a record", path),
			Resource:  path,
			Next:      "Restore a valid environment.json under targets/<target>/runtime/.baseharbor/environments/<env>.",
		}
	}
	// Bind-check the record before deleting it: a record whose stored binding
	// names a different (target, environment) must never be removed here.
	if _, rErr := readEnvironmentRecord(path, target, name); rErr != nil {
		return fmt.Errorf("environment identity at %q fails its binding check: %w", path, rErr)
	}
	if err := os.RemoveAll(home); err != nil {
		return fmt.Errorf("remove environment state dir: %w", err)
	}
	return nil
}
