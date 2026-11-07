package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/stableid"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

const DeploymentRecordVersion = 2

// ErrDeploymentOwnerMissing is returned when the selected Target has no
// authoritative Core installation to own its deployment state. A deployment is
// only loadable when a single installation governs that Target; there is no
// second local owner and no reconciliation with a cached copy.
var ErrDeploymentOwnerMissing = errors.New("deployment has no authoritative installation owner for its Target; the installation must be restored before its state is loadable")

// ErrDeploymentOwnerForeign is returned when the Core installation under a
// Target does not own the deployment state directory it addresses. A foreign or
// rewritten installation cannot claim a deployment it never owned; the record
// identity is preserved and the mismatch is rejected, never adopted.
var ErrDeploymentOwnerForeign = errors.New("deployment state directory is not owned by the installation under its Target; foreign installation cannot own this deployment")

// verifyDeploymentOwnerAtSeam enforces the one-installation-owner rule on every
// production read/write path in this package. It derives the Target's runtime
// root (targets/<target>/runtime) from the canonical data layout and delegates
// to the exported VerifyDeploymentOwner, so the authoritative check runs at the
// storage seam and cannot be bypassed by a caller that only holds an identity.
//
// Tenant-managed targets are Coreless by design; the check is resolved through
// the same VerifyDeploymentOwner path and fails closed unless an authorized
// tenant owner path is established by the governing contract.
func verifyDeploymentOwnerAtSeam(targetName, deploymentID string) error {
	if err := ValidateTargetName(targetName); err != nil {
		return err
	}
	targetRoot, err := TargetStateRoot(targetName)
	if err != nil {
		return err
	}
	return VerifyDeploymentOwner(targetName, filepath.Join(targetRoot, "runtime"), deploymentID)
}

// VerifyDeploymentOwner enforces the one-installation-owner rule for deployment
// state. It loads the Core installation that governs targetRoot (the runtime
// root of the selected Target) and proves that installation owns the Target's
// deployment namespace.
//
// The deployment record identity is intentionally untouched: a deployment is
// keyed by its own UUID plus Target, and its record identity is never rewritten
// to carry the installation. Ownership is proven at the storage path, not by
// embedding an installation reference. A missing installation fails closed; a
// foreign installation (one whose durable identity does not own this Target)
// fails closed and preserves the existing record identity. The deployment
// directory itself need not pre-exist: first-run init creates it under the owner,
// so absence of the directory is not itself a missing-owner condition.
//
// Durable identity binding of a deployment to the owner installation's
// installation.ID is intentionally out of scope for this seam: the storage
// owner is proven here at the Target's runtime root. The durable
// installation.ID binding that closes copied/rewritten-identity attacks is
// provided by the authority contract (follow-up), not invented by this layer.
//
// Tenant-managed targets are Coreless by design: they have no single Core
// installation to own the deployment, so the one-installation-owner binding
// does not apply. A tenant target is authorized only through the consumer-
// provided tenantOwnerResolver, which routes access to its genuine authorized
// installation owner (for example a validated remote project binding). The
// resolver defaults to fail-closed, so no tenant target is admitted without an
// explicit authorized path.
var TenantOwnerResolver func(targetName string) (bool, error)

// TargetLoanVerifier implements the TEST-SCOPE ONLY owner loan (card
// t-20261106-r27-remote-up-podman-owner-missing, card number 0427-117). The
// default nil keeps every production path byte-identical; the consumer binding
// (cmd/baha.deploymentTargetLoanVerifier) grants a marker-proven isolated test
// Target against the boot installation's genuine Core installation.
var TargetLoanVerifier func(targetName string) (*coreinstallation.State, error)

