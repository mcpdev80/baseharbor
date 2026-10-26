package main

import "testing"

// Keep local production workload authority in the mandatory runtime catalogue,
// independently of push-message selectors and a caller's environment defaults.
// The shared real lifecycle fixture verifies CLI and actual stdio MCP access,
// selected rootless ownership, zero management bootstrap and owned cleanup.
func TestCorelessLocalProdRuntimeLifecycle(t *testing.T) {
	t.Setenv("BASEHARBOR_CORELESS_ENVIRONMENT", "prod")
	TestCorelessFreshWorkloadRuntimeLifecycle(t)
}
