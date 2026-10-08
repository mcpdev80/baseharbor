package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteTargetNeverFallsThroughToLocalRuntimeDetection(t *testing.T) {
	for _, access := range []string{"baseharbor-node-connector", "native-api", "vendor/remote"} {
		_, err := detectRuntimeForTarget(context.Background(), deployment.ResolvedTarget{Name: "remote", RuntimeProvider: "docker", AccessProvider: access, AccessReference: "node-a"})
		var problem *machine.Error
		if !errors.As(err, &problem) || problem.Code != machine.ErrorCapabilityMissing {
			t.Fatal("remote target reached local runtime detection", access, err)
		}
	}
}