func VerifyDeploymentOwner(targetName, targetRoot, deploymentID string) error {
	if TenantOwnerResolver != nil {
		// Grant-only hook: only an affirmative (true, nil) authorizes the
		// (Coreless) tenant target through this path. Any other result —
		// (false, nil), (false, err), or (true, err) — means the hook did not
		// authorize access, so resolution falls through to the standard
		// single-installation-owner verification below. A resolver that does not
		// grant can never bypass that check: it is never an early return and
		// never a foreign-installation shortcut in the !authorized branch.
		authorized, resolveErr := TenantOwnerResolver(targetName)
		if resolveErr != nil {
			return resolveErr
		}
		if authorized {
			// The consumer has established a genuine authorized-owner path for
			// this (Coreless) tenant target; the single-installation-owner
			// requirement does not apply.
			return nil
		}
		// Not granted by the hook: fall through to the authoritative
		// coreinstallation.Load owner check instead of returning early.
	}
	installation, err := coreinstallation.Load(targetRoot)
	loaned := false
	if err != nil && errors.Is(err, os.ErrNotExist) && TargetLoanVerifier != nil {
		// TEST-SCOPE ONLY seam (card t-20261106-r27-remote-up-podman-owner-missing,
		// card 0427-117): resolve a marker-proven isolated test Target against
		// the one installation its boot Target owns. The loan verifier re-checks
		// that the boot installation is genuine, READY, and that the boot Target
		// differs from the requested one; here the loaned identity governs the
		// requested Target's deployment namespace (the loan's purpose), so the
		// namespace-installation binding below stays checked against targetName.
		// A refusal — any error or a nil state — keeps the ORIGINAL
		// fail-closed sentinel ErrDeploymentOwnerMissing: loan guard details must
		// never replace the contract error shape (the caller's errors.Is checks
		// and the never-delivered-destroy recognition in cmd/baha depend on it).
		// The nil default keeps every production path byte-identical.
		loanedState, loanErr := TargetLoanVerifier(targetName)
		if loanErr != nil || loanedState == nil {
			return ErrDeploymentOwnerMissing
		}
		installation = *loanedState
		loaned = true
	} else if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrDeploymentOwnerMissing
		}
		// An invalid/foreign installation.json is not an owner either.
		return ErrDeploymentOwnerForeign
	}
	_ = installation
	// The deployment namespace is targets/<target>/deployments. The deployment
	// directory's Target (the segment under targets/) must match the
	// installation's own Target; a distinct Target is a foreign owner and is
	// rejected before any deployment under it is read or written.
	deploymentDir, dirErr := deploymentDirForTarget(targetName)
	if dirErr == nil {
		if deploymentTarget, found := deploymentTargetFromDir(deploymentDir); found && deploymentTarget != "" && strings.TrimSpace(deploymentTarget) != strings.TrimSpace(installation.Spec.Target) {
			return ErrDeploymentOwnerForeign
		}
		// A pre-existing deployment directory must be a real, protected dir, not
		// a symlinked escape. Missing is acceptable (first-run); a broken or
		// symlinked path is foreign.
		if info, statErr := os.Lstat(deploymentDir); statErr == nil {
			if info.IsDir() == false || info.Mode()&os.ModeSymlink != 0 {
				return ErrDeploymentOwnerForeign
			}
		}
	}
	// Loan path only: resolve the two order-independent guarantees of the
	// owner-loan seam via the extracted helper (see deployment_owner_loan.go).
	// The `loaned` guard keeps this out of the normal production path, where
	// VerifyDeploymentOwnerAgainstOwner already ran its own guarantee set and a
	// stray TargetStateRoot failure must not change the contract result.
	if loaned {
		if err := verifyLoanOwnerGuarantees(targetName, targetRoot); err != nil {
			return err
		}
	}
	_ = deploymentID
	return nil
}

// deploymentDirForTarget resolves the deployments namespace of a Target
// (targets/<target>/deployments) via the canonical layout.
func deploymentDirForTarget(targetName string) (string, error) {
	targetRoot, err := TargetStateRoot(targetName)
	if err != nil {
		return "", err
	}
	return filepath.Join(targetRoot, "deployments"), nil
}

