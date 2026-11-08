package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRemoteAppStatusRequiresExplicitNameAndVerifiedCore(t *testing.T) {
	_, _, node := remoteApplicationCoreFixtureEmpty(t)
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", "")
	var out bytes.Buffer
	if err := remoteApplicationStatus(context.Background(), node, nil, &out); err == nil ||
		!strings.Contains(err.Error(), "explicit application NAME") {
		t.Fatalf("remote status accepted implicit local app resolution: %v", err)
	}
	if err := remoteApplicationStatus(context.Background(), node, []string{"demo", "--environment", "prod"}, &out); err == nil ||
		!strings.Contains(err.Error(), "dev operator session only") {
		t.Fatalf("remote status accepted unverified production session: %v", err)
	}
	if err := remoteApplicationStatus(context.Background(), node, []string{"demo", "--environment", "dev"}, &out); err == nil ||
		!strings.Contains(err.Error(), "BASEHARBOR_CONNECTION_PROFILE") {
		t.Fatalf("remote status did not fail closed without Core profile: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("failed remote status produced a success-looking result")
	}
	t.Setenv("BASEHARBOR_TARGET", node.Name)
	if err := appStatusCommandWithTLS(application.DefaultStore()).Run(
		context.Background(), []string{"demo", "--environment", "dev"}, &out, &out,
	); err == nil || !strings.Contains(err.Error(), "BASEHARBOR_CONNECTION_PROFILE") {
		t.Fatalf("CLI status bypassed the remote Core verification boundary: %v", err)
	}

}
