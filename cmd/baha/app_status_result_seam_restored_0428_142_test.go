package main

import (
	"context"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Seam restoration for PR #1704 (develop dea88c8f2).
//
// The perses CLI-witness tests in app_status_result_test.go call two
// package-local helpers — persesRunWithIO and persesTestCorelessTarget — that
// PR #1704 removed from cmd/baha/perses_lifecycle_transport_test.go and folded
// into the new persesMatrixRepository / runWithIO / testOperatorContext seam.
//
// This card (0428-142) only owns the backend-witness correction in
// app_status_result.go plus its tests. The transport seam belongs to the base
// (develop), so this file does NOT re-write the witness tests onto the new
// fixture API. It restores exactly the two helpers PR #1704 removed, backed on
// new-develop symbols that still exist (configureTestTargetEmpty,
// runWithIOAsTestOperator). The seam itself lives in the base (develop); these
// shims only re-expose the names the tests were written against, so the card
// fix layers cleanly above the #1704 refactor without redefining any
// new-develop symbol.
//
// persesTestCorelessTarget mirrors the OLD helper (configureTestTargetEmpty(t))
// so the witness probe resolves through the production entry. persesRunWithIO
// routes over the NEW develop's proven perses status seam (app_perses_cli_test.go:
// runPersesStatusCommand) via runWithIOAsTestOperator; the direct `baha app
// perses status` face otherwise hits the managed-environment operator-auth
// boundary (cmd/baha/operator_auth_boundary.go) with a piped (non-terminal)
// test input, so withAssumeYes returns the configuration-required branch there
// instead of the interactive-setup error.

func persesTestCorelessTarget(t *testing.T) deployment.ResolvedTarget {
	t.Helper()
	return configureTestTargetEmpty(t)
}

func persesRunWithIO(ctx context.Context, args []string, out, errOut io.Writer) error {
	// PR #1704 (develop dea88c8f2) removed this package-local helper and folded
	// the perses transport seam onto runWithIO + testOperatorContext. The NEW
	// develop's own perses status seam (cmd/baha/app_perses_cli_test.go,
	// runPersesStatusCommand) passes the managed-environment operator-auth gate
	// via runWithIOAsTestOperator, which wraps the verified-editor
	// testOperatorContext. This card (0428-142) keeps the witness tests on that
	// proven develop seam: the seam lives in the base (develop), and the shim
	// only re-exposes the name the tests were written against so the card fix
	// layers cleanly above the #1704 refactor without touching develop symbols.
	// The direct `baha app perses status` face hits the managed-environment
	// operator-auth boundary (cmd/baha/operator_auth_boundary.go) before its
	// renderer; withAssumeYes mirrors the OLD persesUnauthenticatedCommandContext
	// fixture so the boundary returns the configuration-required branch instead
	// of the interactive-setup error on a piped (non-terminal) test input.
	return runWithIOAsTestOperator(withAssumeYes(ctx, true), args, out, errOut)
}