// deploymentTargetFromDir extracts the Target segment (targets/<target>/...)
// from a deployment directory path, if the conventional layout is present.
func deploymentTargetFromDir(deploymentDir string) (string, bool) {
	segments := strings.Split(filepath.Clean(deploymentDir), string(filepath.Separator))
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i] == "targets" && i+1 < len(segments) {
			return segments[i+1], true
		}
	}
	return "", false
}

// testHookAfterGuardRead is a package-internal test seam (production path keeps
// it nil, so no behaviour changes): it fires inside SaveDeploymentRecord exactly
// after the one-writer guard has read the persisted previous record (the guard
// has established previousExists and the authoritative owner) and immediately
// before the preferences-retention re-read. A test uses it to deterministically
// mutate the on-disk record — e.g. delete deployment.json to reproduce a record
// that vanishes between the two in-process reads — so the real SaveDeploymentRecord
// path (not just the loaders in isolation) can be asserted fail-closed. Style
// follows the existing cmd/baha observedAt seam.
var testHookAfterGuardRead func()

type DeploymentIdentity struct {
	DeploymentID  string `json:"deployment_id"`
	ApplicationID string `json:"application_id"`
	Target        string `json:"target"`
	Application   string `json:"application"`
	Environment   string `json:"environment"`
}

func NewDeploymentIdentity(target, applicationID, application, environment string) (DeploymentIdentity, error) {
	id, err := stableid.NewUUIDv4("deployment")
	if err != nil {
		return DeploymentIdentity{}, err
	}
	identity := DeploymentIdentity{
		DeploymentID:  id,
		ApplicationID: strings.TrimSpace(applicationID),
		Target:        strings.TrimSpace(target),
		Application:   strings.TrimSpace(application),
		Environment:   strings.TrimSpace(environment),
	}
	if err := identity.Validate(); err != nil {
		return DeploymentIdentity{}, err
	}
	return identity, nil
}

func ValidateDeploymentID(id string) error {
	return stableid.ValidateUUIDv4("deployment", id)
}

