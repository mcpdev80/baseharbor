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

func TestRemoteDevelopmentTargetNeverUsesLocalDockerOrPodmanHost(t *testing.T) {
	// These settings are controlled by the developer workstation and must
	// never provide runtime authority for a remotely selected Target.
	t.Setenv("DOCKER_HOST", "unix:///tmp/baseharbor-developer-docker.sock")
	t.Setenv("DOCKER_CONTEXT", "developer-local")
	t.Setenv("CONTAINER_HOST", "unix:///tmp/baseharbor-developer-podman.sock")

	for _, runtime := range []string{"docker", "podman"} {
		for _, access := range []string{"baseharbor-node-connector", "native-api"} {
			t.Run(runtime+"/"+access, func(t *testing.T) {
				target := deployment.ResolvedTarget{
					Name:            "remote-dev",
					RuntimeProvider: runtime,
					AccessProvider:  access,
					AccessReference: "remote-node",
				}
				provider, err := detectRuntimeForTarget(context.Background(), target)
				var failure *machine.Error
				if provider != nil || !errors.As(err, &failure) ||
					failure.Code != machine.ErrorCapabilityMissing {
					t.Fatalf("remote %s/%s selected local workstation runtime: provider=%v error=%v", runtime, access, provider, err)
				}
			})
		}
	}
}
