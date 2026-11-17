package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestNoImpactPlanCannotAuthorizeChangedImpactWithoutConsent(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	ctx := machineNoninteractiveContext(context.Background())
	before, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil || before.ProviderPlan.Impact != nil {
		t.Fatalf("expected initially no impact, plan=%+v error=%v", before.ProviderPlan.Impact, err)
	}
	resolved.Manifest.Services.SQL = true
	err = authorizeApplicationProviderChanges(ctx, resolved, nil, io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired {
		t.Fatalf("changed provider impact was accepted without consent: %v", err)
	}
}