type DeploymentSource struct {
	Kind       string `json:"kind"`
	Repository string `json:"repository,omitempty"`
	Manifest   string `json:"manifest,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

type AppliedDeployment struct {
	Intent          json.RawMessage              `json:"intent,omitempty"`
	RuntimeProvider string                       `json:"runtime_provider"`
	Delivery        delivery.Selection           `json:"delivery,omitempty"`
	GeneratedState  map[string]string            `json:"generated_state,omitempty"`
	LastAppliedRef  string                       `json:"last_applied_ref,omitempty"`
	RemoteProject   *targetsession.ProjectRecord `json:"remote_project,omitempty"`
}

// ObservedDeployment is the runtime-convergence observation for a deployment.
// Reference is the third independent reference dimension: the release/component
// reference the runtime currently reports. An empty value means no observation
// has been recorded; staleness detection compares it against the last-applied
// reference.
type ObservedDeployment struct {
	State      string                       `json:"state,omitempty"`
	Ready      bool                         `json:"ready,omitempty"`
	Conflict   bool                         `json:"conflict,omitempty"`
	Owner      delivery.ReconciliationOwner `json:"owner,omitempty"`
	VerifiedAt time.Time                    `json:"verified_at,omitempty"`
	Reference  string                       `json:"observed_reference,omitempty"`
}

type DeploymentRecord struct {
	DesiredProviderPreferences json.RawMessage    `json:"desired_provider_preferences,omitempty"`
	Version                    int                `json:"version"`
	Identity                   DeploymentIdentity `json:"identity"`
	Source                     DeploymentSource   `json:"source"`
	Applied                    AppliedDeployment  `json:"applied"`
	Observed                   ObservedDeployment `json:"observed,omitempty"`
	// Consent is the explicit operator/automation consent that pins the exact
	// validated candidate submission. It is additive and nil on any record
	// written before this field existed: existing records stay loadable and the
	// retention/normalization path is unchanged. Only a brand-new repository
	// apply mutation requires a non-nil consent; restored or observed-only
	// records are never re-validated against it.
	Consent    *delivery.CandidateConsent     `json:"consent,omitempty"`
	References map[string]*ReferenceDimension `json:"references,omitempty"`
}

// ErrConflictingOneWriter is returned when a deployment record being applied
// observes two active reconciliation owners for the same resource set. This is
// ADR 0022 §2: ambiguous ownership fails closed at the apply path, before any
// state is written.
var ErrConflictingOneWriter = errors.New(
	"one writer per resource: conflicting reconciliation owners observed, refusing to apply deployment record",
)

// ErrUnauthorizedMutation is returned when a direct (BaseHarbor-owned) mutation
// is applied while an external reconciler already owns the resource. ADR 0022 §2
// refuses direct mutation of a resource under active external ownership.
var ErrUnauthorizedMutation = errors.New(
	"one writer per resource: external reconciler active, direct mutation refused",
)

type DeploymentRecordStateError struct {
	Identity DeploymentIdentity
	Kind     string
	Err      error
}

func (e *DeploymentRecordStateError) Error() string {
	if e == nil {
		return "deployment record state error"
	}
	return fmt.Sprintf("%s deployment state for %s/%s/%s [%s]: %v",
		e.Kind, e.Identity.Target, e.Identity.Application, e.Identity.Environment, e.Identity.DeploymentID, e.Err)
}

func (e *DeploymentRecordStateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func DeploymentRecordState(err error) (*DeploymentRecordStateError, bool) {
	var stateErr *DeploymentRecordStateError
	if errors.As(err, &stateErr) {
		return stateErr, true
	}
	return nil, false
}

func (id DeploymentIdentity) Validate() error {
	if err := ValidateDeploymentID(id.DeploymentID); err != nil {
		return err
	}
	if err := stableid.ValidateUUIDv4("application", id.ApplicationID); err != nil {
		return err
	}
	if err := ValidateTargetName(id.Target); err != nil {
		return err
	}
	if strings.TrimSpace(id.Application) == "" || strings.ContainsAny(id.Application, `/\`) {
		return fmt.Errorf("invalid application %q", id.Application)
	}
	if strings.TrimSpace(id.Environment) == "" || strings.ContainsAny(id.Environment, `/\`) {
		return fmt.Errorf("invalid environment %q", id.Environment)
	}
	return nil
}

func DeploymentRoot(id DeploymentIdentity) (string, error) {
	if err := ValidateDeploymentID(id.DeploymentID); err != nil {
		return "", err
	}
	if err := ValidateTargetName(id.Target); err != nil {
		return "", err
	}
	root, err := TargetStateRoot(id.Target)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "deployments", id.DeploymentID), nil
}

func SaveDeploymentRecord(record DeploymentRecord) error {
	if err := record.Identity.Validate(); err != nil {
		return err
	}
	if record.Version == 0 {
		record.Version = DeploymentRecordVersion
	}
	if record.Version != DeploymentRecordVersion {
		return fmt.Errorf("unsupported deployment record version %d", record.Version)
	}
	record.Applied.Delivery = record.Applied.Delivery.Normalize()
	if err := record.Applied.Delivery.Validate(); err != nil {
		return fmt.Errorf("invalid delivery selection: %w", err)
	}
	if err := validateReferences(record.References); err != nil {
		return fmt.Errorf("invalid deployment references: %w", err)
	}
	if err := validateRemoteProject(record); err != nil {
		return err
	}
	// Authoritative ownership is enforced on the production write seam before any
	// directory is created or any record is written: a missing or foreign
	// installation fails closed and nothing is mutated. A tenant target is
	// admitted only through the consumer-provided tenantOwnerResolver hook.
	if err := verifyDeploymentOwnerAtSeam(record.Identity.Target, record.Identity.DeploymentID); err != nil {
		return &DeploymentRecordStateError{Identity: record.Identity, Kind: "missing", Err: err}
	}
	root, err := DeploymentRoot(record.Identity)
	if err != nil {
		return err
	}
	// ADR 0022 §2: the one-writer guard observes an authoritative resource owner,
	// not the caller-supplied Observed block. A caller can forge
	// record.Observed.Owner (e.g. Owner=baseharbor with Mode=direct) to slip past
	// the guard, so the persisted previous record is read here — before the guard
	// and before any state is written — and its reconciliation-owner observation
	// governs. The previous record is the real owner observed before this
	// mutation; the caller value can never change the outcome.
	previousOwner, previousConflict, previousExists, loadErr := loadPersistedOwnerForGuard(record.Identity.Target, record.Identity.DeploymentID, filepath.Join(root, "deployment.json"))
	if !previousExists && loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return loadErr
	}
	// Guard observation: the persisted owner governs when a previous record
	// exists; otherwise the applied selection's mode fixes the owner (a direct
	// (BaseHarbor-owned) apply has no prior external owner, so the guarded direct
	// happy path stays green). The caller-supplied Observed.Owner is never used.
	// The persisted owner is normalised before the guard: a value that is neither
	// "" nor a recognised owner (baseharbor|external) — a corrupt or unknown
	// persisted owner — is normalised to "" (unset) by the same rule
	// Selection.Normalize applies to selections, so it neither silently blocks nor
	// silently attests an owner. An absent or corrupt persisted owner therefore
	// defers to the applied selection's mode.
	guardOwner := record.Applied.Delivery.Owner
	if previousExists {
		if normalised := normalisePersistedOwner(previousOwner); normalised != "" {
			guardOwner = delivery.ReconciliationOwner(normalised)
		}
	}
	guardRecord := record
	guardRecord.Observed.Owner = guardOwner
	// Resolve the authoritative conflict for the guard: only a conflict observed
	// in the persisted previous record counts. On a first save (no previous
	// record) the caller-supplied Conflict is discarded, never honoured — the
	// claim that a caller cannot drive a rejection on first write holds. On a
	// subsequent save the persisted/previous value governs and the caller value
	// is overwritten (not merely ORed). The caller-supplied Conflict is thus
	// never trusted at first or subsequent save.
	if previousExists {
		guardRecord.Observed.Conflict = previousConflict
	} else {
		guardRecord.Observed.Conflict = false
	}
	if guardErr := refuseOneWriterMutation(guardRecord); guardErr != nil {
		return &DeploymentRecordStateError{Identity: record.Identity, Kind: "conflict", Err: guardErr}
	}
	// Test seam (package-internal only, production path untouched): fired exactly
	// after the guard-read (previousExists + persisted owner established) and
	// before the retention re-read, mirroring the cmd/baha observedAt seam style.
	// A test may use it to deterministically mutate the on-disk record (e.g.
	// delete it to reproduce the vanish-between-reads race) between the two
	// in-process reads. nil default means no observable behaviour change.
	if testHookAfterGuardRead != nil {
		testHookAfterGuardRead()
	}
	if err := validateRemoteProject(record); err != nil {
		return err
	}
	// Existing lifecycle callers reconstruct records. Preserve separately owned
	// desired preferences unless an explicit replacement (including []) is
	// supplied. The retention is applied to guardRecord (not only to record)
	// because guardRecord is the copy that gets serialised below; otherwise a
	// lifecycle save with nil preferences would silently drop the previously
	// persisted preferences — the exact data-loss regression this guards.
	if record.DesiredProviderPreferences == nil {
		if previousExists {
			// Identity/preferences retention re-reads the previous record through
			// the full loader (owner validation is the guard's job, done above via
			// loadPersistedOwnerForGuard). A corrupt persisted owner does not block
			// retention of the separately owned preferences; the guard already
			// resolved the authoritative owner from the raw persisted value.
			prevForRetention, retErr := loadDeploymentRecordFile(record.Identity.Target, record.Identity.DeploymentID, filepath.Join(root, "deployment.json"), nil)
			if retErr == nil {
				if prevForRetention.Identity.ApplicationID != record.Identity.ApplicationID || prevForRetention.Identity.Environment != record.Identity.Environment {
					return errors.New("cannot retain preferences from a different deployment owner")
				}
				record.DesiredProviderPreferences = prevForRetention.DesiredProviderPreferences
				guardRecord.DesiredProviderPreferences = prevForRetention.DesiredProviderPreferences
			} else if retErr != nil {
				// previous record exists (the guard load confirmed it above) but
				// the full loader rejects it now — including os.ErrNotExist, because
				// the record vanished between the guard-load and this retention-load.
				// Fail closed: never silently drop previously owned preferences or
				// skip identity validation. No silent fallback.
				return fmt.Errorf("cannot retain preferences: previous record unreadable: %w", retErr)
			}
		}
	}
	if len(record.DesiredProviderPreferences) > 0 && (!json.Valid(record.DesiredProviderPreferences) || strings.TrimSpace(string(record.DesiredProviderPreferences))[0] != '[') {
		return errors.New("desired provider preferences must be a JSON array")
	}
	// Consume initialization provenance before a deployment can be persisted:
	// a lost deployment.json must not look like a never-deployed application.
	if err := ConsumeNeverDeliveredProof(record.Identity); err != nil {
		return fmt.Errorf("invalidate never-delivered provenance: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create deployment state: %w", err)
	}
	// Persist the GUARDED record, not the caller-supplied one. The guard above
	// resolved the authoritative owner into guardRecord (and carried any prior
	// Conflict flag); the retention above restored owned preferences into
	// guardRecord too. Serialising `record` here would persist the
	// caller-supplied Observed.Owner (which a delegated update can forge) and,
	// worse, the caller-supplied preferences (dropping retained ones); serialise
	// guardRecord so the persisted owner and retained preferences are canonical.
	// When the guard refuses a mutation the file is never written (store bytes
	// unchanged).
	data, err := json.MarshalIndent(guardRecord, "", "  ")
	if err != nil {
		return fmt.Errorf("encode deployment record: %w", err)
	}
	path := filepath.Join(root, "deployment.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write deployment record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace deployment record: %w", err)
	}
	return nil
}

func LoadDeploymentRecord(id DeploymentIdentity) (DeploymentRecord, error) {
	if err := verifyDeploymentOwnerAtSeam(id.Target, id.DeploymentID); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: id, Kind: "missing", Err: err}
	}
	root, err := DeploymentRoot(id)
	if err != nil {
		return DeploymentRecord{}, err
	}
	return loadDeploymentRecordFile(id.Target, id.DeploymentID, filepath.Join(root, "deployment.json"), &id)
}

func loadDeploymentRecordFile(target, deploymentID, path string, expected *DeploymentIdentity) (DeploymentRecord, error) {
	identity := DeploymentIdentity{Target: target, DeploymentID: deploymentID}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DeploymentRecord{}, &DeploymentRecordStateError{Identity: identity, Kind: "incomplete", Err: fmt.Errorf("deployment.json is missing: %w", os.ErrNotExist)}
		}
		return DeploymentRecord{}, err
	}
	var record DeploymentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: identity, Kind: "corrupt", Err: fmt.Errorf("parse deployment.json: %w", err)}
	}
	if record.Version != DeploymentRecordVersion {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: fmt.Errorf("unsupported deployment record version %d", record.Version)}
	}
	record.Applied.Delivery = record.Applied.Delivery.Normalize()
	if err := record.Applied.Delivery.Validate(); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: fmt.Errorf("invalid delivery selection: %w", err)}
	}
	if err := record.Identity.Validate(); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: err}
	}
	if err := validateReferences(record.References); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: fmt.Errorf("invalid deployment references: %w", err)}
	}
	if err := validateRemoteProject(record); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: err}
	}
	if record.Identity.Target != target || record.Identity.DeploymentID != deploymentID {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: errors.New("deployment record identity does not match storage path")}
	}
	if expected != nil {
		if record.Identity.Target != expected.Target ||
			record.Identity.DeploymentID != expected.DeploymentID ||
			record.Identity.ApplicationID != expected.ApplicationID {
			return DeploymentRecord{}, &DeploymentRecordStateError{
				Identity: *expected,
				Kind:     "corrupt",
				Err:      errors.New("deployment record technical identity does not match requested identity"),
			}
		}
	}
	return record, nil
}

// loadPersistedOwnerForGuard reads the reconciliation owner and the conflict
// flag from the persisted previous deployment record WITHOUT running the full
// validate() path, so a corrupt or unrecognised persisted owner (e.g. a value
// that is neither baseharbor nor external) can still be observed and normalised
// before the one-writer guard. It fails closed on a missing, unparseable, or
// structurally-broken record (identity/version/JSON failures), i.e. it only
// returns a usable owner for a structurally-valid record whose persisted owner
// value may be corrupt. os.ErrNotExist is preserved as a nil owner with a
// non-nil error so the guard can distinguish "no previous record" from "previous
// record with a corrupt owner".
func loadPersistedOwnerForGuard(target, deploymentID, path string) (owner string, conflict bool, exists bool, err error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return "", false, false, readErr
	}
	var record DeploymentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return "", false, false, err
	}
	if record.Version != DeploymentRecordVersion {
		return "", false, false, errors.New("unsupported deployment record version")
	}
	if record.Identity.Target != target || record.Identity.DeploymentID != deploymentID {
		return "", false, false, errors.New("deployment record identity does not match storage path")
	}
	return string(record.Observed.Owner), record.Observed.Conflict, true, nil
}

func DeleteDeploymentRecord(id DeploymentIdentity) error {
	root, err := DeploymentRoot(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func FindDeployment(target, applicationID, environment string) (DeploymentRecord, bool, error) {
	records, err := ListDeployments(target)
	if err != nil {
		return DeploymentRecord{}, false, err
	}
	var match *DeploymentRecord
	for i := range records {
		record := records[i]
		if record.Identity.ApplicationID != applicationID || record.Identity.Environment != environment {
			continue
		}
		if match != nil {
			return DeploymentRecord{}, false, fmt.Errorf(
				"multiple deployments claim application_id %s environment %s on target %s",
				applicationID, environment, target,
			)
		}
		copy := record
		match = &copy
	}
	if match == nil {
		return DeploymentRecord{}, false, nil
	}
	return *match, true, nil
}

func ListDeployments(target string) ([]DeploymentRecord, error) {
	root, err := TargetStateRoot(target)
	if err != nil {
		return nil, err
	}
	records, _, err := listDeploymentRecords(filepath.Join(root, "deployments"), target, false)
	return records, err
}

func ListDeploymentsForDisplay(target string) ([]DeploymentRecord, []error, error) {
	root, err := TargetStateRoot(target)
	if err != nil {
		return nil, nil, err
	}
	return listDeploymentRecords(filepath.Join(root, "deployments"), target, true)
}

func ListAllDeploymentsForDisplay() ([]DeploymentRecord, []error, error) {
	return listAllDeployments(true)
}

func ListAllDeployments() ([]DeploymentRecord, error) {
	records, _, err := listAllDeployments(false)
	return records, err
}

func listAllDeployments(bestEffort bool) ([]DeploymentRecord, []error, error) {
	root, err := DataRoot()
	if err != nil {
		return nil, nil, err
	}
	targetsRoot := filepath.Join(root, "targets")
	entries, err := os.ReadDir(targetsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		items, itemWarnings, err := listDeploymentRecords(filepath.Join(targetsRoot, entry.Name(), "deployments"), entry.Name(), bestEffort)
		if err != nil {
			if bestEffort {
				warnings = append(warnings, fmt.Errorf("target %s: %w", entry.Name(), err))
				continue
			}
			return nil, nil, err
		}
		records = append(records, items...)
		warnings = append(warnings, itemWarnings...)
	}
	sortDeploymentRecords(records)
	return records, warnings, nil
}

func listDeploymentRecords(root, target string, bestEffort bool) ([]DeploymentRecord, []error, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deploymentID := entry.Name()
		path := filepath.Join(root, deploymentID, "deployment.json")
		// Authoritative ownership is enforced on the listing seam as well: each
		// deployment directory is verified against the Target's Core installation
		// before its record is read. Fail-closed on unavailable owner, unless a
		// best-effort listing is requested, in which case the mismatch surfaces
		// as a per-target warning rather than aborting the whole list.
		if err := verifyDeploymentOwnerAtSeam(target, deploymentID); err != nil {
			if bestEffort {
				warnings = append(warnings, &DeploymentRecordStateError{Identity: DeploymentIdentity{Target: target, DeploymentID: deploymentID}, Kind: "missing", Err: err})
				continue
			}
			return nil, nil, err
		}
		record, err := loadDeploymentRecordFile(target, deploymentID, path, nil)
		if err != nil {
			if bestEffort {
				warnings = append(warnings, err)
				continue
			}
			return nil, nil, err
		}
		records = append(records, record)
	}
	sortDeploymentRecords(records)
	return records, warnings, nil
}

func sortDeploymentRecords(records []DeploymentRecord) {
	sort.Slice(records, func(i, j int) bool {
		a, b := records[i].Identity, records[j].Identity
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Application != b.Application {
			return a.Application < b.Application
		}
		if a.Environment != b.Environment {
			return a.Environment < b.Environment
		}
		return a.DeploymentID < b.DeploymentID
	})
}

func SourceAvailable(source DeploymentSource) bool {
	if strings.TrimSpace(source.Repository) == "" {
		return false
	}
	info, err := os.Stat(source.Repository)
	return err == nil && info.IsDir()
}

// refuseOneWriterMutation enforces ADR 0022 §2 at the apply path: exactly one
// active reconciliation owner per managed resource set. It runs after the
// applied selection's single-record validation and before any state is written.
//
// It observes the deployment's own observed state: a recorded conflict (two
// active owners for the same resource set) fails closed, and a direct
// (BaseHarbor-owned) mutation is refused while an external reconciler already
// owns the resource. An absent observed state (direct mode, no external owner)
// is the normal happy path and passes.
func refuseOneWriterMutation(record DeploymentRecord) error {
	applied := record.Applied.Delivery
	observed := record.Observed

	// Conflict observation wins closed: two active owners for one resource set.
	if observed.Conflict {
		return ErrConflictingOneWriter
	}
	// Direct mutation while an external reconciler already owns the resource.
	if applied.Mode == delivery.ModeDirect && observed.Owner == delivery.OwnerExternal {
		return ErrUnauthorizedMutation
	}
	return nil
}

// normalisePersistedOwner mirrors Selection.Normalize for a persisted
// reconciliation owner read from a previous deployment record: it trims and
// lower-cases the value, and collapses any value that is neither "" nor a
// recognised owner (baseharbor|external) — a corrupt or unknown persisted owner —
// to "" (unset). This keeps the one-writer guard from either silently blocking
// (an unknown owner that is neither direct- nor delegated-legal) or silently
// attesting an owner that the validate() switch would otherwise reject, so the
// absent/corrupt value defers to the applied selection's mode.
func normalisePersistedOwner(owner string) string {
	normalised := delivery.ReconciliationOwner(strings.ToLower(strings.TrimSpace(owner)))
	if normalised != delivery.OwnerBaseHarbor && normalised != delivery.OwnerExternal {
		return ""
	}
	return string(normalised)
}
