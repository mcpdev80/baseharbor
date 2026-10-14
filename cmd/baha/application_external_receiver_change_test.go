package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

func externalReceiverPlanFixture(t *testing.T) resolvedApplication {
	t.Helper()
	r := corelessResolvedFixture(t)
	r.FromRepository = false
	r.Store = application.Store{Root: t.TempDir(), Namespace: r.Target.Name}
	r.Manifest = application.WithOTLPTelemetry(r.Manifest, "traces")
	t.Setenv(application.OTLPEndpointEnv, "https://receiver-before.example.test")
	t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-before")
	if err := application.RegisterExternalProviderAt(r.TargetStateRoot, externalprovider.Registration{
		ID: "default", ProviderID: "company/otlp", Endpoint: application.ExternalOTLPEndpoint(),
		CredentialRef: "file:///run/secrets/otlp", Provider: capability.ExternalOTLP,
		Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem},
	}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	f := application.RuntimeFilesFor(r.Store, r.Manifest)
	if err := os.MkdirAll(f.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{f.Env, f.ApplicationEnv} {
		if err := os.WriteFile(p, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := application.MaterializeExternalOTLPBinding(r.Manifest, f, application.ExternalOTLPEndpoint(), os.Getenv(telemetry.ExternalHeadersEnv)); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestExternalReceiverChangeRequiresCurrentPlanBeforeAnyDeliveryMutation(t *testing.T) {
	for _, change := range []string{"endpoint", "authorization", "anonymous", "missing-authorization", "credential-url"} {
		t.Run(change, func(t *testing.T) {
			r := externalReceiverPlanFixture(t)
			f := application.RuntimeFilesFor(r.Store, r.Manifest)
			paths := []string{f.Env, f.ApplicationEnv, filepath.Join(r.TargetStateRoot, "provider-registry.json")}
			before := map[string][]byte{}
			for _, path := range paths {
				before[path], _ = os.ReadFile(path)
			}
			switch change {
			case "endpoint":
				t.Setenv(application.OTLPEndpointEnv, "https://receiver-after.example.test")
				t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-after")
			case "authorization":
				t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-after")
			case "anonymous":
				t.Setenv(telemetry.ExternalHeadersEnv, "")
			case "missing-authorization":
				t.Setenv(application.OTLPEndpointEnv, "https://receiver-after.example.test")
				if err := os.Unsetenv(telemetry.ExternalHeadersEnv); err != nil {
					t.Fatal(err)
				}
			case "credential-url":
				t.Setenv(application.OTLPEndpointEnv, "https://user:fixture-sensitive-url@receiver-after.example.test/?access=fixture-sensitive-query")
			}
			plan, err := buildResolvedApplicationPlan(t.Context(), r)
			if err != nil || plan.ProviderPlan.Impact == nil {
				t.Fatalf("receiver/material change lacked concrete consequence disclosure: %v", err)
			}
			impact := plan.ProviderPlan.Impact
			if impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
				t.Fatal("unknown delivery consequences lacked two current confirmations")
			}
			encoded, _ := json.Marshal(plan)
			if strings.Contains(string(encoded), "fixture-sensitive") {
				t.Fatal("consequence plan disclosed authorization")
			}
			for _, consent := range []*application.ProviderChangeConsent{nil, {PlanID: impact.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}, {PlanID: impact.PlanID, First: true, Second: true}} {
				err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), consent), r, nil, io.Discard)
				if consent != nil && consent.PlanID == impact.PlanID && consent.Second && change != "missing-authorization" && change != "credential-url" {
					if err != nil {
						t.Fatalf("supported explicitly acknowledged delivery permanently refused: %v", err)
					}
				} else {
					var failure *machine.Error
					if !errors.As(err, &failure) {
						t.Fatalf("invalid consent/material crossed execution admission: %v", err)
					}
					if consent != nil && consent.PlanID == impact.PlanID && consent.Second {
						if failure.CauseCode != "external_provider_material_preflight_failed" || failure.Details == nil {
							t.Fatal("invalid material lost its concrete acknowledged consequence plan")
						}
					} else if failure.Code != machine.ErrorApprovalRequired && failure.Code != machine.ErrorConflict {
						t.Fatal("technical preflight hid the current two-stage acknowledgement")
					}
				}
				for _, path := range paths {
					after, _ := os.ReadFile(path)
					if !bytes.Equal(before[path], after) {
						t.Fatal("read-only planning or admission changed current delivery")
					}
				}
			}
			oldID := impact.PlanID
			t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer another-selected-value")
			fresh, err := buildResolvedApplicationPlan(t.Context(), r)
			if err != nil || fresh.ProviderPlan.Impact == nil || fresh.ProviderPlan.Impact.PlanID == oldID {
				t.Fatal("changed selected material did not invalidate prior consent")
			}
			stale := application.ProviderChangeConsent{PlanID: oldID, First: true, Second: true}
			err = authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), &stale), r, nil, io.Discard)
			var staleFailure *machine.Error
			if !errors.As(err, &staleFailure) || staleFailure.Code != machine.ErrorConflict {
				t.Fatalf("prior material consent remained usable: %v", err)
			}
		})
	}
}

func TestExternalSameReceiverOmittedAuthorizationHasNoInventedChange(t *testing.T) {
	r := externalReceiverPlanFixture(t)
	if err := os.Unsetenv(telemetry.ExternalHeadersEnv); err != nil {
		t.Fatal(err)
	}
	change, err := applicationExternalReceiverChange(r)
	if err != nil || change != nil {
		t.Fatal("same-receiver retained authorization invented a destructive replacement")
	}
}
