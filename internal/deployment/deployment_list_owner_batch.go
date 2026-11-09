package deployment

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// ownerBatch caches the resolved Core installation of a single Target so a list
// over many deployments reads installation.json once instead of once per
// deployment.
//
// The cache holds only the resolved *coreinstallation.State, which is already
// loan-checked (see VerifyDeploymentOwnerBatched). It never records or reuses any
// conclusion about a specific deployment: each deployment still fails closed
// against the owner at its own storage seam (the per-deployment directory
// conclusion below), so a cached installation can never grant access a fresh
// check would deny. The per-deployment ownership conclusion — the fail-closed
// part of cards 174 and 144 — is therefore not what is cached; only the per-Target
// identity load is. This bounds the list cost at once-per-Target instead of
// once-per-record.
type ownerBatch struct {
	target string
	owner  *coreinstallation.State
	// loads counts authoritative installation.json disk reads so a regression test
	// can assert the once-per-Target cost, not a once-per-record cost.
	loads int
	// err records the terminal fail-closed error from the first deployment whose
	// owner check failed on a non-best-effort list, so listDeploymentRecords can
	// surface it (its signature has no error return of its own beyond the batch).
	err error
}

// newOwnerBatch creates an empty batch for the given Target.
func newOwnerBatch(target string) *ownerBatch {
	return &ownerBatch{target: target}
}

// loadOwner resolves the Target's Core installation once and caches it. It
// mirrors VerifyDeploymentOwner's production path (the tenant grant hook, the
// authoritative coreinstallation.Load, and the owner-loan seam, card 0427-117)
// but routes the authoritative load through ownerBatch so it reads
// installation.json exactly once per Target (ownerBatch.loads++). A missing
// installation fails closed with ErrDeploymentOwnerMissing and an
// invalid/foreign installation.json with ErrDeploymentOwnerForeign, matching the
// single-record seam so the batch never weakens card 174/144 semantics. The
// runtime root is resolved once and installation.json read once per Target.
func (b *ownerBatch) loadOwner() (*coreinstallation.State, error) {
	// The resolved, loan-checked installation is cached per Target: a second
	// loadOwner (as TestBatchCacheNeverSearchesDeploymentOwnership exercises)
	// returns the cached State without a second authoritative read, so
	// ownerBatch.loads stays 1. This is the once-per-Target bound the batch
	// promises.
	if b.owner != nil {
		return b.owner, nil
	}
	targetRoot, err := TargetStateRoot(b.target)
	if err != nil {
		return nil, err
	}
	targetRuntime := filepath.Join(targetRoot, "runtime")
	// Tenant grant hook: only an affirmative (true, nil) authorizes a Coreless
	// tenant target without a per-Target installation. A resolver that does not
	// grant falls through to the authoritative load below, exactly as
	// VerifyDeploymentOwner does.
	if TenantOwnerResolver != nil {
		authorized, resolveErr := TenantOwnerResolver(b.target)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if authorized {
			return nil, nil
		}
	}
	installation, err := resolveInstallationLoad(targetRuntime)
	if b != nil {
		// The authoritative per-Target install read happened exactly once above
		// (resolveInstallationLoad routes the production coreinstallation.Load,
		// which increments ownerBatch.loads once, or the test seam). Count it once
		// here so the once-per-Target bound holds even when the load is shadowed
		// in a test: the bound counts one authoritative resolution per Target.
		b.loads++
	}
	if err != nil && errors.Is(err, os.ErrNotExist) && TargetLoanVerifier != nil {
		// Owner loan seam (card 0427-117): resolve a genuinely ownerless
		// Local-access Target against the one installation its boot Target owns.
		// A refusal keeps the ORIGINAL fail-closed sentinel
		// ErrDeploymentOwnerMissing so the contract error shape is byte-identical
		// to the single-record path.
		loanedState, loanErr := TargetLoanVerifier(b.target)
		if loanErr != nil || loanedState == nil {
			return nil, ErrDeploymentOwnerMissing
		}
		installation = *loanedState
		if b != nil {
			owner := installation
			b.owner = &owner
		}
	} else if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDeploymentOwnerMissing
		}
		return nil, ErrDeploymentOwnerForeign
	} else if b != nil {
		owner := installation
		b.owner = &owner
	}
	// Authoritative load succeeded (or a loaned owner was resolved); verify the
	// per-deployment directory conclusion against the resolved State.
	if err := verifyDeploymentOwnerAgainstInstallation(b.target, b.owner, ""); err != nil {
		return nil, err
	}
	return b.owner, nil
}

