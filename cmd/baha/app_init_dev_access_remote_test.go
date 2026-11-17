package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
)

func TestGuidedDevelopmentCredentialsReachSelectedRemoteCore(t *testing.T) {
	for _, generated := range []bool{false, true} {
		t.Run(map[bool]string{false: "custom", true: "generated"}[generated], func(t *testing.T) {
			ctx, target, _ := remoteJourneyFixture(t)
			saved := guidedRemoteCredentialReconcile
			t.Cleanup(func() { guidedRemoteCredentialReconcile = saved })
			calls := 0
			guidedRemoteCredentialReconcile = func(_ context.Context, selected deployment.ResolvedTarget, user, password string, replace bool) error {
				calls++
				if selected.Name != target.Name || user != "chosen-user" || password == "" || !replace {
					t.Fatal("wizard choice not sent to selected Core as explicit configuration")
				}
				if !generated && password != "custom-test-value" {
					t.Fatal("custom password changed")
				}
				return nil
			}
			secret := []byte("custom-test-value")
			setup := guidedDevAccessSetup{target: target.Name, username: "chosen-user", password: secret, configureCredentials: true, generatePassword: generated}
			if err := applyGuidedDevAccess(ctx, setup); err != nil {
				t.Fatal(err)
			}
			got, err := devaccess.Load(target.Name, "dev")
			if err != nil || got.Username != "chosen-user" || got.Password == "" || calls != 1 {
				t.Fatalf("configured login missing: %v", err)
			}
			for _, b := range secret {
				if b != 0 {
					t.Fatal("password buffer not cleared")
				}
			}
		})
	}
}

func TestGuidedDevelopmentCredentialsRejectFailureWithoutReplacingProtectedCopy(t *testing.T) {
	ctx, target, _ := remoteJourneyFixture(t)
	if _, err := devaccess.Configure(target.Name, "dev", "previous-user", "previous-test-value"); err != nil {
		t.Fatal(err)
	}
	saved := guidedRemoteCredentialReconcile
	t.Cleanup(func() { guidedRemoteCredentialReconcile = saved })
	guidedRemoteCredentialReconcile = func(context.Context, deployment.ResolvedTarget, string, string, bool) error {
		return errors.New("Core rejected configuration")
	}
	setup := guidedDevAccessSetup{target: target.Name, username: "next-user", password: []byte("next-test-value"), configureCredentials: true}
	if err := applyGuidedDevAccess(ctx, setup); err == nil {
		t.Fatal("remote failure hidden")
	}
	got, err := devaccess.Load(target.Name, "dev")
	if err != nil || got.Username != "previous-user" || got.Password != "previous-test-value" {
		t.Fatal("previous protected login lost")
	}
	// Reuse never changes the Core credential implicitly.
	if err := applyGuidedDevAccess(ctx, guidedDevAccessSetup{target: target.Name}); err != nil {
		t.Fatal(err)
	}
}

func TestGuidedDevelopmentCredentialsKeepLocalTargetLocal(t *testing.T) {
	target := configureTestTarget(t)
	saved := guidedRemoteCredentialReconcile
	t.Cleanup(func() { guidedRemoteCredentialReconcile = saved })
	guidedRemoteCredentialReconcile = func(context.Context, deployment.ResolvedTarget, string, string, bool) error {
		t.Fatal("local setup attempted remote configuration")
		return nil
	}
	setup := guidedDevAccessSetup{target: target.Name, username: "local-user", password: []byte("local-test-value"), configureCredentials: true}
	if err := applyGuidedDevAccess(context.Background(), setup); err != nil {
		t.Fatal(err)
	}
	got, err := devaccess.Load(target.Name, "dev")
	if err != nil || got.Username != "local-user" {
		t.Fatalf("local setup changed: %v", err)
	}
	setup.target = "another-target"
	setup.password = []byte("not-applied")
	if err := applyGuidedDevAccess(context.Background(), setup); err == nil {
		t.Fatal("changed Target accepted")
	}
	for _, b := range setup.password {
		if b != 0 {
			t.Fatal("rejected secret buffer not cleared")
		}
	}
}
