package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/health"
)

func TestEvaluateControlPlaneAvailabilityReportsRuntimeHostGuarantee(t *testing.T) {
	running := []string{
		"postgres-member-1", "postgres-member-2", "postgres-member-3",
		"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3",
		"openbao-member-1", "openbao-member-2", "openbao-member-3",
	}
	report := evaluateControlPlaneAvailability(running, []health.Check{
		{Name: "postgres", OK: true},
		{Name: "openbao", OK: true},
	}, true)
	if !report.Satisfied {
		t.Fatalf("full control-plane HA topology reported unsatisfied: %#v", report)
	}
	for _, want := range []string{
		"requested=ha",
		"resolved=postgresql:3,openbao:3",
		"failure-domain=runtime-host",
		"host-failure-tolerance=false",
		"satisfied=true",
	} {
		if !strings.Contains(report.Detail(), want) {
			t.Fatalf("availability detail %q missing %q", report.Detail(), want)
		}
	}
}

func TestEvaluateControlPlaneAvailabilityToleratesOneMemberFailure(t *testing.T) {
	running := []string{
		"postgres-member-1", "postgres-member-2",
		"postgres-etcd-1", "postgres-etcd-2",
		"openbao-member-1", "openbao-member-2",
	}
	report := evaluateControlPlaneAvailability(running, []health.Check{
		{Name: "postgres", OK: true},
		{Name: "openbao", OK: true},
	}, true)
	if !report.Satisfied {
		t.Fatalf("one member failure should preserve the requested guarantee: %#v", report)
	}
}

func TestEvaluateControlPlaneAvailabilityRejectsLostQuorumOrStableEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		running []string
		checks  []health.Check
	}{
		{
			name: "postgres quorum lost",
			running: []string{
				"postgres-member-1", "postgres-member-2",
				"postgres-etcd-1",
				"openbao-member-1", "openbao-member-2",
			},
			checks: []health.Check{{Name: "postgres", OK: true}, {Name: "openbao", OK: true}},
		},
		{
			name: "stable postgres endpoint failed",
			running: []string{
				"postgres-member-1", "postgres-member-2", "postgres-member-3",
				"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3",
				"openbao-member-1", "openbao-member-2", "openbao-member-3",
			},
			checks: []health.Check{{Name: "postgres", OK: false}, {Name: "openbao", OK: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluateControlPlaneAvailability(tt.running, tt.checks, true); got.Satisfied {
				t.Fatalf("unsatisfied control-plane topology reported healthy: %#v", got)
			}
		})
	}
}

func TestSingleControlPlaneAvailabilityReportsActualTopology(t *testing.T) {
	checks := []health.Check{{Name: "postgres", OK: true}, {Name: "openbao", OK: true}}
	running := []string{"postgres-member-1", "openbao-member-1"}
	report := evaluateControlPlaneAvailability(running, checks, false)
	if !report.Satisfied || !strings.Contains(report.Detail(), "failover=false") || !strings.Contains(report.Detail(), "etcd:0/0") {
		t.Fatalf("single availability: %#v %s", report, report.Detail())
	}
	if evaluateControlPlaneAvailability(running, checks, true).Satisfied {
		t.Fatal("single topology must never satisfy HA quorum")
	}
	if evaluateControlPlaneAvailability(append(running, "postgres-etcd-1"), checks, false).Satisfied {
		t.Fatal("hidden etcd must not satisfy single topology")
	}
}
