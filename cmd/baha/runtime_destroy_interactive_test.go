package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestInstallationDestroyReadsInteractiveConsentFromCLIInput(t *testing.T) {
	scope, err := application.ComposeProviderImpactScope(application.ProviderScopeOwner{
		Owner: "installation.destroy", Reference: "/test/config", Subject: "installation.destroy", Revision: "test-revision", Destructive: true,
	}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	oldInput := runtimeInput
	runtimeInput = strings.NewReader("yes\nyes\nyes\n")
	t.Cleanup(func() { runtimeInput = oldInput })

	ack := &application.ProviderChangeConsent{}
	ctx := context.WithValue(t.Context(), runtimeDestroyAcknowledgementKey{}, ack)
	var output bytes.Buffer
	accepted, err := authorizeRuntimeDestroyImpact(ctx, runtimeDestroyImpact{Scope: scope}, true, &output)
	if err != nil || !accepted {
		t.Fatalf("interactive installation destroy consent was not collected: accepted=%t err=%v\n%s", accepted, err, output.String())
	}
	if ack.PlanID != scope.PlanID || !ack.First || !ack.Second || !ack.DeleteData {
		t.Fatalf("destroy consent did not acknowledge the displayed plan and deletion: %+v", ack)
	}
	for _, expected := range []string{"(1/2, plan ", "(2/2, plan ", "separately authorize permanent deletion"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing confirmation prompt %q in output:\n%s", expected, output.String())
		}
	}
}

func TestRemoteInstallationDestroyPromptsForRuntimeCLIInput(t *testing.T) {
	ctx, target, core := remoteJourneyFixture(t)
	oldInput := runtimeInput
	runtimeInput = strings.NewReader("yes\nyes\nyes\n")
	t.Cleanup(func() { runtimeInput = oldInput })
	var output bytes.Buffer
	if err := remoteInstallationDestroy(ctx, target, &output); err != nil {
		t.Fatalf("remote installation destroy did not collect CLI input: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "confirm the displayed changes") || !strings.Contains(output.String(), "separately authorize permanent deletion") {
		t.Fatalf("remote destroy did not show the consent prompts:\n%s", output.String())
	}
	core.mu.Lock()
	defer core.mu.Unlock()
	if got := strings.Count(strings.Join(core.operations, ","), "installation.destroy"); got != 2 {
		t.Fatalf("expected remote preview and confirmed execution, got %v", core.operations)
	}
}
