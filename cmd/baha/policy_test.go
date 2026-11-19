package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/policy"
)

func TestRenderPolicyCheckShowsFailClosedProfileFinding(t *testing.T) {
	result := policy.Result{
		Environment: "dev",
		Profile:     "development",
		Decision:    policy.Deny,
		Findings: []policy.Finding{{
			Code:     "access-profile",
			Decision: policy.Deny,
			Scope:    "profile",
			Message:  "no active organization: cannot resolve owned profile",
		}},
	}

	var output strings.Builder
	renderPolicyCheck(&output, result)
	if got := output.String(); !strings.Contains(got, "[DENY] access-profile profile/") || !strings.Contains(got, "Policy: DENY") || strings.Contains(got, "Policy: ALLOW") {
		t.Fatalf("rendered policy check = %q, want fail-closed profile denial", got)
	}
}