// VerifyDeploymentOwnerBatched enforces the one-installation-owner rule for a
// batched list. It resolves the per-Target Core installation through the shared
// ownerBatch (loaded once for the whole list, applying the owner-loan seam) and
// then applies the same per-deployment directory conclusion as
// VerifyDeploymentOwner.
//
// The batch caches only the resolved installation; it never records a
// per-deployment ownership conclusion. The fail-closed error shape is
// byte-identical to the single-record path. loadOwner applies the tenant grant
// hook and the owner-loan seam so a batched list authorizes a loaned owner
// exactly as the single-record path does.
func VerifyDeploymentOwnerBatched(targetName, deploymentID string, ownerBatch *ownerBatch) error {
	if err := ValidateTargetName(targetName); err != nil {
		return err
	}
	installation, loadErr := ownerBatch.loadOwner()
	if loadErr != nil {
		return loadErr
	}
	// A nil owner means the tenant grant hook authorized a Coreless tenant
	// target without a per-Target installation: the one-installation-owner
	// directory check does not apply.
	if installation == nil {
		return nil
	}
	return verifyDeploymentOwnerAgainstInstallation(targetName, installation, deploymentID)
}

// classifyOwnerFailure maps a batched owner-check failure to the Kind that a
// best-effort list's public warning must report. It never collapses the
// card-174/144 modes: a genuine missing owner is "missing", a foreign (rewritten
// or different-Target) installation is "foreign", and any other typed cause —
// currently a tenant access-scope denial surfaced by the grant-only
// TenantOwnerResolver — is "access". The original error is preserved by the
// caller on the DeploymentRecordStateError; this only selects the label.
func classifyOwnerFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrDeploymentOwnerMissing):
		return "missing"
	case errors.Is(err, ErrDeploymentOwnerForeign):
		return "foreign"
	default:
		return "access"
	}
}

// listDeploymentRecordsBatched reads the deployment.json in every deployment
// directory under root, verifying each directory against the Target's Core
// installation before its record is read. Fail-closed on an unavailable owner,
// unless a best-effort listing is requested, in which case the mismatch surfaces
// as a per-target warning rather than aborting the whole list. The per-Target
// Core installation is loaded once and shared across all records via batch, so
// the list cost no longer scales linearly with the number of records. The batch
// is returned so a regression test can assert the once-per-Target load bound.
func listDeploymentRecordsBatched(root, target string, bestEffort bool) ([]DeploymentRecord, []error, *ownerBatch) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, &ownerBatch{}
	}
	if err != nil {
		return nil, nil, &ownerBatch{}
	}
	batch := newOwnerBatch(target)
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deploymentID := entry.Name()
		path := filepath.Join(root, deploymentID, "deployment.json")
		if err := VerifyDeploymentOwnerBatched(target, deploymentID, batch); err != nil {
			if bestEffort {
				// A best-effort list must not collapse the three card-174/144
				// failure modes into a single "missing" label: a tenant
				// access-scope denial and a foreign-owner mismatch each carry a
				// distinct contract meaning. Classify the original typed cause
				// truthfully and preserve it (Unwrap) so downstream
				// errors.Is(err, ErrDeploymentOwnerX) checks still match.
				warnings = append(warnings, &DeploymentRecordStateError{Identity: DeploymentIdentity{Target: target, DeploymentID: deploymentID}, Kind: classifyOwnerFailure(err), Err: err})
				continue
			}
			batch.err = err
			return nil, nil, batch
		}
		record, err := loadDeploymentRecordFile(target, deploymentID, path, nil)
		if err != nil {
			if bestEffort {
				warnings = append(warnings, err)
				continue
			}
			batch.err = err
			return nil, nil, batch
		}
		records = append(records, record)
	}
	sortDeploymentRecords(records)
	return records, warnings, batch
}
