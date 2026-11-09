package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// A developer workstation with a remote Target must never create an
// authoritative managed application in its own local XDG state if the remote
// Core connection is absent. The server, not the workstation, owns the ID.
func TestRemoteAppCreateWithoutVerifiedCoreLeavesNoLocalDeployment(t *testing.T) {
	_, _, node := remoteApplicationCoreFixtureEmpty(t)
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", "")
	before, err := deployment.ListDeployments(node.Name)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = appCreateCommand().Run(context.Background(), []string{"unbound-remote-app"}, &out, &out)
	if err == nil {
		t.Fatal("remote app create succeeded without authenticated Core")
	}
	if !strings.Contains(err.Error(), "remote management connection profile not configured") {
		t.Fatalf("unexpected remote Core failure: %v", err)
	}
	after, err := deployment.ListDeployments(node.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("remote app create mutated local deployment registry: before=%d after=%d", len(before), len(after))
	}
	if out.Len() != 0 {
		t.Fatalf("failed remote app create emitted a success result: %q", out.String())
	}
}
