package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/preflight"
)

func TestApplicationDestroyPreflightErrorExplainsRemoteFailureWithoutReportingSuccess(t *testing.T) {
	err := applicationDestroyPreflightError([]preflight.Result{
		{Name: "manifest", OK: true},
		{Name: "OpenBao application scope ownership", Detail: "AppRole login failed"},
	})
	if err == nil || !strings.Contains(err.Error(), "nothing was deleted") || !strings.Contains(err.Error(), "OpenBao application scope ownership: AppRole login failed") || strings.Contains(err.Error(), "manifest") {
		t.Fatalf("destroy preflight failure is not safely diagnosable: %v", err)
	}
}

func TestApplicationDestroyPreflightErrorBoundsDetails(t *testing.T) {
	long := strings.Repeat("x", 500)
	err := applicationDestroyPreflightError([]preflight.Result{{Name: "runtime", Detail: long}})
	if got := len(err.Error()); got > 300 {
		t.Fatalf("destroy preflight error is unbounded (%d bytes)", got)
	}
}
