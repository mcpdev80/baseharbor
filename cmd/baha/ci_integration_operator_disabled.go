//go:build !baseharbor_ci_operator_testonly

package main

import "context"

// withCIIntegrationOperator is a no-op in all normal builds. Runtime acceptance
// jobs may opt into the separate test-only build tag when exercising the CLI
// against disposable CI infrastructure.
func withCIIntegrationOperator(ctx context.Context) context.Context { return ctx }
