package deployment

import "github.com/mcpdev80/baseharbor/internal/coreinstallation"

// This file holds the TEST-SCOPE ONLY owner-loan seam of
// VerifyDeploymentOwner (internal/deployment/deployment_store.go). It is split
// out from deployment_store.go so that storage seam stays within the single-file
// readability cap while the loan guard — a distinct, audited concern — lives in
// its own file. No other symbol is moved; the seam calls TargetLoanVerifier
// from deployment_store.go after the tenant hook has refused and the Target's
// own runtime root returns os.ErrNotExist.

// TargetLoanVerifier resolves a marker-proven isolated test Target against the
// single installation its boot Target owns (TEST SCOPE ONLY; the consumer binds
// it in cmd/baha/deployment_record.go and only after the tenant hook has
// refused). Every refusal — an error or a nil state — keeps the seam's
// original ErrDeploymentOwnerMissing sentinel; it can never authorize a
// tenant-managed, connector-bound or production Target, and the default nil
// keeps every production path byte-identical.
var TargetLoanVerifier func(targetName string) (*coreinstallation.State, error)
