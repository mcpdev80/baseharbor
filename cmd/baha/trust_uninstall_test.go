package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/health"
)

func TestTrustUninstallCommandRegisteredAndExplicitConsent(t *testing.T) {
	cmd := trustCommand()
	var uninstall bool
	for _, child := range cmd.Children {
		if child.Name != "uninstall" {
			continue
		}
		uninstall = true
		if !strings.Contains(child.Usage, "--yes") {
			t.Fatalf("missing explicit consent in help: %q", child.Usage)
		}
		if !strings.Contains(child.Long, "only") {
			t.Fatalf("missing ownership boundary: %q", child.Long)
		}
		if err := child.Run(context.Background(), []string{"--unknown"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatal("unknown arguments silently accepted")
		}
	}
	if !uninstall {
		t.Fatal("trust uninstall not registered")
	}
}

func TestTrustUninstallMachineResultSchema(t *testing.T) {
	payload, err := json.Marshal(trustUninstallResult{ContractVersion: "v1", Removed: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"contract_version":"v1"`, `"removed":0`} {
		if !strings.Contains(string(payload), expected) {
			t.Fatalf("missing %s: %s", expected, payload)
		}
	}
}

func TestDoctorHostTrustActionRequiresConfirmation(t *testing.T) {
	findings := classifyDoctorFindings([]health.Check{{Name: "host-trust-ownership", OK: false, Message: "invalid ownership state"}})
	if len(findings) != 1 || findings[0].Class != doctorNeedsConfirmation || !strings.Contains(findings[0].Action, "baha trust uninstall") {
		t.Fatalf("missing explicit trust cleanup guidance: %+v", findings)
	}
}
