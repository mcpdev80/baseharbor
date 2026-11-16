package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

func TestNodeCommandRegistersCompleteLifecycle(t *testing.T) {
	command := nodeCommand()
	if command.Name != "node" {
		t.Fatalf("unexpected command %q", command.Name)
	}
	want := map[string]bool{"add": false, "connect": false, "list": false, "status": false, "disconnect": false}
	for _, child := range command.Children {
		if _, ok := want[child.Name]; ok {
			want[child.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("node command missing %s", name)
		}
	}
}

func TestNodeAddNonInteractiveRequiresExplicitInputs(t *testing.T) {
	ctx := machineNoninteractiveContext(context.Background())
	var out bytes.Buffer
	err := nodeAdd(ctx, nil, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "requires NODE") {
		t.Fatalf("expected bounded non-interactive usage error, got %v", err)
	}
}

func TestNodeUserUnitNeverContainsEnrollmentCredentialMaterial(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateRoot := filepath.Join(home, ".local", "share", "baseharbor-node-connector")
	bundle := nodeEnrollmentBundle{
		ContractVersion: nodeBundleVersion,
		TenantID:        "12345678-1234-1234-1234-123456789abc",
		TargetID:        "edge-a",
		NodeID:          "node-a",
		Runtime:         "podman",
		CoreURL:         "https://core.example:8443",
		CoreAddress:     "core.example:9443",
		ServerName:      "core.example",
		Authorization: targetenrollment.Bootstrap{
			Token:     "secret-token-never-in-unit",
			Nonce:     "secret-nonce-never-in-unit",
			ExpiresAt: time.Now().Add(time.Minute),
		},
	}
	unitPath, err := installNodeUserUnit(
		"/usr/local/bin/baseharbor-node-connector",
		stateRoot,
		filepath.Join(stateRoot, "bootstrap-ca.pem"),
		filepath.Join(stateRoot, "bootstrap.authorization.json"),
		bundle,
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	unit := string(data)
	for _, secret := range []string{bundle.Authorization.Token, bundle.Authorization.Nonce} {
		if strings.Contains(unit, secret) {
			t.Fatal("systemd unit leaked enrollment credential material")
		}
	}
	if !strings.Contains(unit, "--bootstrap-authorization-file") {
		t.Fatal("connector unit does not use protected authorization-file handoff")
	}
	if !strings.Contains(unit, "--user") && strings.Contains(unit, "sudo") {
		t.Fatal("connector unit unexpectedly escalates privileges")
	}
}

func TestNodeMachineCoverageIsSemantic(t *testing.T) {
	want := map[string]bool{
		"baha node add": false, "baha node connect": false, "baha node list": false,
		"baha node status": false, "baha node disconnect": false,
	}
	for _, row := range currentCommandCoverage() {
		if _, ok := want[row.Command]; !ok {
			continue
		}
		if row.Classification != "semantic" || row.Operation == "" || row.MCPTool == "" {
			t.Fatalf("node command lacks semantic machine coverage: %#v", row)
		}
		want[row.Command] = true
	}
	for command, found := range want {
		if !found {
			t.Fatalf("missing machine coverage for %s", command)
		}
	}
}

func TestNodeCoreURLRejectsCredentialAndPlainHTTP(t *testing.T) {
	for _, raw := range []string{
		"http://core.example:8443",
		"https://user:pass@core.example:8443",
		"https://core.example:8443/api/v1",
	} {
		if err := validateCoreURL(raw); err == nil {
			t.Fatalf("unsafe Core URL admitted: %s", raw)
		}
	}
	if err := validateCoreURL("https://core.example:8443"); err != nil {
		t.Fatalf("valid Core URL rejected: %v", err)
	}
}
