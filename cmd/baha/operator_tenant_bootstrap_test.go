package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestParseOperatorLoginArgs(t *testing.T) {
	args, init, err := parseOperatorLoginArgs([]string{"-e", "dev", "--init-tenant"})
	if err != nil || !init || !reflect.DeepEqual(args, []string{"-e", "dev"}) {
		t.Fatalf("parsed login flags = %v, %t, %v", args, init, err)
	}
	args, init, err = parseOperatorLoginArgs([]string{"-e", "test"})
	if err != nil || init || !reflect.DeepEqual(args, []string{"-e", "test"}) {
		t.Fatalf("ordinary login changed = %v, %t, %v", args, init, err)
	}
	if _, _, err = parseOperatorLoginArgs([]string{"--init-tenant", "--init-tenant"}); err == nil {
		t.Fatal("duplicate first-owner authorization must fail")
	}
}

func TestInitialOperatorBootstrapRejectsRemoteTarget(t *testing.T) {
	target := deployment.ResolvedTarget{AccessProvider: "baseharbor-node-connector"}
	if _, err := initialOperatorLocalCoreFiles(context.Background(), target); !errors.Is(err, errInitialOperatorLocalOnly) {
		t.Fatalf("remote target must not bootstrap initial tenant: %v", err)
	}
}
