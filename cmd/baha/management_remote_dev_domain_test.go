package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteDomainAndGuidedSelectionUseOwningCore(t *testing.T) {
	ctx, target, core := remoteJourneyFixture(t)
	// Read the selected Core's value rather than overwriting it with a local default.
	got, err := configureDevelopmentDomain(ctx, "")
	if err != nil || got.Domain != "core.example.test" {
		t.Fatalf("wrong remote default: %v", err)
	}
	setup := guidedDevAccessSetup{target: target.Name, domain: "mcp-dev.de", configureDomain: true}
	if err := applyGuidedDevAccess(ctx, setup); err != nil {
		t.Fatal(err)
	}
	domain, err := devaccess.LoadDomain(target.Name)
	if err != nil || domain != "mcp-dev.de" {
		t.Fatal("selected domain missing from protected local projection")
	}
	core.mu.Lock()
	defer core.mu.Unlock()
	if core.developmentDomain != "mcp-dev.de" {
		t.Fatal("wizard stored domain locally without applying to Core")
	}
}

func TestRemoteDomainPromptUsesCoreDefaultWithoutApplyingBeforeConfirmation(t *testing.T) {
	ctx, target, _ := remoteJourneyFixture(t)
	if _, err := devaccess.Configure(target.Name, "dev", "test-user", "test-value"); err != nil {
		t.Fatal(err)
	}
	m := application.New("demo", "dev", true, false, false)
	m.Services.SQLManagementUI = true
	var out bytes.Buffer
	setup, err := collectGuidedDevAccess(ctx, bufio.NewReader(strings.NewReader("\n")), &out, m)
	if err != nil || setup.domain != "core.example.test" {
		t.Fatalf("prompt ignored owning Core: %v", err)
	}
	if _, err := devaccess.LoadDomain(target.Name); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("wizard changed local domain before confirmation")
	}
}

func TestHTTPDevelopmentDomainRejectsScopeAndInvalidInputBeforeWriting(t *testing.T) {
	target := configureTestTarget(t)
	for _, tc := range []struct {
		scope machine.OperationContext
		input machineHTTPDevelopmentDomainInput
	}{
		{machine.OperationContext{Target: target.Name, Environment: "dev"}, machineHTTPDevelopmentDomainInput{Target: "foreign", Domain: "mcp-dev.de"}},
		{machine.OperationContext{Target: target.Name, Environment: "prod"}, machineHTTPDevelopmentDomainInput{Target: target.Name, Domain: "mcp-dev.de"}},
		{machine.OperationContext{Target: target.Name, Environment: "dev"}, machineHTTPDevelopmentDomainInput{Target: target.Name, Domain: "https://mcp-dev.de/"}},
	} {
		raw, _ := json.Marshal(tc.input)
		if _, err := executeHTTPDevelopmentDomain(context.Background(), tc.scope, raw); err == nil {
			t.Fatal("invalid domain or selector accepted")
		}
	}
	scope := machine.OperationContext{Target: target.Name, Environment: "dev"}
	raw, _ := json.Marshal(machineHTTPDevelopmentDomainInput{Target: target.Name})
	if _, err := executeHTTPDevelopmentDomain(context.Background(), scope, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := devaccess.LoadDomain(target.Name); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read/default selection mutated domain")
	}
	if httpOperationKinds["dev.domain"] != httpPlatformMutation {
		t.Fatal("domain operation missing from verified Machine HTTP transport")
	}
}
