package main

// Card 0428-231 target.go provider-fix regression: detectRuntimeForTarget's
// access-provider guard must treat the runtime-specific host-local provider
// names the registration commands record ("--access local-docker" and its
// Podman sibling) as LOCAL, so the invocation-local grant can reach the
// engine-ownership probe instead of being rejected as a remote Target.
// Every other provider (remote, connector, unknown) and any provider-versus-
// runtime mismatch keeps failing closed with ErrorCapabilityMissing.

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestDetectRuntimeForTargetLocalRuntimeAccessProviders(t *testing.T) {
	cases := []struct {
		name         string
		access       string
		runtime      string
		wantRejected bool
	}{
		{name: "generic local alias unchanged", access: "local", runtime: "docker", wantRejected: false},
		{name: "empty access unchanged", access: "", runtime: "docker", wantRejected: false},
		{name: "local-docker on docker runtime is local", access: "local-docker", runtime: "docker", wantRejected: false},
		{name: "local-podman on podman runtime is local", access: "local-podman", runtime: "podman", wantRejected: false},
		{name: "remote provider fails closed", access: "baseharbor-node-connector", runtime: "docker", wantRejected: true},
		{name: "core provider fails closed", access: "baseharbor-core", runtime: "docker", wantRejected: true},
		{name: "unknown provider fails closed", access: "somewhere-else", runtime: "docker", wantRejected: true},
		{name: "local-docker on podman runtime fails closed", access: "local-docker", runtime: "podman", wantRejected: true},
		{name: "local-podman on docker runtime fails closed", access: "local-podman", runtime: "docker", wantRejected: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := deployment.ResolvedTarget{
				Name:            "probe-target",
				RuntimeProvider: tc.runtime,
				AccessProvider:  tc.access,
			}
			_, err := detectRuntimeForTarget(context.Background(), target)
			if !tc.wantRejected {
				// Accepted forms pass the provider guard: any error must not be
				// the remote-capability rejection; engine probing/ownership
				// decides from here (a local probe error in a sandbox without
				// the engine is fine and expected).
				var merr *machine.Error
				if errors.As(err, &merr) && merr.Code == machine.ErrorCapabilityMissing {
					t.Fatalf("local access provider %q with runtime %q was rejected as remote: %v", tc.access, tc.runtime, err)
				}
				return
			}
			var merr *machine.Error
			if !errors.As(err, &merr) || merr.Code != machine.ErrorCapabilityMissing {
				t.Fatalf("provider %q (runtime %q) must fail closed with ErrorCapabilityMissing, got %v", tc.access, tc.runtime, err)
			}
		})
	}
}
